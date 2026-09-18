---
title: Workflow Server Auth + List Endpoint
description: Enforce Keycloak JWT auth on all workflow-server REST endpoints and add a list-definitions endpoint, following com.sixt.service.managed-agents' pattern
status: implemented
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: Workflow Server Auth + List Endpoint

## Goal

Prerequisite for the workflow-server web UI: enforce real Keycloak-issued
JWT auth on every REST endpoint (currently none is enforced — `POST
/definitions` and `GET /definitions/{tenant}/{name}` are open), expose the
same `/api/v1/auth/config` shape the reference web app
(`com.sixt.web.managed-agents`) already knows how to consume, and add a
`GET /definitions` list endpoint so the UI has something to render.

## Acceptance Criteria

- [x] `GET /api/v1/auth/config` returns `{keycloak_url, keycloak_realm,
      keycloak_client_id, admin_roles}` — same field names/casing as
      `com.sixt.service.managed-agents/controlplane/api/auth.go`'s
      `AuthConfigResponse` — sourced from env vars
      (`KEYCLOAK_URL`/`KEYCLOAK_REALM`/`KEYCLOAK_CLIENT_ID`/`KEYCLOAK_ADMIN_ROLES`).
      This endpoint itself is unauthenticated (chicken-and-egg: the UI needs
      it before it has a token).
- [x] `POST /definitions` and `GET /definitions/...` reject requests with no
      `Authorization: Bearer <token>` header, or an invalid/expired one, with
      401 and a JSON `{"error": "..."}` body — mirroring
      `internal/auth/middleware.go`'s exact rejection messages
      ("missing or invalid authorization header", "invalid or expired
      token").
- [x] A valid token (correct signature verified against the configured
      Keycloak realm's JWKS, correct issuer, not expired) is accepted and the
      request proceeds — no tenant mapping or user provisioning in this
      slice (see Out of Scope).
- [x] `GET /definitions` lists every stored definition (tenant, name,
      buildId, status, createdAt), newest first, no tenant/pagination
      filtering.
- [x] Server fails to start with a clear error if `KEYCLOAK_URL` or
      `KEYCLOAK_REALM` is unset — no silent "auth disabled" fallback.

## Approach

Port `internal/auth`'s `JWKSValidator` (RSA/JWKS verification, no external
JWT library — copied near-verbatim from
`com.sixt.service.managed-agents/internal/auth/jwks.go`) and a trimmed
`Middleware` (checks the bearer token, rejects on failure, does *not* upsert
a user or resolve a tenant — this server has no user/tenant store yet).
`internal/api`'s `Server.Routes` wraps the mux with this middleware, exempting
only `GET /api/v1/auth/config`. `store.DefinitionStore` gains a `List`
method; `store/postgres` implements it with a plain `SELECT ... ORDER BY
created_at DESC`.

## Affected Modules

- `internal/auth/` (new) — `JWKSValidator`, `KeycloakClaims`, `Middleware`,
  ported from `com.sixt.service.managed-agents/internal/auth/{jwks,claims,middleware}.go`
  with `UserStore`/`TenantMap` support stripped (no equivalent store here).
- `internal/api/api.go` — add `AuthConfigHandler`, wire `internal/auth.Middleware`
  around the mux in `Routes`, add `GET /definitions` handler.
- `store/definitions.go` — add `List(ctx) ([]*Definition, error)` to the
  `DefinitionStore` interface.
- `store/postgres/definitions.go` — implement `List`.
- `cmd/workflow-server/main.go` — read `KEYCLOAK_URL`/`KEYCLOAK_REALM`/
  `KEYCLOAK_CLIENT_ID`/`KEYCLOAK_ADMIN_ROLES` env vars, construct the
  validator, fail fast if URL/realm are unset.

## Test Strategy

- Unit tests on `internal/auth`: valid token accepted, expired token
  rejected, wrong-issuer token rejected, malformed bearer header rejected,
  missing header rejected — using a locally-generated RSA keypair and a
  hand-built JWKS response (same approach as
  `com.sixt.service.managed-agents/internal/auth/jwks_test.go`; no real
  Keycloak needed for unit tests).
- Unit tests on `internal/api`: `/api/v1/auth/config` reachable without a
  token; `/definitions` (both verbs) reject with 401 when no/invalid token
  is set on the fake-store-backed test server.
- Integration test on `store/postgres`: `List` returns multiple definitions
  newest-first.
- End-to-end: with `KEYCLOAK_URL`/`KEYCLOAK_REALM` pointed at the real stage
  Keycloak (`identity-stage.goorange.sixt.com`, realm `SixtEmployees`),
  mint a real token via `agentctl login`'s cached credentials (same
  extraction approach used earlier in this session) and confirm a real
  `POST /definitions` succeeds with it and fails with a tampered token.

