package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// generateTestKey creates an RSA key pair and returns the private key and JWK.
func generateTestKey(t *testing.T, kid string) (*rsa.PrivateKey, JWK) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk := JWK{
		Kid: kid,
		Kty: "RSA",
		Alg: "RS256",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}
	return key, jwk
}

// signTestJWT creates a JWT signed with the given key. Callers supply
// complete claims so tests can build issuer/expiry/etc. relative to the
// test server's own URL.
func signTestJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	h := sha256.New()
	h.Write([]byte(signingInput))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest)
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func newTestValidator(t *testing.T, jwk JWK) (*JWKSValidator, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(JWKSet{Keys: []JWK{jwk}})
	}))
	t.Cleanup(srv.Close)

	v := NewJWKSValidator(srv.URL, "test-realm")
	t.Cleanup(v.Stop)
	return v, srv.URL
}

func validClaims(issuer string) map[string]any {
	return map[string]any{
		"sub":                "user-sub-123",
		"email":              "user@example.com",
		"preferred_username": "testuser",
		"name":               "Test User",
		"iss":                issuer,
		"exp":                time.Now().Add(1 * time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"realm_access":       map[string]any{"roles": []string{"user", "admin"}},
	}
}

func TestJWKSValidator_ValidToken(t *testing.T) {
	key, jwk := generateTestKey(t, "kid-1")
	v, srvURL := newTestValidator(t, jwk)

	claims := validClaims(fmt.Sprintf("%s/realms/test-realm", srvURL))
	token := signTestJWT(t, key, "kid-1", claims)

	result, err := v.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if result.Sub != "user-sub-123" {
		t.Errorf("Sub = %q, want %q", result.Sub, "user-sub-123")
	}
	if len(result.RealmAccess.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(result.RealmAccess.Roles))
	}
}

func TestJWKSValidator_ExpiredToken(t *testing.T) {
	key, jwk := generateTestKey(t, "kid-1")
	v, srvURL := newTestValidator(t, jwk)

	claims := validClaims(fmt.Sprintf("%s/realms/test-realm", srvURL))
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	token := signTestJWT(t, key, "kid-1", claims)

	if _, err := v.ValidateToken(token); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestJWKSValidator_WrongIssuer(t *testing.T) {
	key, jwk := generateTestKey(t, "kid-1")
	v, _ := newTestValidator(t, jwk)

	claims := validClaims("https://not-the-right-issuer.example.com/realms/other")
	token := signTestJWT(t, key, "kid-1", claims)

	if _, err := v.ValidateToken(token); err == nil {
		t.Fatal("expected error for wrong issuer, got nil")
	}
}

func TestJWKSValidator_WrongSigningKey(t *testing.T) {
	_, jwk := generateTestKey(t, "kid-1")
	v, srvURL := newTestValidator(t, jwk)

	// Sign with a DIFFERENT key than the one published in the JWKS, but
	// claim the same kid — the signature must fail verification.
	otherKey, _ := generateTestKey(t, "kid-1")
	claims := validClaims(fmt.Sprintf("%s/realms/test-realm", srvURL))
	token := signTestJWT(t, otherKey, "kid-1", claims)

	if _, err := v.ValidateToken(token); err == nil {
		t.Fatal("expected error for token signed with wrong key, got nil")
	}
}

func TestJWKSValidator_MalformedToken(t *testing.T) {
	_, jwk := generateTestKey(t, "kid-1")
	v, _ := newTestValidator(t, jwk)

	if _, err := v.ValidateToken("not-a-jwt"); err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}
