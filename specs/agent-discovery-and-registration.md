---
title: Stateless Agent Discovery and Explicit Worker Registration
description: Discover agents on demand using the caller's own token, then explicitly and persistently register selected agents as Temporal workers
status: implemented
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: agent-discovery-and-registration

## Goal

Replace the background discovery poller (superseded design, specs/agent-discovery.md) with a stateless, per-request discovery call plus an explicit, persisted registration step, so the workflow server never needs its own long-lived credential to the managed agents control plane.

## Acceptance Criteria

- [x] `GET /agents` calls the control plane's `GET /agents` using the caller's own Bearer token (forwarded, not the server's), returning the live catalog. No background poller, no server-held token, no in-memory catalog cache. Verified live against stage: the same Keycloak token this server's own middleware validated was forwarded and accepted by the control plane.
- [x] `POST /agents/{id}/register` persists a registration row (agent id, name, task queue) and starts a Temporal worker for that agent (task queue `agent-<id>`, `invoke-agent` activity). Registering an already-registered agent is idempotent. Verified live and by test.
- [x] `DELETE /agents/{id}/register` removes the persisted registration and stops that agent's Temporal worker. Verified live and by test.
- [x] `GET /agents/registered` lists persisted registrations with worker status (running/stopped), for the UI.
- [x] On server startup, all persisted registrations get their Temporal workers started automatically (no discovery call needed to restore state after a restart). Verified live: restarting the server with 2 leftover persisted rows restarted both workers before the HTTP listener even came up.
- [x] `InvokeAgentInput.Token` exists and flows into `Dispatcher.Dispatch`'s token parameter for the A2A call — the activity-side plumbing is in place and worker Register/Unregister is tested. **Not fully verified**: no Zigflow workflow definition in this repo calls `invoke-agent` yet, so the claim "a workflow execution's own token reaches the A2A call" is unverified end-to-end (no test starts a real execution with a token input and observes it used in a live/mocked A2A call).
- [x] UI: agent list shows discovered (live) vs. registered (persisted) status side by side, with Register/Unregister actions per agent. Builds clean; HTTP wiring (proxy paths) verified via curl; visual rendering not checked (no browser tool available this session).

## Approach

Split into two independently-simple pieces: (1) `internal/agents.Client.ListAgents` takes a token per call instead of being constructed with one, and the `GET /agents` handler forwards the request's own Bearer token — no poller, no diffing, no catalog cache. (2) A new `store.AgentRegistrationStore` (Postgres, mirroring `store.DefinitionStore`) persists registrations; `internal/worker.Supervisor` gains explicit `Register`/`Unregister` methods called directly by the API handlers instead of being fed diffs from a poller, and `main.go` loads persisted registrations at boot to restart their workers. The `invoke-agent` activity's input gains a token field, threaded through from whatever started the workflow execution.

## Affected Modules

- `internal/agents` — drop `Poller` entirely; `Client.ListAgents(ctx, token)` takes the token as a parameter. `Dispatcher.Dispatch` gains a token parameter used for the A2A call instead of a client-held one.
- `internal/worker` — `Supervisor.ApplyDiff` replaced by `Register(agent)` / `Unregister(id)`; `invokeAgentActivity` input struct gains a `Token string` field passed through to `Dispatch`.
- `store` / `store/postgres` — new `AgentRegistrationStore` interface + Postgres implementation + migration for a `agent_registrations` table (id, name, task_queue, registered_at).
- `internal/api` — `GET /agents` (stateless, forwards caller token), `POST /agents/{id}/register`, `DELETE /agents/{id}/register`, `GET /agents/registered`.
- `cmd/workflow-server/main.go` — drop poller wiring; load persisted registrations at boot and call `Supervisor.Register` for each; no more `MANAGED_AGENTS_TOKEN`/`AGENT_DISCOVERY_INTERVAL_SECONDS` env vars (only `MANAGED_AGENTS_API_URL` remains, since there's no server-side credential).
- `ui/src/stores`, `ui/src/views` — `AgentsListView` shows both discovered and registered state; drop the "Force Refresh" action (discovery is now inherently live on every load); add Register/Unregister buttons.

## Test Strategy

- `internal/agents`: unit tests for `Client.ListAgents` with different tokens (no shared state between calls).
- `internal/worker`: `Supervisor.Register`/`Unregister` tests against the real embedded Temporal dev server (as before), replacing the diff-based tests.
- `store/postgres`: registration store CRUD tests against local Postgres (`make db`), mirroring `store/postgres/definitions.go` test conventions.
- `internal/api`: handler tests for register/unregister/list-registered, plus a test that `GET /agents` forwards the caller's token rather than any server-configured one.
- Integration: restart the server with existing registration rows and confirm workers come back without a discovery call.

## Out of Scope

- Token refresh/rotation for long-running executions whose caller token expires mid-flight — the execution's A2A call fails once the passed-in token expires; no retry-with-refreshed-token behavior in this iteration.
- Admin/service-account credential for A2A dispatch as an alternative to per-execution tokens — revisit if the "token flows with the execution" model proves insufficient.
- Multi-tenancy scoping of registrations (single control-plane project/tenant assumed, as before).

## Notes

Supersedes specs/agent-discovery.md: that design's background poller required the server to hold a long-lived control-plane token, but managed-agents tokens are short-lived (~5 min observed TTL) and per-user, so no such token exists to hold. This design removes the poller and makes discovery a thin per-request proxy using whichever token the caller already has, while registration (which needs to persist beyond any one request) is explicit and stores no token at registration time — the token instead flows with each workflow execution that actually needs to call the agent.
