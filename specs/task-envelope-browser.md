---
title: Task Envelope Browser
description: UI + API to look up a workflow execution's task envelope (slot refs) and inspect blob content, keyed by workflow ID
status: implemented
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: task-envelope-browser

## Goal

Let a person look up a workflow execution by ID and see what slots/refs it
produced, then drill into a slot's actual blob content — without manually
decoding Temporal history and cross-referencing Minio objects by hand
(the reconstruction done ad hoc while verifying
`specs/task-envelopes-minimal-slice.md`).

## Acceptance Criteria

- [x] `blobstore.Store` gains `PutIndex(ctx, tenant, workflowID string, slots map[string]manifest.Ref) error` and `GetIndex(ctx, tenant, workflowID string) (map[string]manifest.Ref, error)`, backed by a fixed (non-content-addressed) key `{tenant}/by-workflow/{workflowID}.json`.
- [x] `pod-memory-trend.workflow.yaml` gets a new final activity (`writeEnvelopeIndex`), called once from the root workflow only — never from inside a `pollX` child loop — passing its own `activity.GetInfo(ctx).WorkflowExecution.ID` and `$context`'s current slot refs (`opsBuddyRef`, `logMonitoringRef`, `editorRef`). Registering this activity required extending `workflowworker.Supervisor` (not anticipated in the original "Affected Modules" — see Notes) since it's the only thing hosting workers on a workflow's own task queue.
- [x] `internal/api.Server` gains an `EnvelopeStore` dependency and two new protected routes: `GET /envelopes/{workflowID}` (returns `{slotName: Ref}` JSON from the index) and `GET /envelopes/{workflowID}/{digest}` (returns raw blob content, `Content-Type` from the stored `mediaType`, restricted to digests actually listed in that workflow's own envelope).
- [x] New UI view `EnvelopeBrowserView.vue` reachable from the router/nav: input a workflow ID, list its slots with ref metadata (digest, size, media type), click a slot to fetch and display its blob content. Deep-linkable via `envelopes/:workflowId`.
- [x] End-to-end: ran `pod-memory-trend.workflow.yaml` (workflow ID `pod-memory-trend-1789799936`, input `envoy-qt`/`platform`) against real registered agents, then looked up that workflow ID via `GET /envelopes/{workflowID}` and confirmed `opsBuddyRef`/`logMonitoringRef`/`editorRef` all present with correct digests/sizes, and fetched Editor's blob content via `GET /envelopes/{workflowID}/{digest}` — matched the workflow's actual final result text exactly. This also closed a gap noticed while validating `specs/task-envelopes-minimal-slice.md`: Editor's own blob was never separately visible before; it now shows up correctly in the index.

## Approach

Blobs stay content-addressed and untouched (`{tenant}/sha256/{hash}`, written by `invoke-agent`/`poll-agent-task` as today). A new activity, called only as the root workflow's last step, writes a small index object mapping slot name → `Ref` under a workflow-ID-keyed path — this sidesteps the fact that `poll-agent-task` runs inside Zigflow's per-iteration child workflows (confirmed via `temporal workflow list`: each `for` loop iteration is its own child execution) and Temporal's `ActivityInfo` has no root-execution field, so a child-scoped activity can't recover the root ID on its own. The API layer reads the index, then streams individual blobs on demand; the UI is a thin lookup-and-drill-down view matching the existing `DefinitionDetailView.vue` pattern (fetch on route param, render structured result).

## Affected Modules

- `internal/blobstore/blobstore.go` — add `PutIndex`/`GetIndex` to `Store` and `MinioStore`. Boundary unchanged: still the only thing touching Minio.
- `internal/workflowworker/` (new `activity.go`, changed `supervisor.go`) — `write-envelope-index` activity implementation and registration on every workflow worker `Register` starts; `Supervisor` gains a `BlobStore` dependency. Not in the original plan — needed because this is the only place that hosts a worker on a workflow's own task queue.
- `workflows/pod-memory-trend.workflow.yaml` — one new final-step activity call; no changes to `askX`/`pollX` stages.
- `internal/api/api.go` — new `EnvelopeStore` interface (`GetIndex`, `Get`), wired into `Server`, two new routes registered in `Routes()`.
- `cmd/workflow-server/main.go` — pass the already-constructed `blobstore.MinioStore` into both `workflowworker.NewSupervisor` and `api.New`.
- `ui/src/views/EnvelopeBrowserView.vue` (new), `ui/src/stores/envelopes.ts` (new), `ui/src/types/index.ts` (new `EnvelopeRef`/`EnvelopeIndex` types), `ui/src/router/index.ts` (new `envelopes/:workflowId?` route), `ui/src/views/AppLayout.vue` (nav link), `ui/vite.config.ts` (dev proxy needed `/envelopes` added).

## Test Strategy

- `internal/blobstore`: extend the existing `-tags=integration` suite with `PutIndex`/`GetIndex` round-trip and not-found cases against real Minio.
- `internal/api`: handler tests for the two new routes (happy path + 404 for unknown workflow ID / digest), following `internal/api/api_test.go`'s existing pattern.
- Manual end-to-end: run the workflow, hit `GET /envelopes/{workflowID}` and `GET /envelopes/{workflowID}/{digest}` via curl, then confirm the same data renders correctly in the new UI view.
- `go test ./...` and `-tags=integration` must keep passing.

## Out of Scope

- Revision history in the browser (index holds only current refs per slot, not the full append-only `Slot.Revisions` list) — a v2 concern if needed.
- Any change to how `invoke-agent`/`poll-agent-task` store content-addressed blobs.
- Making the index/browser generic across arbitrary workflows — this slice wires it for `pod-memory-trend.workflow.yaml`; generalizing the "write index as last step" pattern to other workflow YAMLs is a follow-up.
- Auth/authorization beyond the existing Keycloak JWT middleware already applied to all protected routes.

## Notes

Prior art / why this shape: `docs/architecure/task-envelope-design.md` §5.1's `Ref.urls` field anticipates self-locating refs (implemented in `specs/task-envelopes-minimal-slice.md` as `Ref.Bucket`); this spec adds the missing "how do I find all the refs for task X" lookup, which the design doc left as an open question (§15.1, ref registry) — this is a lighter-weight stand-in (a per-workflow index object) rather than the full Postgres ref registry, appropriate for this project's current scale.

### Implementation notes / scope expansion beyond the original spec

- `workflowworker.Supervisor` (not in the original "Affected Modules") needed a `BlobStore` dependency and a new `internal/workflowworker/activity.go` registering `write-envelope-index` on every workflow worker it starts — `workflowworker.Supervisor.Register` previously only built Zigflow's own closure tree via `zigflowadapter.Build`, with no mechanism for registering custom Go activities on a workflow's own task queue. This is the only place that hosts a worker on `pod-memory-trend`'s queue (as opposed to `agent-<id>` queues, handled by `worker.Supervisor`).
- Follow-up ticket `trex-qvv` tracks making this automatic (e.g. via `go.temporal.io/sdk/interceptor.WorkerInterceptor`, confirmed to exist and plausibly usable from inside `workflowworker.Supervisor`) instead of requiring every workflow author to manually add a `writeEnvelopeIndex` step as their literal last `do:` entry before output — this is real authoring friction and an easy mistake to make (e.g. calling it from inside a `pollX` loop, which would silently record the wrong workflow ID).
- Editor's activity output (`InvokeAgentOutput.Result`/`PollAgentTaskOutput.Result`) already carries plain text for `formatOutput`'s use (per `specs/task-envelopes-minimal-slice.md`); this feature didn't need to touch that — the browser reads `editorRef` from the index like any other slot.
- Digest lookups (`GET /envelopes/{workflowID}/{digest}`) are restricted to digests actually present in that workflow's own envelope index, not arbitrary blobstore contents — prevents one workflow's viewer from becoming a general Minio browser.
- Found and fixed two UI gaps only surfaced by testing a real deep link in a running browser (not caught by `vue-tsc`/`vite build`): the dev proxy (`ui/vite.config.ts`) didn't forward `/envelopes`, and the router had no `envelopes/:workflowId` param route for direct navigation — both fixed.
