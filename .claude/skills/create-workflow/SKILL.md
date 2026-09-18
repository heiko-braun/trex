---
name: create-workflow
description: >
  Create a new Zigflow workflow for the trex workflow-server that chains
  managed agents as Temporal activities. Use when the user asks to build,
  author, or add a new workflow, especially one that calls one or more
  managed agents in sequence. Discovers available agents, asks the user
  which to wire in and in what order, registers any that aren't yet
  registered, writes the workflow YAML following this repo's Zigflow
  data-flow rules, validates it, and publishes it.
---

# Create Workflow

Build a new Zigflow workflow definition for trex's workflow-server, wiring
in managed agents as `invoke-agent`/`poll-agent-task` activity calls.

Read `docs/zigflow-syntax-reference.md` and `docs/zigflow-dsl-reference.md`
first if you haven't already this session — the data-flow rules there
(`export.as` merge semantics, the dual inner/outer `export.as` a `for` loop
needs, `$output` vs `$context` vs `$data`) are not optional background, they
are the exact source of every past failure building this pattern. Do not
improvise around them.

## Prerequisites

The workflow-server, Postgres, and Temporal must already be running — see
"Running a workflow: startup order" in `CLAUDE.md`. If `curl` calls below
fail with connection refused, that's the first thing to check, not a code
bug.

```bash
lsof -i :8080 -sTCP:LISTEN   # workflow-server
pgrep -f "temporal server start-dev"
podman ps --format "{{.Names}}" | grep -x trex-postgres
```

Every API call below needs a Bearer token:

```bash
agentctl whoami >/dev/null 2>&1   # refresh the cached session first
TOK=$(./scripts/workflow-server-token.sh)
```

## Workflow

### 1. Discover available agents

```bash
curl -s -H "Authorization: Bearer $TOK" http://localhost:8080/agents | python3 -m json.tool
```

This calls the control plane live (stateless discovery — see
`specs/agent-discovery-and-registration.md`), returning every agent the
caller's token can see: `id`, `name`, `type`.

### 2. Ask the user what to build — ONE question at a time

Do not guess the storyline. Ask, in order:

1. **What should this workflow do?** (one or two sentences — the goal)
2. **Which agents, in what order?** Show the discovered list (id + name +
   type) and ask the user to pick and sequence them. A workflow can also
   have zero agent stages (pure `set`/`call: http`/etc.) — don't assume
   every workflow needs one.
3. **What's the input?** (e.g. a pod name, a ticket ID) — becomes the
   workflow's `input.schema`.
4. **How does each agent's prompt get built from the input and prior
   agents' results?** — this shapes each stage's `Input` expression.
5. **What's the final output?** — usually the last agent's result, or a
   synthesis/summary if there's an "editor"-style final agent.

Do not batch these into one message — this repo's `/spec` skill's
one-question-at-a-time convention applies here too.

### 3. Register any agent that isn't already registered

```bash
curl -s -H "Authorization: Bearer $TOK" http://localhost:8080/agents/registered | python3 -m json.tool
```

For each agent the user picked that isn't in that list:

```bash
./scripts/register-agent.sh <agent-id> "<agent-name>"
```

This starts a real Temporal worker on task queue `agent-<id>` — confirm it
shows `"running":true` in the registered list afterward, or in the Temporal
UI under **Task Queues** → `agent-<id>` → **Pollers**.

### 4. Write the workflow YAML

For each agent stage, use this exact three-part pattern (see
`workflows/pod-memory-trend.workflow.yaml` for a complete worked example
with two chained stages):

