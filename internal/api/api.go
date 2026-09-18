// Package api implements the Publish API described in
// docs/architecure/zigflow-workflow-server-architecture.md: validate and
// persist workflow definitions, and fetch the current one back.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/heiko-braun/trex/internal/auth"
	"github.com/heiko-braun/trex/internal/buildid"
	"github.com/heiko-braun/trex/internal/validator"
	"github.com/heiko-braun/trex/store"
)

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

// Server wires the Publish API's HTTP handlers to a definition store.
type Server struct {
	store store.DefinitionStore
	auth  AuthConfig
}

// New creates a Server backed by the given definition store and auth config.
func New(s store.DefinitionStore, authCfg AuthConfig) *Server {
	return &Server{store: s, auth: authCfg}
}

const authConfigPath = "/api/v1/auth/config"

// Routes registers this server's handlers on mux, protecting every route
// except GET /api/v1/auth/config with the Keycloak JWT auth middleware.
func (s *Server) Routes(mux *http.ServeMux) {
	protected := http.NewServeMux()
	protected.HandleFunc("POST /definitions", s.handlePublish)
	protected.HandleFunc("GET /definitions", s.handleList)
	protected.HandleFunc("GET /definitions/{tenant}/{name}", s.handleGetCurrent)

	middleware := auth.Middleware(auth.MiddlewareConfig{
		Validator:   s.auth.Validator,
		AdminRoles:  s.auth.AdminRoles,
		ExemptPaths: []string{authConfigPath},
	})

	mux.HandleFunc("GET "+authConfigPath, s.handleAuthConfig)
	mux.Handle("/", middleware(protected))
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
	Tenant  string `json:"tenant"`
	Name    string `json:"name"`
	BuildID string `json:"buildId"`
	YAML    string `json:"yaml"`
	Status  string `json:"status"`
}

func toResponse(d *store.Definition) definitionResponse {
	return definitionResponse{
		Tenant:  d.Tenant,
		Name:    d.Name,
		BuildID: d.BuildID,
		YAML:    d.YAML,
		Status:  string(d.Status),
	}
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
