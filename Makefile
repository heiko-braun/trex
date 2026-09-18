.PHONY: db db-stop db-logs temporal run build test test-integration

DB_CONTAINER := trex-postgres
DB_PORT := 55433
DB_USER := trex
DB_PASSWORD := trex
DB_NAME := trex
DATABASE_URL := postgres://$(DB_USER):$(DB_PASSWORD)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=disable

KEYCLOAK_URL := https://identity-stage.goorange.sixt.com/auth
KEYCLOAK_REALM := SixtEmployees
KEYCLOAK_CLIENT_ID := managed-agents-console

## db: start a local Postgres for development/testing via podman
db:
	podman run -d --name $(DB_CONTAINER) \
		-e POSTGRES_USER=$(DB_USER) \
		-e POSTGRES_PASSWORD=$(DB_PASSWORD) \
		-e POSTGRES_DB=$(DB_NAME) \
		-p $(DB_PORT):5432 \
		docker.io/library/postgres:16-alpine
	@echo "waiting for postgres to accept connections..."
	@until podman exec $(DB_CONTAINER) pg_isready -U $(DB_USER) >/dev/null 2>&1; do sleep 0.5; done
	@echo "postgres ready at $(DATABASE_URL)"

## db-stop: stop and remove the local Postgres container
db-stop:
	podman stop $(DB_CONTAINER)
	podman rm $(DB_CONTAINER)

## db-logs: tail the local Postgres container's logs
db-logs:
	podman logs -f $(DB_CONTAINER)

## temporal: start a local Temporal dev server (UI at :8233)
temporal:
	temporal server start-dev --ui-port 8233

## run: run the workflow-server against the local Postgres (requires `make db` first)
run:
	DATABASE_URL="$(DATABASE_URL)" \
	KEYCLOAK_URL="$(KEYCLOAK_URL)" \
	KEYCLOAK_REALM="$(KEYCLOAK_REALM)" \
	KEYCLOAK_CLIENT_ID="$(KEYCLOAK_CLIENT_ID)" \
	go run ./cmd/workflow-server

## build: compile all packages
build:
	go build ./...

## test: run unit tests (no external dependencies required)
test:
	go test ./...

## test-integration: run integration tests against the local Postgres (requires `make db` first)
test-integration:
	TEST_DATABASE_URL="$(DATABASE_URL)" go test -tags=integration ./...
