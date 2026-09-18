package api

import (
	"bytes"
	"context"
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
	"sync"
	"testing"
	"time"

	"github.com/heiko-braun/trex/internal/auth"
	"github.com/heiko-braun/trex/store"
)

// fakeStore is an in-memory store.DefinitionStore for testing the API layer
// in isolation from Postgres.
type fakeStore struct {
	mu   sync.Mutex
	rows []*store.Definition
}

func (f *fakeStore) Create(_ context.Context, def *store.Definition) (*store.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.rows {
		if existing.Tenant == def.Tenant && existing.Name == def.Name && existing.BuildID == def.BuildID {
			return existing, nil
		}
	}
	copyDef := *def
	f.rows = append(f.rows, &copyDef)
	return &copyDef, nil
}

func (f *fakeStore) GetCurrent(_ context.Context, tenant, name string) (*store.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var latest *store.Definition
	for _, d := range f.rows {
		if d.Tenant == tenant && d.Name == name {
			latest = d
		}
	}
	if latest == nil {
		return nil, store.ErrNotFound
	}
	return latest, nil
}

func (f *fakeStore) List(_ context.Context) ([]*store.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*store.Definition, len(f.rows))
	copy(out, f.rows)
	return out, nil
}

const validWorkflowYAML = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
`

const shellWorkflowYAML = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - runIt:
      run:
        shell:
          command: /bin/sh
          arguments:
            - -c
            - echo hi
`

// testAuth generates an RSA key, serves it as a JWKS, and returns a
// JWKSValidator plus a helper to mint valid bearer tokens against it.
func testAuth(t *testing.T) (*auth.JWKSValidator, func() string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk := auth.JWK{
		Kid: "kid-1",
		Kty: "RSA",
		Alg: "RS256",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(auth.JWKSet{Keys: []auth.JWK{jwk}})
	}))
	t.Cleanup(srv.Close)

	v := auth.NewJWKSValidator(srv.URL, "test-realm")
	t.Cleanup(v.Stop)

	mintToken := func() string {
		header := map[string]string{"alg": "RS256", "typ": "JWT", "kid": "kid-1"}
		claims := map[string]any{
			"sub":                "user-1",
			"preferred_username": "tester",
			"iss":                fmt.Sprintf("%s/realms/test-realm", srv.URL),
			"exp":                time.Now().Add(1 * time.Hour).Unix(),
			"iat":                time.Now().Unix(),
		}
		headerJSON, _ := json.Marshal(header)
		claimsJSON, _ := json.Marshal(claims)
		signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
		h := sha256.New()
		h.Write([]byte(signingInput))
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h.Sum(nil))
		if err != nil {
			t.Fatal(err)
		}
		return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
	}

	return v, mintToken
}

func newTestServer(t *testing.T) (*Server, *http.ServeMux, func() string) {
	v, mintToken := testAuth(t)
	s := New(&fakeStore{}, AuthConfig{Validator: v})
	mux := http.NewServeMux()
	s.Routes(mux)
	return s, mux, mintToken
}

func doPublish(mux *http.ServeMux, token string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/definitions", bytes.NewReader(b))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func doGet(mux *http.ServeMux, token, tenant, name string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/definitions/"+tenant+"/"+name, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestPublish_ValidWorkflow(t *testing.T) {
	_, mux, mintToken := newTestServer(t)

	rec := doPublish(mux, mintToken(), publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp definitionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.BuildID == "" {
		t.Error("expected non-empty BuildID")
	}
	if resp.Status != string(store.StatusPending) {
		t.Errorf("Status = %q, want %q", resp.Status, store.StatusPending)
	}
}

func TestPublish_RejectsShellPolicyViolation(t *testing.T) {
	_, mux, mintToken := newTestServer(t)

	rec := doPublish(mux, mintToken(), publishRequest{Tenant: "acme", Name: "wf", YAML: shellWorkflowYAML})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var resp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !containsAll(resp.Error, "policy violation", "shell") {
		t.Errorf("error message %q does not identify the policy check that failed", resp.Error)
	}
}

func TestPublish_RejectsMissingFields(t *testing.T) {
	_, mux, mintToken := newTestServer(t)

	rec := doPublish(mux, mintToken(), publishRequest{Tenant: "acme", Name: "", YAML: validWorkflowYAML})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPublish_IsIdempotent(t *testing.T) {
	_, mux, mintToken := newTestServer(t)
	token := mintToken()

	first := doPublish(mux, token, publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})
	second := doPublish(mux, token, publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})

	var firstResp, secondResp definitionResponse
	json.Unmarshal(first.Body.Bytes(), &firstResp)
	json.Unmarshal(second.Body.Bytes(), &secondResp)

	if firstResp.BuildID != secondResp.BuildID {
		t.Errorf("expected same BuildID on re-publish, got %q and %q", firstResp.BuildID, secondResp.BuildID)
	}
}

func TestGetCurrent_ReturnsPublished(t *testing.T) {
	_, mux, mintToken := newTestServer(t)
	token := mintToken()
	doPublish(mux, token, publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})

	rec := doGet(mux, token, "acme", "wf")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp definitionResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.YAML != validWorkflowYAML {
		t.Errorf("YAML mismatch")
	}
}

func TestGetCurrent_NotFound(t *testing.T) {
	_, mux, mintToken := newTestServer(t)

	rec := doGet(mux, mintToken(), "acme", "does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAuthConfig_ReachableWithoutToken(t *testing.T) {
	_, mux, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, authConfigPath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestPublish_RejectsMissingToken(t *testing.T) {
	_, mux, _ := newTestServer(t)

	rec := doPublish(mux, "", publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestPublish_RejectsInvalidToken(t *testing.T) {
	_, mux, _ := newTestServer(t)

	rec := doPublish(mux, "not-a-valid-token", publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGetCurrent_RejectsMissingToken(t *testing.T) {
	_, mux, _ := newTestServer(t)

	rec := doGet(mux, "", "acme", "wf")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestList_ReturnsAllNewestFirst(t *testing.T) {
	_, mux, mintToken := newTestServer(t)
	token := mintToken()
	doPublish(mux, token, publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})

	req := httptest.NewRequest(http.MethodGet, "/definitions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp []definitionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(resp))
	}
}

func TestList_RejectsMissingToken(t *testing.T) {
	_, mux, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/definitions", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !bytes.Contains([]byte(s), []byte(sub)) {
			return false
		}
	}
	return true
}
