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
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/heiko-braun/trex/internal/agents"
	"github.com/heiko-braun/trex/internal/auth"
	"github.com/heiko-braun/trex/internal/worker"
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

// fakeDiscoverer is an in-memory AgentDiscoverer for testing the
// discovery endpoint in isolation from the real control plane. It
// records the token passed to ListAgents so tests can assert the
// caller's own token is forwarded, not any server-held credential.
type fakeDiscoverer struct {
	result    []agents.Agent
	err       error
	lastToken string
}

func (f *fakeDiscoverer) ListAgents(_ context.Context, token string) ([]agents.Agent, error) {
	f.lastToken = token
	return f.result, f.err
}

// fakeSupervisor is an in-memory AgentSupervisor for testing the
// register/unregister endpoints in isolation from the real Temporal
// worker supervisor.
type fakeSupervisor struct {
	mu          sync.Mutex
	statuses    map[string]worker.Status
	registerErr error
}

func newFakeSupervisor() *fakeSupervisor {
	return &fakeSupervisor{statuses: map[string]worker.Status{}}
}

func (f *fakeSupervisor) Register(agent agents.Agent) error {
	if f.registerErr != nil {
		return f.registerErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[agent.Slug()] = worker.Status{Agent: agent, TaskQueue: agent.TaskQueue(), Running: true}
	return nil
}

func (f *fakeSupervisor) Unregister(slug string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.statuses, slug)
}

func (f *fakeSupervisor) Statuses() []worker.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]worker.Status, 0, len(f.statuses))
	for _, st := range f.statuses {
		out = append(out, st)
	}
	return out
}

// fakeRegistrationStore is an in-memory store.AgentRegistrationStore for
// testing the API layer in isolation from Postgres.
type fakeRegistrationStore struct {
	mu   sync.Mutex
	rows map[string]*store.AgentRegistration
}

func newFakeRegistrationStore() *fakeRegistrationStore {
	return &fakeRegistrationStore{rows: map[string]*store.AgentRegistration{}}
}

func (f *fakeRegistrationStore) Register(_ context.Context, reg *store.AgentRegistration) (*store.AgentRegistration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.rows[reg.AgentID]; ok {
		return existing, nil
	}
	copyReg := *reg
	f.rows[reg.AgentID] = &copyReg
	return &copyReg, nil
}

func (f *fakeRegistrationStore) Unregister(_ context.Context, agentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, agentID)
	return nil
}

func (f *fakeRegistrationStore) List(_ context.Context) ([]*store.AgentRegistration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*store.AgentRegistration, 0, len(f.rows))
	for _, r := range f.rows {
		out = append(out, r)
	}
	return out, nil
}

// fakeWorkflowSupervisor is an in-memory WorkflowSupervisor for testing
// the publish endpoint in isolation from the real Temporal worker
// supervisor.
type fakeWorkflowSupervisor struct {
	mu            sync.Mutex
	registerErr   error
	panicRegister bool
	calls         []fakeWorkflowRegisterCall
}

type fakeWorkflowRegisterCall struct {
	tenant, name, taskQueue string
}

func (f *fakeWorkflowSupervisor) Register(tenant, name, taskQueue string, _ []byte) error {
	if f.panicRegister {
		panic("simulated zigflow panic")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeWorkflowRegisterCall{tenant: tenant, name: name, taskQueue: taskQueue})
	return f.registerErr
}

