// Package api implements the Publish API described in
// docs/architecure/zigflow-workflow-server-architecture.md: validate and
// persist workflow definitions, and fetch the current one back.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zigflow/zigflow/pkg/zigflow"

	"github.com/heiko-braun/trex/internal/agents"
	"github.com/heiko-braun/trex/internal/auth"
	"github.com/heiko-braun/trex/internal/buildid"
	"github.com/heiko-braun/trex/internal/manifest"
	"github.com/heiko-braun/trex/internal/validator"
	"github.com/heiko-braun/trex/internal/worker"
	"github.com/heiko-braun/trex/store"
)

// envelopeTenant is hardcoded for this slice, matching
// internal/worker/activity.go's own hardcoded tenant: no multi-tenant
// wiring exists yet anywhere in the workflow input or activity input.
const envelopeTenant = "platform"

// AuthConfig holds the Keycloak settings this server hands back to clients
// via GET /api/v1/auth/config, and the values used to build the auth
// middleware.
type AuthConfig struct {
	Validator      *auth.JWKSValidator
	KeycloakURL    string
	KeycloakRealm  string
	KeycloakClient string
	AdminRoles     []string
}

// AgentDiscoverer fetches the live agent catalog from the managed agents
// control plane using the caller's own token, per request. Implemented by
// *agents.Client.
type AgentDiscoverer interface {
	ListAgents(ctx context.Context, token string) ([]agents.Agent, error)
}

// AgentSupervisor starts/stops Temporal workers for registered agents and
// reports their status. Implemented by *worker.Supervisor.
type AgentSupervisor interface {
	Register(agent agents.Agent) error
	Unregister(slug string)
	Statuses() []worker.Status
}

// WorkflowSupervisor starts a Temporal worker for a published workflow
// definition so it can actually execute, replacing any worker already
// running for the same (tenant, name). Implemented by
// *workflowworker.Supervisor.
type WorkflowSupervisor interface {
	Register(tenant, name, taskQueue string, yamlBytes []byte) error
}

// EnvelopeStore reads task envelope data written by the
// write-envelope-index activity, per specs/task-envelope-browser.md.
// Implemented by *blobstore.MinioStore.
type EnvelopeStore interface {
	GetIndex(ctx context.Context, tenant, workflowID string) (map[string]manifest.Ref, error)
	Get(ctx context.Context, tenant string, ref manifest.Ref) ([]byte, error)
}

// Server wires the Publish API's HTTP handlers to a definition store.
type Server struct {
	store          store.DefinitionStore
	auth           AuthConfig
	discoverer     AgentDiscoverer
	supervisor     AgentSupervisor
	registrations  store.AgentRegistrationStore
	workflowWorker WorkflowSupervisor
	envelopes      EnvelopeStore
}

// New creates a Server backed by the given definition store, discovery
// client, agent worker supervisor, agent registration store, workflow
// worker supervisor, and envelope store.
func New(s store.DefinitionStore, authCfg AuthConfig, discoverer AgentDiscoverer, supervisor AgentSupervisor, registrations store.AgentRegistrationStore, workflowWorker WorkflowSupervisor, envelopes EnvelopeStore) *Server {
	return &Server{store: s, auth: authCfg, discoverer: discoverer, supervisor: supervisor, registrations: registrations, workflowWorker: workflowWorker, envelopes: envelopes}
}

const authConfigPath = "/api/v1/auth/config"

// Routes registers this server's handlers on mux, protecting every route
// except GET /api/v1/auth/config with the Keycloak JWT auth middleware.
func (s *Server) Routes(mux *http.ServeMux) {
	protected := http.NewServeMux()
	protected.HandleFunc("POST /definitions", s.handlePublish)
	protected.HandleFunc("GET /definitions", s.handleList)
	protected.HandleFunc("GET /definitions/{tenant}/{name}", s.handleGetCurrent)
	protected.HandleFunc("GET /agents", s.handleDiscoverAgents)
	protected.HandleFunc("GET /agents/registered", s.handleListRegisteredAgents)
	protected.HandleFunc("POST /agents/{id}/register", s.handleRegisterAgent)
	protected.HandleFunc("DELETE /agents/{id}/register", s.handleUnregisterAgent)
	protected.HandleFunc("GET /envelopes/{workflowID}", s.handleGetEnvelope)
	protected.HandleFunc("GET /envelopes/{workflowID}/{digest}", s.handleGetEnvelopeBlob)

	middleware := auth.Middleware(auth.MiddlewareConfig{
		Validator:   s.auth.Validator,
		AdminRoles:  s.auth.AdminRoles,
		ExemptPaths: []string{authConfigPath},
	})

	mux.HandleFunc("GET "+authConfigPath, s.handleAuthConfig)
	mux.Handle("/", recoverMiddleware(middleware(protected)))
}