## Out of Scope

- User provisioning / `store.UserStore` — no user table exists in this
  service; the middleware only validates the token, it doesn't persist
  anything about the caller.
- Tenant mapping (`TenantMapping`, realm-role → tenant) — `GET /definitions`
  returns every tenant's rows regardless of caller identity.
- `RequireAdmin` / role-gated routes — `admin_roles` is round-tripped through
  `/auth/config` for the UI's own use (e.g. hiding an admin-only nav item
  later) but nothing server-side enforces it yet.
- Pagination on `GET /definitions` — fine for a demo-scale row count.

## Notes

Confirmed the exact response shape and rejection strings by reading
`com.sixt.service.managed-agents/controlplane/api/auth.go` and
`internal/auth/middleware.go` directly rather than inferring them from the
frontend — the frontend's `AuthConfig` type names
(`keycloak_url`/`keycloak_realm`/`keycloak_client_id`/`admin_roles`) match
the backend's JSON tags exactly, so no translation layer is needed between
this server and the existing web app's `auth.ts` if it's ever pointed here.

## Implementation notes

- `internal/auth/{jwks,claims,middleware}.go` ported from
  `com.sixt.service.managed-agents/internal/auth/`, trimmed as planned
  (no `UserStore`/`TenantMap`/`RequireAdmin`/`ExemptPrefixes`/`ExemptPatterns`).
  `internal/auth/jwks_test.go` covers valid/expired/wrong-issuer/
  wrong-signing-key/malformed-token cases with a locally generated RSA key
  and a hand-built JWKS `httptest.Server` — no real Keycloak needed.
- `internal/api.Server` now takes an `AuthConfig{Validator, KeycloakURL,
  KeycloakRealm, KeycloakClient, AdminRoles}`. `Routes` registers
  `GET /api/v1/auth/config` unauthenticated, then wraps every other route
  (`POST /definitions`, `GET /definitions`, `GET /definitions/{tenant}/{name}`)
  behind `auth.Middleware` via a nested `http.ServeMux` mounted at `/`.
  `internal/api/api_test.go` mints tokens against a per-test JWKS server to
  cover: auth-config reachable without a token, all three protected routes
  reject a missing/invalid token with 401, and `GET /definitions` returns
  the fake store's rows.
- `store.DefinitionStore` gained `List(ctx) ([]*Definition, error)`;
  `store/postgres` implements it as a plain `SELECT ... ORDER BY created_at
  DESC` with no filtering. New integration test
  `TestDefinitionStore_ListReturnsNewestFirst` creates three rows across two
  workflow names and asserts their relative order within the tenant (the
  table isn't test-isolated, so it filters the full result down to rows
  matching its own generated tenant before asserting order).
- `cmd/workflow-server/main.go` now requires `KEYCLOAK_URL` and
  `KEYCLOAK_REALM` (fails fast via `fmt.Errorf` if either is unset, same
  pattern as the existing `DATABASE_URL` check), reads
  `KEYCLOAK_CLIENT_ID`/`KEYCLOAK_ADMIN_ROLES` optionally
  (`auth.ParseAdminRoles` on the latter), and constructs one
  `auth.JWKSValidator` for the process lifetime.
- End-to-end verification against the real prod Keycloak
  (`identity-prod.orange.sixt.com`, realm `SixtEmployees`): ran
  `workflow-server` against local Postgres with `KEYCLOAK_URL`/`KEYCLOAK_REALM`
  pointed at prod, extracted a real access token from `agentctl`'s cached
  session (`~/.agentctl/tokens/default.yaml`'s `auth_token` field is itself a
  JSON blob — the actual JWT is its nested `access_token` key, not the outer
  string), and confirmed: no token -> 401, valid token -> 200 on `GET
  /definitions` and 201 on `POST /definitions`, tampered token (real token
  with characters appended) -> 401 `{"error":"invalid or expired token"}`.
  The published row round-tripped correctly through `GET /definitions`
  alongside pre-existing integration-test rows, confirming newest-first
  ordering against real data, not just fixtures.
- All unit tests (`go test ./...`, 26 cases across 7 packages) and
  integration tests (`go test -tags integration ./store/...` against a
  podman-run Postgres) pass.
