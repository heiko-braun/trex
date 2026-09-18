---
title: Zigflow Adapter and Workflow Worker Supervisor (unversioned first pass)
description: Build and run a Temporal worker for each published workflow definition, so publishing actually executes workflows instead of just validating and storing YAML
status: proposed
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: workflow-worker-supervisor

## Goal

Close the gap between the Publish API (validate + store YAML) and actually running workflows: on publish, build the Zigflow closure tree from stored YAML and start a Temporal worker for it, so `temporal workflow start` against a published definition's task queue executes instead of finding no poller. First of three specs implementing docs/architecure/zigflow-workflow-server-architecture.md's Worker supervisor (this one, unversioned); Build-ID/Worker-Deployment versioning and the rollout coordinator follow as separate specs once this unblocks running workflows at all.

## Acceptance Criteria

- [ ] A new `internal/zigflowadapter` package wraps `zigflow.ValidateBytes` -> `zigflow.LoadFromBytes` -> `zigflow.NewWorkflow(temporalWorker, doc, ...)` behind a single `Build(temporalWorker worker.Worker, yamlBytes []byte) error` seam, isolating the rest of the server from Zigflow's Go API per the architecture doc's stated adapter boundary.
- [ ] A `internal/workflowworker.Supervisor` starts one plain Temporal worker per (tenant, name) on the definition's own `taskQueue` (from the YAML's `document.taskQueue`), calling the adapter to register the workflow on it. No Build ID, no `UseVersioning`, no Worker Deployment API calls yet.
- [ ] On workflow-server startup, the supervisor loads every current definition (`DefinitionStore.List` filtered to latest per tenant/name) and starts a worker for each.
- [ ] Publishing a new revision for an existing (tenant, name) replaces that pair's worker: stop the old one, start a new one on the freshly built closure tree.
- [ ] The demo (`workflows/pod-memory-trend.workflow.yaml`) can be published via the Publish API and then actually executed with `temporal workflow start --task-queue pod-memory-trend --type pod-memory-trend`, without a separately-run `zigflow run` process.

## Approach

`internal/zigflowadapter` provides the thin `Build` seam calling Zigflow's confirmed Go API (`zigflow.ValidateBytes` -> `zigflow.LoadFromBytes` -> `zigflow.NewWorkflow`, the last of which internally calls `worker.Worker.RegisterWorkflowWithOptions`). `internal/workflowworker.Supervisor` mirrors `internal/worker.Supervisor`'s shape (a map of running `worker.Worker` keyed by tenant/name, `Register`/`Unregister`-style methods, `Stop()` for shutdown) but keyed by workflow identity instead of agent slug, and is driven by the Publish API's `handlePublish` calling it directly after a successful `Create`, plus a boot-time load from the store.

## Affected Modules

- `internal/zigflowadapter` (new) — `Build(w worker.Worker, yamlBytes []byte) error`; only this package imports `github.com/zigflow/zigflow/pkg/zigflow`'s `NewWorkflow`/`LoadFromBytes` beyond what `internal/validator` already does for validation.
- `internal/workflowworker` (new) — `Supervisor` with `Register(tenant, name, taskQueue string, yamlBytes []byte) error` / `Unregister(tenant, name string)` / `Statuses() []Status`, analogous to `internal/worker.Supervisor`.
- `internal/api` — `handlePublish` calls `workflowworker.Supervisor.Register` after a successful `store.Create`, so a publish that fails to start its worker is surfaced as a 500 rather than silently stored-but-unrunnable.
- `cmd/workflow-server/main.go` — construct the workflow-worker `Supervisor`, load existing definitions from the store at boot (mirrors the agent-registration restore-on-boot pattern), wire into `api.New`.

## Test Strategy

- `internal/zigflowadapter`: unit test that `Build` against the repo's own `workflows/pod-memory-trend.workflow.yaml` succeeds and registers a workflow type on a `worker.Worker` (assert via a real embedded Temporal dev server, same pattern as `internal/worker/supervisor_test.go`).
- `internal/workflowworker`: `Supervisor.Register`/`Unregister`/republish-replaces-worker tests against the embedded dev server.
- `internal/api`: extend `handlePublish` tests to assert a worker starts (via a fake `workflowworker.Supervisor`) on successful publish.
- Manual/integration: publish the demo workflow via the API, `temporal workflow start` against its task queue, confirm it reaches `askOpsBuddy`'s `invoke-agent` activity (registered agents from specs/agent-discovery-and-registration.md) instead of timing out with no poller.

## Out of Scope

- Build ID computation tied to worker versions, `UseVersioning`, Worker Deployment registration (spec #2).
- Rollout coordinator, leader election, `SetCurrentVersion`, drain tracking, retirement (spec #3).
- Multi-pod convergence/readiness reporting (depends on spec #3's coordinator).
- Changing `store.Definition`'s `Status` beyond `StatusPending` (no "active"/"retired" states yet — this spec runs whatever is most recently published, unconditionally).

## Notes

Unblocks running `workflows/pod-memory-trend.workflow.yaml` (and any future workflow) without a hand-started `zigflow run` process, which the user flagged as not the intended path. Sequenced as step 1 of 3 per the split agreed for docs/architecure/zigflow-workflow-server-architecture.md's Worker supervisor + Zigflow adapter + rollout coordinator design — later specs add versioning and safe rollout on top of the mechanism this one establishes.
