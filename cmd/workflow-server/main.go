// Command workflow-server runs the Zigflow Workflow Server's Publish API:
// validate and store workflow definitions. See
// docs/architecure/zigflow-workflow-server-architecture.md and
// specs/definition-store.md.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zigflow/zigflow/pkg/zigflow"
	"go.temporal.io/sdk/client"

	"github.com/heiko-braun/trex/internal/agents"
	"github.com/heiko-braun/trex/internal/api"
	"github.com/heiko-braun/trex/internal/auth"
	"github.com/heiko-braun/trex/internal/blobstore"
	"github.com/heiko-braun/trex/internal/worker"
	"github.com/heiko-braun/trex/internal/workflowworker"
	"github.com/heiko-braun/trex/store"
	"github.com/heiko-braun/trex/store/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("workflow-server", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	keycloakURL := os.Getenv("KEYCLOAK_URL")
	keycloakRealm := os.Getenv("KEYCLOAK_REALM")
	if keycloakURL == "" || keycloakRealm == "" {
		return fmt.Errorf("KEYCLOAK_URL and KEYCLOAK_REALM environment variables are required")
	}
	keycloakClient := os.Getenv("KEYCLOAK_CLIENT_ID")
	adminRoles := auth.ParseAdminRoles(os.Getenv("KEYCLOAK_ADMIN_ROLES"))

	managedAgentsURL := os.Getenv("MANAGED_AGENTS_API_URL")
	if managedAgentsURL == "" {
		return fmt.Errorf("MANAGED_AGENTS_API_URL environment variable is required")
	}

	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	if minioEndpoint == "" {
		return fmt.Errorf("MINIO_ENDPOINT environment variable is required")
	}
	minioAccessKey := os.Getenv("MINIO_ACCESS_KEY")
	minioSecretKey := os.Getenv("MINIO_SECRET_KEY")
	minioBucket := os.Getenv("MINIO_BUCKET")
	if minioBucket == "" {
		return fmt.Errorf("MINIO_BUCKET environment variable is required")
	}

	if err := postgres.RunMigrations(dbURL); err != nil {
		return err
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	validator := auth.NewJWKSValidator(keycloakURL, keycloakRealm)
	defer validator.Stop()

	defStore := postgres.NewDefinitionStore(pool)
	registrationStore := postgres.NewAgentRegistrationStore(pool)

	temporalAddr := os.Getenv("TEMPORAL_ADDRESS")
	if temporalAddr == "" {
		temporalAddr = "localhost:7233"
	}
	temporalClient, err := client.Dial(client.Options{HostPort: temporalAddr})
	if err != nil {
		return fmt.Errorf("connect to temporal: %w", err)
	}
	defer temporalClient.Close()

	blobs, err := blobstore.NewMinioStore(context.Background(), blobstore.Config{
		Endpoint:  minioEndpoint,
		AccessKey: minioAccessKey,
		SecretKey: minioSecretKey,
		Bucket:    minioBucket,
	})
	if err != nil {
		return fmt.Errorf("connect to minio: %w", err)
	}

	agentSupervisor, err := startAgentSupervisor(temporalClient, managedAgentsURL, registrationStore, blobs)
	if err != nil {
		return err
	}
	defer agentSupervisor.Stop()

	workflowSupervisor, err := startWorkflowSupervisor(temporalClient, defStore)
	if err != nil {
		return err
	}
	defer workflowSupervisor.Stop()

	discoveryClient := agents.NewClient(managedAgentsURL)

	server := api.New(defStore, api.AuthConfig{
		Validator:      validator,
		KeycloakURL:    keycloakURL,
		KeycloakRealm:  keycloakRealm,
		KeycloakClient: keycloakClient,
		AdminRoles:     adminRoles,
	}, discoveryClient, agentSupervisor, registrationStore, workflowSupervisor)

	mux := http.NewServeMux()
	server.Routes(mux)

	slog.Info("workflow-server listening", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

// startAgentSupervisor restarts a worker for every persisted agent
// registration, per specs/agent-discovery-and-registration.md:
// registration state survives restarts without needing a fresh
// discovery call.
func startAgentSupervisor(temporalClient client.Client, managedAgentsURL string, registrations store.AgentRegistrationStore, blobs worker.BlobStore) (*worker.Supervisor, error) {
	dispatcher := agents.NewDispatcher(managedAgentsURL)
	tokens, err := agents.NewAgentctlTokenSource()
	if err != nil {
		return nil, fmt.Errorf("build token source: %w", err)
	}
	supervisor := worker.NewSupervisor(temporalClient, dispatcher, tokens, blobs)

	regs, err := registrations.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list persisted agent registrations: %w", err)
	}
	for _, reg := range regs {
		agent := agents.Agent{ID: reg.AgentID, Name: reg.Name, Enabled: true}
		if err := supervisor.Register(agent); err != nil {
			slog.Error("restart agent worker from persisted registration", "agent", reg.AgentID, "error", err)
			continue
		}
	}
	slog.Info("agent workers restored from persisted registrations", "count", len(regs))

	return supervisor, nil
}

// startWorkflowSupervisor restarts a worker for every currently published
// workflow definition, per specs/workflow-worker-supervisor.md: the most
// recent revision per (tenant, name) becomes runnable again without
// needing a fresh publish.
func startWorkflowSupervisor(temporalClient client.Client, defs store.DefinitionStore) (*workflowworker.Supervisor, error) {
	supervisor := workflowworker.NewSupervisor(temporalClient)

	all, err := defs.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list stored definitions: %w", err)
	}

	// List returns every revision, newest first; keep only the first
	// (most recent) seen per (tenant, name).
	type tenantName struct{ tenant, name string }
	seen := make(map[tenantName]bool, len(all))
	started := 0
	for _, def := range all {
		tn := tenantName{tenant: def.Tenant, name: def.Name}
		if seen[tn] {
			continue
		}
		seen[tn] = true

		if restoreWorkflowWorker(supervisor, def) {
			started++
		}
	}
	slog.Info("workflow workers restored from stored definitions", "count", started)

	return supervisor, nil
}

// restoreWorkflowWorker attempts to start a worker for one stored
// definition, reporting success. zigflow.LoadFromBytes has been observed
// to panic (rather than return an error) on malformed YAML that isn't a
// valid workflow document, so a bad row must not take down server
// startup for every other definition.
func restoreWorkflowWorker(supervisor *workflowworker.Supervisor, def *store.Definition) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic parsing stored definition for worker start", "tenant", def.Tenant, "name", def.Name, "panic", r)
			ok = false
		}
	}()

	doc, err := zigflow.LoadFromBytes([]byte(def.YAML))
	if err != nil {
		slog.Error("parse stored definition for worker start", "tenant", def.Tenant, "name", def.Name, "error", err)
		return false
	}
	if err := supervisor.Register(def.Tenant, def.Name, doc.Document.Namespace, []byte(def.YAML)); err != nil {
		slog.Error("restart workflow worker from stored definition", "tenant", def.Tenant, "name", def.Name, "error", err)
		return false
	}
	return true
}
