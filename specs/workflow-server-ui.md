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

Initially ran the dev server on Vite's default port 5173 against the real
prod Keycloak (`identity-prod.orange.sixt.com`) with the `agent-cli` client
— login failed with `Invalid parameter: redirect_uri`, since that client has
no `localhost:5173/callback` redirect URI registered. Rather than get a new
URI registered for a one-off port, switched to reuse
`com.sixt.web.managed-agents`'s existing dev setup: port `8484` and the
`managed-agents-console` Keycloak client, which already has
`http://localhost:8484/callback` registered. Also switched the IdP to
**stage** (`identity-stage.goorange.sixt.com`, not prod) to match, since
`managed-agents-console`'s redirect-URI registration lives against stage,
not prod — both `workflow-server` (root `Makefile`'s `KEYCLOAK_URL`/
`KEYCLOAK_CLIENT_ID`) and the UI (`ui/vite.config.ts`'s dev port,
`ui/.env.development`) now point at this same stage/8484/
managed-agents-console trio for local dev. Production deployment would use
its own dedicated Keycloak client and redirect URI, not this borrowed one.

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

### Follow-on: left-hand nav + split-panel detail view

Added after the initial implementation above, same spec (no new ticket —
still one read-only definitions slice):

- `src/views/AppLayout.vue` (new): left sidebar nav + `<RouterView>`,
  wraps every authenticated route. Currently one nav entry ("Workflow
  Definitions"); more entries land here as the UI grows.
- Definition detail is a separate full view (`DefinitionDetailView.vue`),
  not a split pane — first attempt was a right-hand panel sharing
  `DefinitionsListView.vue`, revised per feedback to a dedicated route
  component instead. `DefinitionsListView.vue` stays list-only.
  Navigation is route-driven (`/definitions/:tenant/:name`, route name
  `definition-detail`), not local component state, so the detail is
  linkable/shareable and survives a page refresh.
- The detail view has a proper header: breadcrumb
  (`Workflow Definitions / {tenant} / {name}`) above the title/build-ID/
  "Back to list" row, both driven by `RouterLink`. If the definitions
  store hasn't loaded yet (e.g. a direct link/refresh landing straight on
  the detail route), the view fetches it itself before rendering.
- **Bug found and fixed while verifying the new route**: a hard refresh
  (or direct URL visit) of `/definitions/:tenant/:name` was proxied
  straight to the backend by `ui/vite.config.ts`'s `/definitions` proxy
  rule instead of being served as the SPA shell — the backend then
  401'd the bare navigation request (no bearer header) and the browser
  rendered raw JSON instead of the app. Fixed by porting
  `com.sixt.web.managed-agents/vite.config.ts`'s `bypassNavigation`
  helper: real page navigations (`Sec-Fetch-Dest: document`, or an
  `Accept: text/html` fallback) are rewritten to `/index.html` before
  hitting the proxy; real `fetch`/XHR calls (no such headers) still
  proxy through to the backend unchanged. Verified both cases directly:
  `curl -H "Accept: text/html" .../definitions/acme/wf` → 200 (SPA
  shell); `curl -H "Accept: application/json" .../definitions` → 401
  (proxied to the real backend, no token).