// recoverMiddleware turns a panic in any handler into a 500 instead of
// crashing the process. zigflow.LoadFromBytes (called by validator.Validate
// on every publish, and by the workflow worker supervisor) has been
// observed to panic rather than return an error on some malformed input,
// so one bad request must not take down every other in-flight request.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic handling request", "path", r.URL.Path, "panic", rec)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type authConfigResponse struct {
	KeycloakURL    string   `json:"keycloak_url"`
	KeycloakRealm  string   `json:"keycloak_realm"`
	KeycloakClient string   `json:"keycloak_client_id"`
	AdminRoles     []string `json:"admin_roles"`
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, authConfigResponse{
		KeycloakURL:    s.auth.KeycloakURL,
		KeycloakRealm:  s.auth.KeycloakRealm,
		KeycloakClient: s.auth.KeycloakClient,
		AdminRoles:     s.auth.AdminRoles,
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	defs, err := s.store.List(r.Context())
	if err != nil {
		slog.Error("list definitions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]definitionResponse, 0, len(defs))
	for _, d := range defs {
		resp = append(resp, toResponse(d))
	}
	writeJSON(w, http.StatusOK, resp)
}

type publishRequest struct {
	Tenant string `json:"tenant"`
	Name   string `json:"name"`
	YAML   string `json:"yaml"`
}

type definitionResponse struct {
	Tenant    string    `json:"tenant"`
	Name      string    `json:"name"`
	BuildID   string    `json:"buildId"`
	YAML      string    `json:"yaml"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

func toResponse(d *store.Definition) definitionResponse {
	return definitionResponse{
		Tenant:    d.Tenant,
		Name:      d.Name,
		BuildID:   d.BuildID,
		YAML:      d.YAML,
		Status:    string(d.Status),
		CreatedAt: d.CreatedAt,
	}
}

type discoveredAgentResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type registeredAgentResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TaskQueue string `json:"taskQueue"`
	Running   bool   `json:"running"`
}

// bearerToken extracts the raw Bearer token from the request's own
// Authorization header, so it can be forwarded to the managed agents
// control plane as-is: discovery is stateless and uses whichever
// credential the caller already has, not one the server holds.
func bearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func (s *Server) handleDiscoverAgents(w http.ResponseWriter, r *http.Request) {
	discovered, err := s.discoverer.ListAgents(r.Context(), bearerToken(r))
	if err != nil {
		slog.Error("discover agents", "error", err)
		writeError(w, http.StatusBadGateway, "discover agents: "+err.Error())
		return
	}

	resp := make([]discoveredAgentResponse, 0, len(discovered))
	for _, a := range discovered {
		resp = append(resp, discoveredAgentResponse{ID: a.ID, Name: a.Name, Type: a.Type})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListRegisteredAgents(w http.ResponseWriter, r *http.Request) {
	statuses := s.supervisor.Statuses()
	resp := make([]registeredAgentResponse, 0, len(statuses))
	for _, st := range statuses {
		resp = append(resp, registeredAgentResponse{
			ID:        st.Agent.ID,
			Name:      st.Agent.Name,
			TaskQueue: st.TaskQueue,
			Running:   st.Running,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

type registerAgentRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	var req registerAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	agent := agents.Agent{ID: agentID, Name: req.Name, Enabled: true}

	if err := s.supervisor.Register(agent); err != nil {
		slog.Error("register agent worker", "agent", agentID, "error", err)
		writeError(w, http.StatusInternalServerError, "register agent worker: "+err.Error())
		return
	}

	if _, err := s.registrations.Register(r.Context(), &store.AgentRegistration{
		AgentID:   agentID,
		Name:      req.Name,
		TaskQueue: agent.TaskQueue(),
	}); err != nil {
		slog.Error("persist agent registration", "agent", agentID, "error", err)
		writeError(w, http.StatusInternalServerError, "persist agent registration: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, registeredAgentResponse{
		ID:        agentID,
		Name:      req.Name,
		TaskQueue: agent.TaskQueue(),
		Running:   true,
	})
}

func (s *Server) handleUnregisterAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	s.supervisor.Unregister(agentID)

	if err := s.registrations.Unregister(r.Context(), agentID); err != nil {
		slog.Error("delete agent registration", "agent", agentID, "error", err)
		writeError(w, http.StatusInternalServerError, "delete agent registration: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetEnvelope returns the slot -> ref index written by the
// write-envelope-index activity for one workflow execution ID, per
// specs/task-envelope-browser.md.
func (s *Server) handleGetEnvelope(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("workflowID")

	slots, err := s.envelopes.GetIndex(r.Context(), envelopeTenant, workflowID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no envelope found for workflow "+workflowID)
		return
	}

	writeJSON(w, http.StatusOK, slots)
}

// handleGetEnvelopeBlob returns the raw content of one blob referenced by
// workflowID's envelope, identified by its digest (e.g.
// "sha256:44dd..."). The digest must belong to a ref actually listed in
// that workflow's envelope index — this endpoint does not allow fetching
// arbitrary digests outside the envelope it was asked about.
func (s *Server) handleGetEnvelopeBlob(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("workflowID")
	digest := r.PathValue("digest")

	slots, err := s.envelopes.GetIndex(r.Context(), envelopeTenant, workflowID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no envelope found for workflow "+workflowID)
		return
	}

	var ref *manifest.Ref
	for _, slotRef := range slots {
		if slotRef.Digest == digest {
			r := slotRef
			ref = &r
			break
		}
	}
	if ref == nil {
		writeError(w, http.StatusNotFound, "digest "+digest+" is not part of workflow "+workflowID+"'s envelope")
		return
	}

	content, err := s.envelopes.Get(r.Context(), envelopeTenant, *ref)
	if err != nil {
		slog.Error("get envelope blob", "workflow", workflowID, "digest", digest, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	contentType := ref.MediaType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Tenant == "" || req.Name == "" || req.YAML == "" {
		writeError(w, http.StatusBadRequest, "tenant, name, and yaml are all required")
		return
	}

	yamlBytes := []byte(req.YAML)

	if err := validator.Validate(yamlBytes); err != nil {
		writeError(w, http.StatusBadRequest, classifyValidationError(err))
		return
	}

	id, err := buildid.Compute(yamlBytes)
	if err != nil {
		slog.Error("compute build id", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	def := &store.Definition{
		Tenant:  req.Tenant,
		Name:    req.Name,
		BuildID: id,
		YAML:    req.YAML,
		Status:  store.StatusPending,
	}

	created, err := s.store.Create(r.Context(), def)
	if err != nil {
		slog.Error("create definition", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	doc, err := zigflow.LoadFromBytes(yamlBytes)
	if err != nil {
		// Already passed validator.Validate above, so this would only
		// fail on a Zigflow-internal inconsistency; the definition is
		// stored either way, just not yet runnable.
		slog.Error("parse published definition for worker start", "tenant", req.Tenant, "name", req.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "stored but failed to start worker: "+err.Error())
		return
	}
	if err := s.workflowWorker.Register(req.Tenant, req.Name, doc.Document.Namespace, yamlBytes); err != nil {
		slog.Error("start workflow worker", "tenant", req.Tenant, "name", req.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "stored but failed to start worker: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toResponse(created))
}

func (s *Server) handleGetCurrent(w http.ResponseWriter, r *http.Request) {
	tenant := r.PathValue("tenant")
	name := r.PathValue("name")

	def, err := s.store.GetCurrent(r.Context(), tenant, name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no definition found for "+tenant+"/"+name)
		return
	}
	if err != nil {
		slog.Error("get current definition", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(def))
}

// classifyValidationError turns a validation error into a message that
// names which check failed, per the spec's acceptance criteria.
func classifyValidationError(err error) string {
	if pv, ok := validator.AsPolicyViolation(err); ok {
		return "policy violation: " + pv.Error()
	}
	if validator.IsNonDeterministic(err) {
		return "non-deterministic expression: " + err.Error()
	}
	if validator.IsSchemaInvalid(err) {
		return "schema validation failed: " + err.Error()
	}
	return "validation failed: " + err.Error()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
