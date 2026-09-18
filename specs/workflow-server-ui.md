---
title: Workflow Server UI (definitions list)
description: Vue 3 SPA showing stored workflow definitions, authenticated via the same Keycloak flow as com.sixt.web.managed-agents
status: implemented
author: Heiko Braun <ike.braun@googlemail.com>
---

# Feature: Workflow Server UI (definitions list)

## Goal

First UI slice for the Zigflow Workflow Server: a Vue 3 SPA in `ui/` that
logs in via Keycloak (same `keycloak-js` PKCE flow as
`com.sixt.web.managed-agents`) and lists the workflow definitions stored by
`workflow-server` (`specs/definition-store.md`,
`specs/workflow-server-auth.md`). Read-only — no publish/edit from the UI
in this slice.

## Acceptance Criteria

- [x] Visiting the app with no session redirects to Keycloak login; after
      login, lands back on the definitions list.
- [x] The list view calls `GET /definitions` with the user's bearer token
      and renders each row's tenant, name, build ID, status, and created-at,
      newest first.
- [x] A 401 from any API call (session expired, token revoked) redirects
      back to login — no dead "stuck loading" state.
- [x] `npm run build` produces a working static bundle (`vue-tsc` type
      -check + `vite build`, matching the reference app's `build` script).
- [x] Dev server (`npm run dev`) proxies `/definitions` and
      `/api` to the configured `workflow-server` origin, so the app talks to
      `http://localhost:5173` in the browser and the real backend underneath
      — same shape as the reference app's `vite.config.ts` proxy.

## Approach

Scaffold with the same toolchain as `com.sixt.web.managed-agents`
(Vite + Vue 3 + TypeScript + Tailwind 4 + Pinia + vue-router +
shadcn-vue/reka-ui), copying `services/auth.ts` and `services/http.ts`
near-verbatim (they're already generic over API base URL and token source).
One store (`stores/definitions.ts`), one view (`DefinitionsListView.vue`),
router with `requiresAuth` meta + a global guard — same pattern as the
reference app's `router/index.ts`, trimmed to the one real route plus
`/login` and `/callback`.

## Affected Modules

- `ui/` (new) — standalone npm package, not part of the Go module. Layout:
  `src/{services,stores,views,router,types,components,lib}`.
- `ui/Makefile` (new) — `make dev` (npm install if needed, then `npm run
  dev`), `make build`, matching the root `Makefile`'s target-naming style so
  running the whole demo stack is a consistent `make <thing>` across both
  the Go service and the UI.
- No changes to existing Go code beyond what `specs/workflow-server-auth.md`
  already covers (CORS: `cmd/workflow-server` needs to allow the UI's dev
  origin — tracked as part of that spec's `CORSAllowedOrigins`-equivalent,
  added here only if not already present).

## Test Strategy

- `npm run build` succeeds (type-check + production bundle).
- Manual verification per this project's UI-testing convention: run
  `workflow-server` + the UI dev server together, log in with a real
  Keycloak account, confirm the definitions list renders real rows
  published earlier in this session (via the `definition-store` spec's
  e2e curl calls), and confirm a forced 401 (revoked/expired token) bounces
  back to login instead of showing a blank or broken page.

## Out of Scope

- Publish/edit/retire from the UI — read-only list view only.
- Any view beyond the definitions list (no per-definition detail page yet).
- Admin-role-gated UI elements — `admin_roles` is available from
  `/auth/config` but nothing in this slice branches on it.
- Real-time updates (polling/websocket) — a manual refresh is enough for
  this slice.

## Notes

Requires a new Keycloak client redirect URI to be registered for this UI's
dev origin (e.g. `http://localhost:5173/callback`) before browser login can
work end-to-end — a Keycloak-admin action outside this repo, confirmed with
the user as a prerequisite before implementation starts on this spec (see
`specs/workflow-server-auth.md`'s sibling discussion). Until that's done,
`npm run dev` will build and run, but the login redirect will fail at
Keycloak with a redirect_uri mismatch.

## Implementation notes

- Scaffolded via `npm create vite@latest ui -- --template vue-ts`, then
  trimmed to the default template's assets/components and added
  `vue-router`, `pinia`, `keycloak-js`, Tailwind 4
  (`@tailwindcss/postcss`+`postcss`+`autoprefixer`), and `vue-tsc` — no
  shadcn-vue/reka-ui component library pulled in for this slice, since a
  single read-only table doesn't need it; plain Tailwind utility classes are
  enough. Revisit if/when a second view needs real form controls.
- `src/services/auth.ts` and `src/services/http.ts` ported from
  `com.sixt.web.managed-agents`, trimmed: no admin-role helpers
  (`isAdmin`/`hasAdminRole`/`getRoles`), no fallback `AuthConfig` on fetch
  failure (this app has nothing sensible to fall back to without a real
  `/api/v1/auth/config`, so a failed fetch surfaces as a thrown error
  instead of silently degrading), no `silentCheckSsoRedirectUri` (no
  `silent-check-sso.html` asset in this slice).
- `src/stores/auth.ts` trimmed similarly (no roles/isAdmin/userRole).
  `src/stores/definitions.ts` is new: `fetchAll()` via `authedFetch`,
  exposes `definitions`/`isLoading`/`error`.
- Router (`src/router/index.ts`) trimmed to 3 routes: `/login`, `/callback`,
  `/` (the definitions list), same `beforeEach` guard shape as the
  reference app minus the `requiresAdmin` branch (nothing admin-gated yet).
- `internal/api`'s `definitionResponse` gained a `createdAt` field (was
  missing before this slice — the Definition Store spec never needed it
  since neither of its two endpoints displayed a list) so the UI's newest-
  first acceptance criterion has real data to render; `store.Definition`
  already carried `CreatedAt`, so this was JSON-shape-only, no store change.
- `ui/vite.config.ts` proxies `/api` and `/definitions` to
  `VITE_API_BASE_URL` (default `http://localhost:8080`, matching the root
  Makefile's `workflow-server` target) — verified by running the real
  `workflow-server` binary + Vite dev server together and confirming
  `curl http://localhost:5173/api/v1/auth/config` returns the backend's real
  JSON and `curl http://localhost:5173/definitions` returns the backend's
  real 401 (no token), proving the proxy reaches the live server rather
  than a mock.
- `npm run build` (`vue-tsc -b && vite build`) passes cleanly. One fixup
  needed: TypeScript's `baseUrl` compiler option is deprecated in the
  scaffolded TS version, so the `@/*` path alias is declared via `paths`
  alone (no `baseUrl`), which still resolves correctly since the paths are
  relative to `tsconfig.app.json` itself.
- Did not verify the full interactive Keycloak login redirect in a real
  browser (no browser automation available in this environment) — per the
  Notes above, that also requires a redirect-URI registration this repo
  doesn't control. Verified everything up to that boundary: real Keycloak
  realm reachable (`/.well-known/openid-configuration` returns 200), real
  `/api/v1/auth/config` values flow through the dev proxy into what
  `services/auth.ts` would construct the `Keycloak` client from.