func newTestServer(t *testing.T) (*Server, *http.ServeMux, func() string) {
	v, mintToken := testAuth(t)
	s := New(&fakeStore{}, AuthConfig{Validator: v}, &fakeDiscoverer{}, newFakeSupervisor(), newFakeRegistrationStore(), &fakeWorkflowSupervisor{})
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

func TestPublish_StartsWorkflowWorker(t *testing.T) {
	v, mintToken := testAuth(t)
	wfSupervisor := &fakeWorkflowSupervisor{}
	s := New(&fakeStore{}, AuthConfig{Validator: v}, &fakeDiscoverer{}, newFakeSupervisor(), newFakeRegistrationStore(), wfSupervisor)
	mux := http.NewServeMux()
	s.Routes(mux)

	rec := doPublish(mux, mintToken(), publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	wfSupervisor.mu.Lock()
	defer wfSupervisor.mu.Unlock()
	if len(wfSupervisor.calls) != 1 {
		t.Fatalf("workflow supervisor Register called %d times, want 1", len(wfSupervisor.calls))
	}
	call := wfSupervisor.calls[0]
	if call.tenant != "acme" || call.name != "wf" || call.taskQueue != "test" {
		t.Errorf("Register call = %+v, want tenant=acme name=wf taskQueue=test", call)
	}
}

func TestPublish_WorkflowWorkerStartFailureIsSurfaced(t *testing.T) {
	v, mintToken := testAuth(t)
	wfSupervisor := &fakeWorkflowSupervisor{registerErr: errors.New("temporal unreachable")}
	s := New(&fakeStore{}, AuthConfig{Validator: v}, &fakeDiscoverer{}, newFakeSupervisor(), newFakeRegistrationStore(), wfSupervisor)
	mux := http.NewServeMux()
	s.Routes(mux)

	rec := doPublish(mux, mintToken(), publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
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

func TestDiscoverAgents_ForwardsCallersToken(t *testing.T) {
	v, mintToken := testAuth(t)
	discoverer := &fakeDiscoverer{result: []agents.Agent{{ID: "a1", Name: "agent-one", Type: "coding"}}}
	s := New(&fakeStore{}, AuthConfig{Validator: v}, discoverer, newFakeSupervisor(), newFakeRegistrationStore(), &fakeWorkflowSupervisor{})
	mux := http.NewServeMux()
	s.Routes(mux)

	token := mintToken()
	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if discoverer.lastToken != token {
		t.Errorf("ListAgents called with token %q, want the caller's own token %q", discoverer.lastToken, token)
	}
	var resp []discoveredAgentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 || resp[0].ID != "a1" {
		t.Fatalf("resp = %+v, want one agent with id a1", resp)
	}
}

func TestDiscoverAgents_PropagatesControlPlaneError(t *testing.T) {
	v, mintToken := testAuth(t)
	discoverer := &fakeDiscoverer{err: errors.New("control plane unreachable")}
	s := New(&fakeStore{}, AuthConfig{Validator: v}, discoverer, newFakeSupervisor(), newFakeRegistrationStore(), &fakeWorkflowSupervisor{})
	mux := http.NewServeMux()
	s.Routes(mux)

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	req.Header.Set("Authorization", "Bearer "+mintToken())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestDiscoverAgents_RejectsMissingToken(t *testing.T) {
	_, mux, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRegisterAgent_StartsWorkerAndPersists(t *testing.T) {
	_, mux, mintToken := newTestServer(t)

	body, _ := json.Marshal(registerAgentRequest{Name: "agent-one"})
	req := httptest.NewRequest(http.MethodPost, "/agents/a1/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+mintToken())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp registeredAgentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != "a1" || !resp.Running {
		t.Errorf("resp = %+v, want id=a1 running=true", resp)
	}
}

func TestRegisterAgent_IsIdempotent(t *testing.T) {
	_, mux, mintToken := newTestServer(t)
	token := mintToken()

	body, _ := json.Marshal(registerAgentRequest{Name: "agent-one"})
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/agents/a1/register", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("register #%d: status = %d, want %d; body: %s", i+1, rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/agents/registered", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var resp []registeredAgentResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("registered agents = %d, want 1 (duplicate register should be idempotent)", len(resp))
	}
}

func TestRegisterAgent_RejectsMissingName(t *testing.T) {
	_, mux, mintToken := newTestServer(t)

	body, _ := json.Marshal(registerAgentRequest{})
	req := httptest.NewRequest(http.MethodPost, "/agents/a1/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+mintToken())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUnregisterAgent_StopsWorkerAndRemovesPersisted(t *testing.T) {
	_, mux, mintToken := newTestServer(t)
	token := mintToken()

	body, _ := json.Marshal(registerAgentRequest{Name: "agent-one"})
	req := httptest.NewRequest(http.MethodPost, "/agents/a1/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(httptest.NewRecorder(), req)

	delReq := httptest.NewRequest(http.MethodDelete, "/agents/a1/register", nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := httptest.NewRecorder()
	mux.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", delRec.Code, http.StatusNoContent, delRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/agents/registered", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	var resp []registeredAgentResponse
	json.Unmarshal(listRec.Body.Bytes(), &resp)
	if len(resp) != 0 {
		t.Fatalf("registered agents = %d, want 0 after unregister", len(resp))
	}
}

func TestListRegisteredAgents_RejectsMissingToken(t *testing.T) {
	_, mux, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/agents/registered", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestPublish_SurvivesPanicInWorkflowSupervisor(t *testing.T) {
	v, mintToken := testAuth(t)
	wfSupervisor := &fakeWorkflowSupervisor{panicRegister: true}
	s := New(&fakeStore{}, AuthConfig{Validator: v}, &fakeDiscoverer{}, newFakeSupervisor(), newFakeRegistrationStore(), wfSupervisor)
	mux := http.NewServeMux()
	s.Routes(mux)

	token := mintToken()
	rec := doPublish(mux, token, publishRequest{Tenant: "acme", Name: "wf", YAML: validWorkflowYAML})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	// The mux (and process) must still be alive for the next request.
	rec2 := doGet(mux, token, "acme", "does-not-exist")
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("server did not survive the panic: status = %d, want %d", rec2.Code, http.StatusNotFound)
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
