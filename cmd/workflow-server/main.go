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

	"github.com/heiko-braun/trex/internal/api"
	"github.com/heiko-braun/trex/internal/auth"
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
	server := api.New(defStore, api.AuthConfig{
		Validator:      validator,
		KeycloakURL:    keycloakURL,
		KeycloakRealm:  keycloakRealm,
		KeycloakClient: keycloakClient,
		AdminRoles:     adminRoles,
	})

	mux := http.NewServeMux()
	server.Routes(mux)

	slog.Info("workflow-server listening", "addr", addr)
	return http.ListenAndServe(addr, mux)
}
