---
title: Async agent dispatch via a separate poll-agent-task activity
description: invoke-agent returns as soon as an A2A task is created instead of blocking on it; a new poll-agent-task activity checks status, so no single call needs a token that outlives it
status: proposed
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: async-agent-dispatch

## Goal

Fix the mechanism behind trex-8hb: `Dispatcher.Dispatch` currently blocks one Temporal activity attempt inside an in-process HTTP poll loop for an A2A task's entire lifetime, reusing one token captured at call-start. Split into two fast activities — `invoke-agent` (send, return immediately) and `poll-agent-task` (check status once) — so every individual call is short-lived and can carry its own fresh token.

## Acceptance Criteria

- [ ] `invoke-agent`'s activity sends the A2A message and returns as soon as the control plane responds — either a completed `Message` result (still returned directly, no polling needed) or a `{agentID, taskID}` pair when the control plane returns an async `Task`. It never calls `awaitTask`/blocks on task completion itself.
- [ ] A new `poll-agent-task` activity takes `{agentID, taskID, token}`, calls `GET /a2a/{agentID}/tasks/{id}` once, and returns `{done bool, result string, failed bool, failureReason string}` — one HTTP call, no internal loop.
- [ ] `workflows/pod-memory-trend.workflow.yaml` is updated to call `invoke-agent`, then (when it returned a taskID rather than a direct result) loop `call: activity, name: poll-agent-task` with a `wait` between attempts until `done`, using Zigflow's own retry/wait constructs — each poll call passes the input's `callerToken` fresh, not a value threaded through from the first call.
- [ ] Both activities are registered on the same per-agent Temporal worker/task queue (`agent-<id>`) the supervisor already starts — no new worker/task queue.
- [ ] Existing agent-registration/worker-supervisor tests and the demo workflow's validation both stay green; a live run against a real agent shows `invoke-agent` returning in the time of one HTTP call (not minutes) and `poll-agent-task` being called repeatedly until completion.

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
