---
title: Task Envelopes (minimal slice)
description: Pass agent activity results by reference (Minio-backed) instead of inline strings, proving the task-envelope pattern on pod-memory-trend.workflow.yaml
status: implemented
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: task-envelopes-minimal-slice

## Goal

Prove the reference-based data-passing pattern from
`docs/architecure/task-envelope-design.md` end-to-end on one real workflow,
so agent results stop living as raw strings in Temporal Event History and
gain minimal lineage (producedBy/reason), backed by a local Minio
container.

## Acceptance Criteria

- [x] `internal/manifest` package: `Ref` (mediaType, digest, size),
      `Revision` (Ref, ProducedBy, Reason), `Slot` (Current + Revisions),
      append-only merge helper — no `facts`, no derived views.
- [x] `internal/blobstore` package: content-addressed put/get over Minio
      (`minio-go/v7`), key layout `{tenant}/sha256/{hash}`, tenant
      hardcoded to `"platform"`.
- [x] `internal/worker/activity.go`: `InvokeAgentInput` carries
      `Reads map[string]manifest.Ref` + `Instruction string` (fixed
      per-call task text with placeholders for resolved slot content, no
      pre-interpolated results); `InvokeAgentOutput` returns a
      `manifest.Revision` instead of `Result string`. Same shape for the
      completed-result path of `PollAgentTaskOutput`.
- [x] `pod-memory-trend.workflow.yaml` updated so `$context` carries slot
      revisions (refs) between stages, each `askX` step passes refs +
      instruction template rather than concatenated text, and the
      workflow's final output is unchanged from the user's perspective.
- [x] `make minio` / `make minio-stop` / `make minio-logs` targets mirror
      `make db`'s podman lifecycle and health-check pattern; `make run`
      wires Minio env vars.
- [x] End-to-end run of `pod-memory-trend.workflow.yaml` against real
      agents produces the same synthesized answer as before, with
      Temporal activity payloads now small refs and each agent result
      landing as a content-addressed object in Minio. Run
      `pod-memory-trend-1789797895` (input `podName=envoy-qt,
      namespace=platform`) against the three real registered agents
      (Ops Buddy, Log Monitoring, Editor) completed successfully; see
      Implementation notes below for the reconstructed manifest and a
      gap it surfaced (`Ref.Bucket`, since fixed).

## Approach

Activities resolve their own declared refs from Minio and build the
prompt by substituting resolved slot content into a workflow-supplied
instruction template (placeholder substitution in Go, template string
comes from YAML) — same division of labor as today's `export.as` string
building, just moved past the ref-resolution step. The manifest continues
to live in Zigflow's `$context` (workflow remains single writer per
design doc §7); this slice does not change that ownership model, only
what the slots hold (refs instead of raw text).

## Affected Modules

- `internal/manifest/` (new) — Ref/Revision/Slot types and append-only
  merge helper. No dependency on Temporal or Zigflow.
- `internal/blobstore/` (new) — `Store` interface + `MinioStore` impl
  (`minio-go/v7`). Boundary: activities are the only callers; agents and
  the workflow itself never touch it directly (per design doc §9).
- `internal/worker/activity.go` — `InvokeAgentInput`/`InvokeAgentOutput`
  and `PollAgentTaskOutput` reshaped to carry refs/revisions; activity
  bodies gain resolve-then-format-then-dispatch and
  write-result-then-return-revision steps.
- `internal/worker/supervisor.go`, `cmd/workflow-server/main.go` — thread
  a `blobstore.Store` dependency alongside existing `tokens`/`dispatcher`.
- `workflows/pod-memory-trend.workflow.yaml` — `export.as` expressions
  and `askX`/`pollX` activity arguments updated to the ref+instruction
  shape.
- `Makefile`, `CLAUDE.md` — new Minio lifecycle targets and startup-order
  step, following the existing `db`/`db-stop`/`db-logs` pattern.
- `go.mod` — add `github.com/minio/minio-go/v7`.

## Test Strategy

- `internal/manifest`: unit tests for append-only revision behavior and
  `current` pointer movement.
- `internal/blobstore`: integration test behind `-tags=integration`
  against a real local Minio (mirrors `store/postgres`'s
  `-tags=integration` convention against real Postgres), run via
  `make minio && TEST_DATABASE_URL=... go test -tags=integration ./...`.
- `go test ./...` must keep passing for existing packages
  (`internal/worker`, `internal/agents`, `internal/zigflowadapter`).
- Manual end-to-end: `make minio && make db && make temporal && make run`,
  register the three demo agents per CLAUDE.md's startup order, run
  `pod-memory-trend.workflow.yaml` via `zigflow run`, confirm in Temporal
  UI that activity payloads are small refs, and inspect the Minio bucket
  to confirm each agent result is stored content-addressed.

## Out of Scope

- Schema registry / JSON Schema validation (design doc §10).
- Pull-mode MCP resolver tool for agents (design doc §9.3).
- Ref registry in Postgres, GC, retention classes (design doc §12).
- Manifest snapshots to object store (design doc §7 "Snapshots").
- `facts` map, derived views, revision compaction, continue-as-new
  handling.
- Multi-tenancy plumbing (tenant stays hardcoded `"platform"`).

## Notes

Each excluded item above should get its own follow-up beads ticket
referencing the relevant design-doc section, rather than being silently
dropped. Per this repo's CLAUDE.md, this spec and its implementation get
one beads ticket tracking the whole slice.

### Implementation notes

- `docker.io/minio/minio` now requires Docker Hub auth ("requested access
  to the resource is denied" on pull — Minio Inc. moved official images
  behind a login wall in 2025) and `quay.io/minio/minio` is unreachable
  from this network (same Cato TLS-inspection proxy issue documented in
  CLAUDE.md's "Connecting to Temporal" section: `x509: certificate signed
  by unknown authority`). `make minio` uses `docker.io/bitnamilegacy/minio`
  instead — an open, unauthenticated mirror of the last Bitnami Minio
  build.
- The local podman machine's VM clock had drifted ~21 hours behind host
  time (likely from the host sleeping), which made every Minio S3 call
  fail with "the difference between the request time and the server's
  time is too large" until resynced with `podman machine ssh -- sudo date
  -u -s "@$(date -u +%s)"` and the container restarted. Not specific to
  this feature, but worth knowing if `make minio` integration tests fail
  with a clock-skew error later.
- Editor's `invoke-agent`/`poll-agent-task` output carries both a
  `Revision` (ref, for lineage/downstream consumers) and a plain `Result`
  string, used only by `formatOutput`'s final `output.as` so the workflow
  doesn't need a separate ref-resolution activity just to produce its
  own text result.
- End-to-end run against real registered agents (Ops Buddy, Log
  Monitoring, Editor) on `pod-memory-trend-1789797895` (input `envoy-qt`/
  `platform`) completed successfully: `opsBuddyRef` and `logMonitoringRef`
  each resolved to real content-addressed objects in Minio, and the
  workflow's final result matched Editor's synthesis. Confirms the ref
  round-trip works against live agents, not just the blobstore
  integration tests.
- Manually reconstructing that run's manifest (decoding Temporal history
  + reading Minio objects by hand) surfaced a real gap: `manifest.Ref` had
  no bucket/location field, so a digest alone wasn't resolvable without
  already knowing the tenant/bucket convention out of band. Fixed by
  adding `Ref.Bucket`, populated by `blobstore.MinioStore.Put` and
  consulted (with a fallback to the store's own configured bucket, for
  refs produced before this field existed) by `Get` — closer to design
  doc section 5.1's `urls` field, minus the array since this slice has a
  single store.
