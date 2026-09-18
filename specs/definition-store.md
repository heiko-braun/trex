---
title: Definition Store
description: Publish, validate, and fetch Zigflow workflow definitions via HTTP API backed by Postgres
status: implemented
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: Definition Store

## Goal

First vertical slice of the Zigflow Workflow Server (see
`docs/architecure/zigflow-workflow-server-architecture.md`): persist
validated workflow definitions with a content-addressed Build ID, so later
specs (worker supervisor, rollout coordinator) have something real to poll.
No Temporal worker lifecycle in this slice — definitions are stored as
`pending`, nothing runs them yet.

## Acceptance Criteria

- [x] `POST /definitions` accepts `{tenant, name, yaml}`, runs it through the
      Validator (JSON Schema + zigflow's own `ValidateBytes` + policy check
      rejecting `run: shell`/`run: script`), computes the Build ID, and
      writes a `pending` row. Re-publishing byte-identical YAML for the same
      tenant/name returns the existing row (idempotent), not a duplicate.
- [x] `GET /definitions/{tenant}/{name}` returns the most recently published
      definition (YAML + Build ID + status) for that tenant/name, 404 if none
      exists.
- [x] Schema-invalid YAML, non-deterministic-expression errors (via zigflow's
      `ErrNonDeterministicExpression`), and YAML containing `run: shell` or
      `run: script` anywhere in the task tree are all rejected with a 400 and
      a message identifying which check failed.
- [x] Build ID = first 12 hex chars of SHA-256 over normalized YAML bytes +
      the pinned zigflow module version string, matching the architecture
      doc's versioning model.
- [x] Postgres schema and migrations follow the `golang-migrate/migrate/v4`
      convention used in `com.sixt.service.managed-agents/store/postgres/migrations/`
      (numbered `.up.sql`/`.down.sql` pairs).

## Approach

Three packages: `internal/validator` (wraps `zigflow.ValidateBytes` +
`zigflow.LoadFromBytes`, walks the returned `*model.Workflow`'s task tree
recursively for `RunTask.Shell`/`RunTask.Script`), `store/postgres`
(definitions table + golang-migrate migrations, mirroring the managed-agents
repo's layout), and `cmd/workflow-server` (stdlib `net/http` + `ServeMux`
pattern routing, `pgx/v5` pool). No framework, no ORM — matches the
"pinned versions, thin adapters" philosophy in the architecture doc.

## Affected Modules

- `internal/validator/` (new) — schema + zigflow + policy validation; the
  seam every later spec's Publish API call goes through unchanged.
- `store/postgres/` (new) — `definitions` table, migrations, `Store`
  interface (`Create`, `GetCurrent`) and its Postgres implementation.
- `cmd/workflow-server/` (new) — HTTP entrypoint wiring validator + store.
- `go.mod` (new) — adds `github.com/zigflow/zigflow` (Apache-2.0),
  `github.com/open-workflow-specification/sdk-go/v4`,
  `github.com/golang-migrate/migrate/v4`, `github.com/jackc/pgx/v5`.

## Test Strategy

- Unit tests on `internal/validator` covering: valid workflow accepted,
  schema-invalid YAML rejected, non-deterministic expression rejected,
  `run: shell` rejected, `run: script` rejected (nested under `do`/`for`/
  `try`/`fork` to prove the walk is recursive, not top-level-only).
- Integration tests on `store/postgres` against a local Postgres (via
  `podman run postgres`, per this project's testing convention) covering:
  create, idempotent re-publish (same YAML -> same row, no duplicate),
  fetch-current, fetch-missing (404-equivalent store error).
- End-to-end test: start `cmd/workflow-server` against the local Postgres,
  `POST` a real workflow YAML (e.g. this repo's
  `workflows/pod-memory-trend.workflow.yaml`, sans any `run: shell` steps —
  or a trimmed fixture proving the policy rejection path), `GET` it back,
  assert Build ID matches independently recomputed hash.

## Out of Scope

- Worker supervisor, Zigflow adapter, Temporal worker registration — no
  workflow defined here ever actually runs (separate spec).
- Rollout coordinator, Current/Ramping/Retired state transitions — this spec
  only ever writes `pending`; status flips belong with the coordinator.
- List/history endpoints, explicit retire endpoint — deferred until a
  coordinator exists to make "current vs. retired" meaningful.
- Multi-tenancy enforcement beyond a `tenant` column (no auth/namespacing
  yet — every caller can publish/read any tenant's definitions).
- `call: activity`, `call: http`/`grpc` runtime policy (egress restrictions,
  credential scoping) — this spec only checks for `run: shell`/`run: script`.

## Notes

Confirmed via the zigflow GitHub org (`github.com/zigflow/zigflow`,
Apache-2.0) that `pkg/zigflow` exports `ValidateBytes(data []byte) error` and
`LoadFromBytes(data []byte) (*model.Workflow, error)` — real, stable-enough
public API, not an internal package. `model.Workflow` comes from
`github.com/open-workflow-specification/sdk-go/v4/model`, whose `RunTask.
RunTaskConfiguration` has typed `Shell *Shell` / `Script *Script` fields,
making the policy check a straightforward tree walk rather than a YAML
string/regex scan.

### Implementation notes

- zigflow requires Go 1.27.1; `go.mod`'s `go` directive was bumped
  accordingly and `GOTOOLCHAIN=auto` downloads it transparently.
- Build ID's zigflow-version input is read at runtime via
  `runtime/debug.ReadBuildInfo()` (`internal/buildid`), not hardcoded, so it
  can never drift from what `go.mod` actually pins.
- `store/definitions.go` defines the `DefinitionStore` interface;
  `store/postgres/definitions.go` implements it — same split as
  `com.sixt.service.managed-agents/store/agent.go` vs. `store/postgres/agent.go`.
- Idempotent `Create` uses `INSERT ... ON CONFLICT (tenant, name, build_id)
  DO NOTHING RETURNING ...`, falling back to a plain `SELECT` of the
  existing row when the insert is a no-op (no row returned).
- All 5 criteria verified against a real compiled `cmd/workflow-server`
  binary running against a real (podman) Postgres, using this repo's actual
  `workflows/pod-memory-trend.workflow.yaml` to exercise the `run: shell`
  rejection path end-to-end, and an independently-recomputed SHA-256 to
  confirm the Build ID formula.
- Test suite: `go test ./...` (unit, no external deps) and
  `go test -tags=integration ./...` (requires `TEST_DATABASE_URL`) both
  pass.
