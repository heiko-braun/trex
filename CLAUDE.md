# Resource and References

- zigflow: https://zigflow.dev/docs/
- temporal: https://docs.temporal.io/
- managed agents: https://docs.agents.sixt.cloud/
- managed agents sources: ../com.sixt.service.managed-agents/
- managed agent API endpoint: https://api.agents.sixt.cloud

# Tools and CLI extensions
- agentctl tool: $(which agentctl)
- zigflow: $(which zigflow)
- temporal: $(which temporal) 

# Procedures

## Beads

This project uses **bd** (beads) for issue tracking. Run `bd prime` for full workflow context.
You can open the UI like this: `bdui start --open`

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work atomically
bd close <id>         # Complete work
bd dolt push          # Push beads data to remote
```

### Rule: every step/enhancement gets a ticket

For this demo project specifically, every meaningful step or enhancement
(architecture doc, workflow YAML change, a fix discovered while debugging a
run, a new agent added to the storyline, etc.) must have a corresponding
beads ticket that:

- is created **before** or as soon as the work starts (`bd create`),
- moves through real states as work progresses (`open` -> `in_progress` ->
  `closed`, using `bd update`/`bd close`), not created and closed in one shot
  after the fact,
- carries enough detail in its description/comments (`bd comment`) for a
  human to understand *what was tried, what broke, and what fixed it* —
  e.g. "default activity StartToCloseTimeout (15s) too short for K8s Helper's
  ~90s response; added activityOptions.startToCloseTimeout: 3m" — not just
  "fix timeout".

The goal is a readable trail of how the demo was built, not just a changelog
of file diffs.

### Rule: every spec gets a ticket

Every spec created under `specs/` (via the `/spec` skill) and its
implementation must have a corresponding beads ticket, following the same
create-before-work / real-state-transitions / detailed-comments rules above.
One ticket per spec is enough — track the whole implementation under it
rather than opening a separate ticket per file touched.

### Actor attribution

All `bd` commands (Claude's, in this session and future ones) must be run with
`BEADS_ACTOR=claude` so comments/creates are attributed to "claude", not to
the human user (`bd`'s default actor resolution falls back to git
`user.name`/`$USER`, which would otherwise misattribute Claude's own audit
trail entries to Heiko). Example: `BEADS_ACTOR=claude bd comment <id> "..."`.

## Retrieving a token

You can use `agentctl login|whoami` to fetch and refresh tokens.

## Talkig to agent on manged agents platform

Agentctl for agent discovery and it's a2a support to dispatch task

# APIKeys, etc

Found in .env

## Connecting to Temporal

Temporal Cloud (`TEMPORAL_NAMESPACE_ENDPOINT` / `TEMPORAL_KEY` in `.env`) is
unreachable from this machine's network — a Cato Networks TLS-inspection
proxy strips the ALPN extension gRPC requires, breaking both `temporal` and
`zigflow run` with an identical handshake error
(`missing selected ALPN property`). Confirmed via `openssl s_client`: the
proxy terminates TLS and re-signs with its own CA
(`issuer=CN=Cato-Networks-Server-...`) instead of passing through to
`*.tmprl.cloud`.

**Use a local Temporal dev server instead** (`temporal server start-dev`) —
loopback traffic bypasses the proxy entirely. Revisit Temporal Cloud only
after Cato inspection is disabled/excluded for `*.tmprl.cloud:7233`.

```bash
temporal server start-dev --ui-port 8233
# Temporal Server: localhost:7233
# Temporal UI:     http://localhost:8233
```

No `--tls` / `--api-key` needed against localhost. `zigflow run` connects the
same way via `--temporal-address localhost:7233` (omit the `--temporal-tls`
and `--temporal-api-key` flags).


<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:1105d646 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
