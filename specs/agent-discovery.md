---
title: Agent Discovery and Temporal Activity Registration
description: Discover agents from the managed agents platform and register each as a Temporal activity worker
status: superseded by agent-discovery-and-registration.md
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: agent-discovery

## Goal

Workflow server discovers agents from the managed agents control plane and registers each one as a Temporal activity on its own task queue, so Zigflow workflows can call agents via `call: activity` by name.

## Acceptance Criteria

- [ ] Discovery service polls the managed agents control plane (agent list endpoint) on a configurable interval and produces a catalog of agents (name/slug, id, status).
- [ ] Worker supervisor reconciles the catalog against running agent-activity workers: new agent -> start worker with activity registered on `agent-<slug>` task queue; removed/disabled agent -> stop its worker gracefully. Unchanged agents are left running (no full rebuild).
- [ ] Activity implementation dispatches the call via the platform's A2A endpoint using the workflow's input, waits for the result, and returns it (or the A2A error) as the activity outcome.
- [ ] A Zigflow workflow definition can invoke a discovered agent via `call: activity, name: <agent-slug>` and receive its result.
- [ ] UI adds a read-only agent catalog view (list) showing agent name, task queue, worker status (up/down), and last-discovered timestamp, following the existing `DefinitionsListView` pattern.

## Approach

Add a discovery poller (interval-driven) that calls the managed agents API and republishes an in-memory catalog on change; extend the worker supervisor's existing level-triggered reconcile loop to also manage one Temporal worker per agent (task queue `agent-<slug>`, single generic activity function that performs the A2A call). Expose the catalog over a small read endpoint for the new UI list view.

## Affected Modules

- `internal/agents` (new) — discovery poller + catalog (fetch, diff, in-memory store); boundary: only this package talks to the managed-agents control plane API.
- `internal/worker` (new or extend supervisor) — reconcile catalog diffs into start/stop of per-agent Temporal workers; activity function wraps A2A dispatch.
- `internal/api` — new read endpoint (e.g. `GET /agents`) exposing the catalog for the UI.
- `cmd/workflow-server/main.go` — wire discovery poller + supervisor extension into startup, add config (poll interval, control-plane URL/credentials).
- `ui/src/services`, `ui/src/stores`, `ui/src/views`, `ui/src/router` — new `AgentsListView` + service/store, new nav entry, mirroring `DefinitionsListView`/`definitions.ts`.

## Test Strategy

- Unit tests for catalog diffing (added/removed/unchanged agents) in `internal/agents`.
- Unit tests for supervisor reconcile logic (start/stop decisions) using a fake catalog source, mirroring existing supervisor tests if any.
- Integration test: register a fake agent via a stubbed control-plane response, verify a Temporal worker comes up on `agent-<slug>` and a test workflow calling it via `call: activity` receives the stubbed A2A result.
- UI: manual check of the new list view against a running workflow-server (per project convention of exercising UI changes in-browser before calling done).

## Out of Scope

- Custom per-agent retry/timeout policy (use Temporal activity defaults).
- Editing, enabling/disabling, or manually triggering agents from the UI (read-only list only).
- Webhook/event-driven discovery (polling only, this iteration).
- Multi-tenancy scoping of the agent catalog (assumes single control-plane project/tenant for now).

## Notes

Mirrors the existing worker supervisor's level-triggered reconcile pattern (docs/architecure/zigflow-workflow-server-architecture.md) but for agent-activity workers instead of Zigflow definition workers — same self-healing property on pod restart or missed poll.
