// Ported near-verbatim from
// com.sixt.service.managed-agents/internal/auth/jwks.go — see
// specs/workflow-server-auth.md for why: this server enforces the same
// Keycloak JWT validation as the managed-agents control plane, with no
// external JWT library dependency.
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// JWKSValidator validates JWTs using Keycloak's JWKS endpoint.
type JWKSValidator struct {
	keycloakURL string
	realm       string

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey // kid -> public key
	lastFetch time.Time
	cacheTTL  time.Duration

	httpClient *http.Client
	stopCh     chan struct{}
}

// JWK represents a single JSON Web Key.
type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKSet is the response from the JWKS endpoint.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// NewJWKSValidator creates a validator that fetches and caches JWKS keys.
// Starts a background goroutine to refresh keys hourly.
func NewJWKSValidator(keycloakURL, realm string) *JWKSValidator {
	v := &JWKSValidator{
		keycloakURL: keycloakURL,
		realm:       realm,
		keys:        make(map[string]*rsa.PublicKey),
		cacheTTL:    24 * time.Hour,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		stopCh:      make(chan struct{}),
	}

	// Initial fetch (non-blocking — warn on failure)
	if err := v.refreshKeys(); err != nil {
		fmt.Printf("WARNING: initial JWKS fetch failed: %v\n", err)
	}

	// Background refresh
	go v.backgroundRefresh()

	return v
}

// Stop halts the background refresh goroutine.
func (v *JWKSValidator) Stop() {
	close(v.stopCh)
}

// ValidateToken parses and validates a JWT string.
// Returns the decoded claims on success.
func (v *JWKSValidator) ValidateToken(tokenString string) (*KeycloakClaims, error) {
	// Split JWT
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode JWT header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("parse JWT header: %w", err)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported signing algorithm: %s", header.Alg)
	}

	// Find key
	key, err := v.getKey(header.Kid)
	if err != nil {
		return nil, err
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode JWT signature: %w", err)
	}

	h := sha256.New()
	h.Write([]byte(signingInput))
	digest := h.Sum(nil)

	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest, signature); err != nil {
		return nil, fmt.Errorf("invalid JWT signature: %w", err)
	}

	// Decode claims
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT claims: %w", err)
	}
	var claims KeycloakClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}

	// Validate expiration
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	// Validate issuer
	expectedIssuer := fmt.Sprintf("%s/realms/%s", v.keycloakURL, v.realm)
	if claims.Issuer != expectedIssuer {
		return nil, fmt.Errorf("invalid issuer: got %q, want %q", claims.Issuer, expectedIssuer)
	}

	// Validate required claims
	if claims.Sub == "" {
		return nil, fmt.Errorf("missing sub claim")
	}

	return &claims, nil
}

// getKey returns the RSA public key for the given kid, refreshing if not found.
func (v *JWKSValidator) getKey(kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	v.mu.RUnlock()
	if ok {
		return key, nil
	}

	// Key not found — try refreshing
	if err := v.refreshKeys(); err != nil {
		return nil, fmt.Errorf("refresh JWKS: %w", err)
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("key %q not found in JWKS", kid)
	}
	return key, nil
}

// refreshKeys fetches the JWKS endpoint and updates the cache.
func (v *JWKSValidator) refreshKeys() error {
	url := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/certs", v.keycloakURL, v.realm)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned HTTP %d", resp.StatusCode)
	}

	var jwks JWKSet
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, jwk := range jwks.Keys {
		if jwk.Kty != "RSA" || jwk.Use != "sig" {
			continue
		}
		key, err := jwkToRSAPublicKey(&jwk)
		if err != nil {
			continue // skip malformed keys
		}
		keys[jwk.Kid] = key
	}

	v.mu.Lock()
	v.keys = keys
	v.lastFetch = time.Now()
	v.mu.Unlock()

	return nil
}

// backgroundRefresh periodically refreshes the JWKS cache.
func (v *JWKSValidator) backgroundRefresh() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-v.stopCh:
			return
		case <-ticker.C:
			if err := v.refreshKeys(); err != nil {
				fmt.Printf("WARNING: JWKS background refresh failed: %v\n", err)
			}
		}
	}
}

// jwkToRSAPublicKey constructs an RSA public key from JWK N and E fields.
func jwkToRSAPublicKey(jwk *JWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("decode JWK N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("decode JWK E: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	return &rsa.PublicKey{N: n, E: e}, nil
}
