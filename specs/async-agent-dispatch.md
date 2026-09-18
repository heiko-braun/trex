---
title: Async agent dispatch via a separate poll-agent-task activity
description: invoke-agent returns as soon as an A2A task is created instead of blocking on it; a new poll-agent-task activity checks status, so no single call needs a token that outlives it
status: implemented
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: async-agent-dispatch

## Goal

Fix the mechanism behind trex-8hb: `Dispatcher.Dispatch` currently blocks one Temporal activity attempt inside an in-process HTTP poll loop for an A2A task's entire lifetime, reusing one token captured at call-start. Split into two fast activities — `invoke-agent` (send, return immediately) and `poll-agent-task` (check status once) — so every individual call is short-lived and can carry its own fresh token.

## Acceptance Criteria

- [x] `invoke-agent`'s activity sends the A2A message with `configuration.returnImmediately: true` and returns as soon as the control plane responds — either a completed result (`Done: true`) or a `{TaskID}` handle. It never blocks on task completion. Verified live: 92-120ms for a prompt whose task took 100+ seconds to actually finish.
- [x] A new `poll-agent-task` activity takes `{TaskID}`, calls `GET /a2a/{agentID}/tasks/{id}` once, and returns `{Done, Result, Failed, FailureReason}` — one HTTP call, no internal loop. Verified live: ~30-60ms per call, 20 calls to track one real task to completion.
- [x] `workflows/pod-memory-trend.workflow.yaml` calls `invoke-agent`, then loops `call: activity, name: poll-agent-task` with an 8s `wait` between attempts until `Done`, using Zigflow's `for`/`while` constructs. Token: per the follow-up decision (see Notes), each activity fetches its own fresh token server-side via `AgentctlTokenSource` rather than one passed through workflow input.
- [x] Both activities are registered on the same per-agent Temporal worker/task queue (`agent-<id>`) the supervisor already starts.
- [x] Tests green (62 passing). Live run against real agents (Ops Buddy, Log Monitoring on stage) completed successfully end-to-end, `poll-agent-task` visible as its own repeated activity type in Temporal, correct final formatted output produced.

## Approach

Split `agents.Dispatcher` into `Dispatch` (send only, one HTTP call, returns either the direct result or a task handle) and `PollTask` (one status check, one HTTP call) — both already exist as private helpers (`sendMessage`, `getTaskStatus`) inside `dispatch.go` today; this just makes `PollTask` public and removes the `awaitTask` loop that currently glues them together inside a single blocking call. `internal/worker.Supervisor` registers a second activity, `poll-agent-task`, alongside `invoke-agent` on the same worker. The workflow YAML gets an explicit retry/wait loop instead of relying on the activity to hide that wait.

## Affected Modules

- `internal/agents/dispatch.go` — remove `awaitTask`; rename/expose `sendMessage` as `Dispatch` (returns a discriminated result: direct text, or a task handle) and `getTaskStatus` as `PollTask`.
- `internal/worker/activity.go` — `invokeAgentActivity` no longer calls a blocking `Dispatch`; add `pollAgentTaskActivity` calling `PollTask` once. `InvokeAgentInput`/output and new `PollAgentTaskInput`/output types.
- `internal/worker/supervisor.go` — `Register` registers both `invoke-agent` and `poll-agent-task` on the per-agent worker.
- `workflows/pod-memory-trend.workflow.yaml` — add the poll/wait loop around each `invoke-agent` call.
- `internal/worker/*_test.go` — update/add tests for the split activities.

## Test Strategy

- `internal/agents`: unit tests for `Dispatch` (direct-message case, task-handle case) and `PollTask` (done/pending/failed) against a stubbed A2A HTTP server.
- `internal/worker`: activity-level tests (via the embedded Temporal dev server, as existing tests do) that both activities register and can be invoked; a fake `Dispatcher` verifying `invoke-agent` never blocks past one call.
- Manual/live: re-run the pod-memory-trend demo against real agents and confirm `invoke-agent` returns within the timeout of one HTTP call rather than the full task duration, with `poll-agent-task` visible as repeated short activity invocations in the Temporal UI.

## Out of Scope

- Token refresh/exchange mechanisms (trex-8hb's broader discussion) — this spec only shortens each call so a fresh token can be supplied per call; it does not change where that per-call token comes from.
- Changing how the *first* `invoke-agent` call gets authenticated — still the token passed as workflow input.
- Backoff/interval tuning for the poll loop beyond a sensible default (e.g. Zigflow `wait` of a few seconds between polls).

## Notes

Directly implements the fix direction identified in trex-8hb's comment: agentctl's own `a2a send --async`/`--wait` already models this split (create-and-return vs. poll-until-done as separate concerns); this spec brings the same shape into the workflow-server's activity design.

Two things discovered during implementation that shifted scope beyond the original spec:

1. **`message:send` blocks server-side by default.** The A2A protocol's `configuration.returnImmediately` flag (confirmed against `agentctl`'s own SDK usage) has to be explicitly set to `true` on every `Dispatch` call — without it, the control plane holds the HTTP response open until the task finishes server-side (observed: 100+ seconds for a real prompt), which defeats the whole point of splitting dispatch from polling regardless of how short `Dispatch`'s own Go code is.
2. **The token now flows differently than originally planned.** Mid-implementation the user asked for token lookup to move into the worker (shelling out to `agentctl whoami`, which silently refreshes the cached session, then reading its token cache) rather than the workflow passing `callerToken` as input. `InvokeAgentInput`/`PollAgentTaskInput` dropped their `Token` fields; `internal/agents.AgentctlTokenSource` (new) is called by `internal/worker`'s activities right before each A2A call. This is explicitly a placeholder for local/demo use (documented as such in the token source's own doc comment) — it requires whoever runs the workflow-server to have their own `agentctl login` session on that machine.

Also surfaced and fixed along the way: `internal/agents/dispatch.go`'s A2A response parsing was wrong from the very first version of this code (assumed a `{"kind": "message"|"task", ...}` shape; the real control plane always returns `{"task": {...}}` with result text under `artifacts[0].parts`, states like `TASK_STATE_COMPLETED`) — confirmed and fixed against the live stage control plane, with tests updated to the real fixture shapes.

See `docs/zigflow-syntax-reference.md` for the Zigflow `for`-loop/data-flow rules this workflow's fix depended on (`export.as` merge semantics, why a `for` loop needs both an inner and outer `export.as`, `$output` vs `$data` vs `$context`).
