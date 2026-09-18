---
name: run-workflow
description: >
  Start a published Zigflow workflow execution on trex's workflow-server
  and track it to completion, decoding and reporting the final result.
  Use when the user asks to run, execute, invoke, trigger, or kick off a
  workflow, or asks for the status/result of one already running.
---

# Run Workflow

Start a Temporal execution of an already-published trex workflow and poll
it to a terminal state, then decode and report the result.

## Prerequisites

- The workflow must already be published (`GET /definitions` lists it) and
  every agent it calls must be registered (`GET /agents/registered`) — see
  the `create-workflow` skill if either isn't true yet. Starting an
  execution against an unregistered agent's task queue doesn't fail fast;
  it sits waiting for a poller that never shows up until
  `scheduleToCloseTimeout` expires.
- Temporal CLI (`temporal`) must be on `PATH` and pointed at the same
  server the workflow-server uses (`localhost:7233` for local dev).

## 1. Confirm the target and gather input

Ask the user (if not already clear from context):
- Which workflow (`taskQueue` + `workflowType` — usually the same value,
  e.g. `pod-memory-trend`)?
- What input does it need? Check the published definition's
  `input.schema` if unsure:

```bash
TOK=$(./scripts/workflow-server-token.sh)
curl -s -H "Authorization: Bearer $TOK" \
  http://localhost:8080/definitions/<tenant>/<name> | python3 -m json.tool
```

## 2. Start the execution

```bash
WFID="<workflow-name>-$(date +%s)"
temporal workflow start \
  --task-queue <taskQueue> \
  --type <workflowType> \
  --workflow-id "$WFID" \
  --input '<json-matching-input-schema>'
```

Report the `WorkflowId`/`RunId` to the user immediately — don't wait
silently through the whole run before saying anything.

## 3. Poll to a terminal state

Use `Monitor`'s until-loop pattern (or a background-tracked loop), not a
tight sleep chain in the foreground — these workflows run real agents and
can take 1-3+ minutes per stage. Check every 8-10s:

```bash
until temporal workflow describe --workflow-id "$WFID" 2>&1 | grep -qE "Status\s+(COMPLETED|FAILED|TERMINATED|TIMED_OUT|CANCELED)"; do
  sleep 8
done
temporal workflow describe --workflow-id "$WFID" 2>&1 | grep -i "^  Status"
```

While waiting, `temporal workflow describe` also shows **Pending Child
Workflows** — each `_for_N` child is one iteration of a `poll-agent-task`
loop. Seeing the child workflow ID's suffix climb (`_for_3`, `_for_11`,
...) is a live progress signal, not a sign something is stuck. Only worry
if the same `_for_N` value persists across several checks — that suggests
a stuck poll (check whether the agent it's polling has actually finished,
via `agentctl a2a task get <task-id> --agent <agent-id>` if you have the
task ID, or check the workflow-server's own log for repeated identical
poll attempts).

## 4. Decode the result

`temporal workflow describe` only shows status, not the payload. Get the
actual result:

```bash
temporal workflow show --workflow-id "$WFID" --output json 2>&1 | python3 -c "
import json, sys, base64
data = json.load(sys.stdin)
for e in data.get('events', []):
    if e.get('eventType') == 'EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED':
        for p in e['workflowExecutionCompletedEventAttributes']['result']['payloads']:
            print(base64.b64decode(p['data']).decode())
    if e.get('eventType') == 'EVENT_TYPE_WORKFLOW_EXECUTION_FAILED':
        f = e['workflowExecutionFailedEventAttributes']['failure']
        print('FAILED:', f.get('message'))
        def show(fail, depth=0):
            ea = fail.get('encodedAttributes', {}).get('data')
            if ea:
                print(' ' * depth, base64.b64decode(ea).decode())
            if fail.get('cause'):
                show(fail['cause'], depth + 2)
        show(f)
"
```

Report the decoded result to the user directly — don't just say "it
completed," show what it actually returned.

## 5. If it failed

Common causes, cheapest checks first:

1. **`unexpected status 401`/`403` in the failure message** — a fresh
   token wasn't available server-side. Confirm `agentctl whoami` succeeds
   on the machine running the workflow-server (the server shells out to it
   per `internal/agents.AgentctlTokenSource`); this is a documented
   local-only placeholder, not a bug to chase further (see
   `specs/agent-discovery-and-registration.md` Notes).
2. **`context deadline exceeded` / timeout on `invoke-agent`** — the
   agent's `message:send` call is legitimately slow for this prompt.
   Check the activity's configured `startToCloseTimeout` isn't tighter
   than the agent's real latency (test with `agentctl a2a send <id>
   "<prompt>" --async` to see how long the task actually takes).
3. **`ACTIVITY_TASK_TIMED_OUT` with "not enough time to schedule next
   retry"** — `scheduleToCloseTimeout` was exhausted by retries against a
   slow call. Raise it, or fix cause #2 first.
4. **Loop never terminates / same `_for_N` forever** — almost certainly a
   missing `export.as` on the `for` task itself (see
   `docs/zigflow-syntax-reference.md` Rule 3) — the `while` condition
   isn't seeing an updated value.
5. **Final result is empty/null despite the loop completing** — the
   opposite half of the same issue: the nested per-iteration `export.as`
   updated `while` correctly but nothing surfaced the result past the
   loop exit. Check the `for` task has its own `export.as` reading
   `.[-1].<field>`.

If none of these explain it, decode the full failure chain (step 4's
`show()` helper walks nested `cause`s) before guessing further.