```yaml
- ask<Stage>:
    call: activity
    with:
      name: invoke-agent
      taskQueue: agent-<agent-id>
      arguments:
        - Input: ${ <expression building the prompt from $input/$context> }
    metadata:
      activityOptions:
        startToCloseTimeout: { seconds: 60 }
        scheduleToCloseTimeout: { minutes: 3 }
    export:
      as: '${ $context + { <stage>Done: .Done, <stage>Result: .Result, <stage>TaskID: .TaskID } }'

- poll<Stage>:
    for:
      each: attempt
      in: ${ 60 }
      at: index
    while: ${ ($context.<stage>Done // false) == false and $data.index < 59 }
    export:
      as: '${ $context + { <stage>Done: (.[-1].Done // $context.<stage>Done), <stage>Failed: (.[-1].Failed // false), <stage>Result: (.[-1].Result // $context.<stage>Result) } }'
    do:
      - cooldown:
          wait: { seconds: 8 }
      - poll<Stage>Task:
          call: activity
          with:
            name: poll-agent-task
            taskQueue: agent-<agent-id>
            arguments:
              - TaskID: ${ $context.<stage>TaskID }
          metadata:
            activityOptions:
              startToCloseTimeout: { seconds: 15 }
          export:
            as: '${ $context + { <stage>Done: .Done, <stage>Failed: .Failed, <stage>Result: .Result } }'
```

Non-negotiable details (each has cost real debugging time before — see
`docs/zigflow-syntax-reference.md` Rules 1 and 3):

- **Every `export.as` merges**: `${ $context + { ... } }`, never a bare
  object literal — a bare literal silently wipes every other key in
  `$context`.
- **Every `for` loop needs BOTH exports**: the nested one inside `do:`
  (feeds `while` between iterations) AND the `for` task's own top-level
  one (the only way anything survives past the loop, read via `.[-1]`).
  Dropping either breaks a different thing — the loop either never stops
  or the result is unreachable afterward.
- **No `Token` field anywhere** — `internal/agents.AgentctlTokenSource`
  fetches a fresh token server-side per activity call now; do not add a
  `callerToken` input or thread a token through `arguments`.
- The final task's `output.as` is what the workflow actually returns —
  the document-level top-level `output:` block is parsed but **never
  applied at runtime**. Put `output.as` on the literal last task in `do:`.

### 5. Validate before publishing

```bash
mkdir -p cmd/validate-check
cat > cmd/validate-check/main.go << 'EOF'
package main

import (
	"fmt"
	"os"

	"github.com/heiko-braun/trex/internal/validator"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("read error:", err)
		os.Exit(1)
	}
	if err := validator.Validate(b); err != nil {
		fmt.Println("VALIDATION FAILED:", err)
		os.Exit(1)
	}
	fmt.Println("VALID")
}
EOF
go run ./cmd/validate-check workflows/<name>.workflow.yaml
rm -rf cmd/validate-check
```

This runs the exact same schema + determinism + policy checks the Publish
API runs (rejects `run: shell`/`run: script`), so a YAML syntax mistake or
policy violation surfaces before publish rather than as an opaque 400.

### 6. Publish

```bash
TOK=$(./scripts/workflow-server-token.sh)
python3 -c "
import json
print(json.dumps({'tenant': '<tenant>', 'name': '<name>', 'yaml': open('workflows/<name>.workflow.yaml').read()}))
" > /tmp/publish.json
curl -s -X POST -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" \
  --data-binary @/tmp/publish.json \
  http://localhost:8080/definitions | python3 -m json.tool
rm -f /tmp/publish.json
```

A successful publish immediately starts a real Temporal worker for this
workflow (see `internal/workflowworker.Supervisor` /
`specs/workflow-worker-supervisor.md`) — no separate `zigflow run` step,
and do not start one; it competes with the workflow-server's own worker on
the same task queue and produces confusing, nondeterministic routing.

### 7. Hand off to `run-workflow`

Once published, use the `run-workflow` skill to actually execute it and
track it to completion — don't just leave it published-but-unrun.

## Beads

Every workflow YAML addition/change is a meaningful step per this
project's `CLAUDE.md` rule — create a beads ticket before or as soon as
work starts, with `BEADS_ACTOR=claude`, and close it with a comment
describing what was built and how it was verified.
