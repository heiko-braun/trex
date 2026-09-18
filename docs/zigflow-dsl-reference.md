# Zigflow DSL Reference — Task Types

*Complete reference for every Zigflow task type, distilled from the Zigflow
source repo's docs (`../zigflow/docs/docs/dsl/tasks/`). For data-flow
subtleties (`$output`/`$context`/`$data`, `for`-loop export gotchas,
document-level `output:` being inert) see
[docs/zigflow-syntax-reference.md](./zigflow-syntax-reference.md) — this
file covers task syntax, that one covers data semantics.*

## Document shape

```yaml
document:
  dsl: 1.0.0
  taskQueue: <temporal-task-queue>   # NOT the Temporal namespace
  workflowType: <workflow-name>      # ignored if do: has multiple top-level workflows
  version: 0.0.1
  title: Optional Title
  summary: Optional summary text
input:
  schema:
    document:
      type: object
      properties: { ... }
      required: [ ... ]
do:
  - taskName:
      <task-type-keyword>: ...
output:
  schema: { ... }   # still validated at runtime
  as: ${ ... }       # NEVER applied at runtime — see zigflow-syntax-reference.md Rule 4
```

Multiple top-level `do:` entries with their own nested `do:` create multiple
independent workflow types, named after the task key (`document.workflowType`
is then ignored).

## Common task properties (every task type)

| Property | Description |
| --- | --- |
| `output.as` | Runtime expression shaping `$output` from the task's raw result. No `output.as` → raw result becomes `$output` unchanged. |
| `output.schema` | JSON Schema validated against the shaped output. |
| `export.as` | Runtime expression shaping `$context`. **Replaces** `$context` wholesale — use `${ $context + {...} }` to merge. |
| `export.schema` | JSON Schema validated against the exported context. |
| `if` | Runtime expression; skips the task if falsy. |
| `then` | [Flow directive](#flow-directives) run after the task completes. |
| `metadata.activityOptions` | Temporal `ActivityOptions` (timeouts, retry policy) for activity-backed tasks (`call:`, `run: container/script/shell`). |
| `metadata.timeout` | `listen` task's await timeout (default 60s). |

### Flow directives

| Directive | Effect |
| --- | --- |
| `"continue"` | Proceed to the next task (default). |
| `"exit"` | End the current scope (may end the whole workflow if in the root `do:`). |
| `"end"` | Gracefully end the workflow explicitly. |
| `<task-name>` (string) | Jump to the named task. **Must be in the same scope/depth** — cannot target a task at a different nesting level. |

---

## `call` — invoke an activity, HTTP, or gRPC endpoint

| Field | Required | Notes |
| --- | --- | --- |
| `call` | yes | `activity`, `http`, or `grpc` |
| `with` | no | parameters for the chosen call type |

### `call: activity`

```yaml
- fetchProfile:
    call: activity
    with:
      name: activitycall.FetchProfile   # exact, case-sensitive Temporal activity name
      arguments: [${ $data.requestedUserId }, ${ $data.requestId }]  # positional args
      taskQueue: activity-call-worker
```

`arguments` is a **list**, passed positionally to the Go activity function.
For an activity with a single struct parameter, pass one list element that's
a map matching the struct's JSON field names.

### `call: http`

```yaml
- getUser:
    call: http
    with:
      method: get
      endpoint: https://api.example.com/users/2
      headers: { }          # optional
      body: { }              # optional; JSON-encoded unless Content-Type is form-urlencoded
      query: { }              # optional
      output: content          # raw | content | response (default: content)
      redirect: false           # treat 3xx as error if false (default)
```

Trex's `internal/validator` policy does not restrict `call: http`/`call:
grpc`/`call: activity` — only `run: shell`/`run: script` are rejected (see
[Run](#run--shell-script-container-or-child-workflow)).

**Gotchas**
- HTTP status handling: `200–299` success; `300–399` non-retryable unless
  `redirect: true`; `408`/`429` retryable; `400–499` non-retryable by
  default; `500–599` retryable by default; `501` non-retryable.
- A retryable response with a valid `Retry-After` header delays the next
  retry to that time instead of normal backoff — uncapped, so bound overall
  execution with `scheduleToCloseTimeout`.
- Activity/HTTP/gRPC/container/script/shell tasks register on the worker
  under a path-derived name (`<workflowType>.<taskName>`, or
  `<workflowType>.<forName>.<taskName>` when nested) — this is what shows
  up as the Temporal SDK metric `activity_type` label.

### `call: grpc`

```yaml
- greet:
    call: grpc
    with:
      proto:
        endpoint: file:///path/to/service.proto   # must be readable by the worker at runtime
      service:
        name: providers.v1.BasicService
        host: grpc
        port: 3000
      method: Command1
      arguments:
        input: hello world
```

---

## `do` — sequential subtasks

```yaml
do:
  - stepOne:
      set: { key: value }
  - stepTwo:
      wait: { seconds: 5 }
```

`do` is implicit at the workflow root and used by `for`/`fork`/`try` to
define their nested task lists. Every task name must be unique within its
own scope (sibling list) — duplicates fail validation.

---

## `for` — iterate over a collection

| Field | Required | Notes |
| --- | --- | --- |
| `for.in` | yes | expression producing the collection: array, map, **or a plain integer count** (`${ 60 }` = 60 iterations) |
| `for.each` | no | current-item variable name, default `item` |
| `for.at` | no | current-index variable name, default `index` |
| `while` | no | expression checked **after each iteration**; stops the loop when false |
| `do` | yes | tasks to run per iteration, as a **child workflow** |

```yaml
- pollTask:
    for:
      each: attempt
      in: ${ 60 }
      at: index
    while: ${ $data.index < 59 }
    do:
      - waitAndPoll:
          wait: { seconds: 3 }
```

**Gotchas** (see [zigflow-syntax-reference.md](./zigflow-syntax-reference.md)
Rule 3 for the full data-flow breakdown — summary below):
- Each iteration is a real Temporal **child workflow** (`<parentID>_for_<index>`
  in the Temporal UI — seeing two workflow IDs per `for` task is expected,
  not a bug).
- `while` cannot prevent the first iteration.
- A `for` loop needs **two separate** `export.as` blocks to both drive
  `while` and surface a result afterward: one nested inside `do:` (feeds
  `while` between iterations, discarded on exit) and one on the `for` task
  itself (the only thing readable by sibling tasks after the loop; sees the
  aggregated per-iteration array as `.`, so use `.[-1].<field>` for the
  final result).
- Large loops create many child workflows — mind Temporal history limits.

---

## `fork` — run subtasks concurrently

| Field | Required | Notes |
| --- | --- | --- |
| `fork.branches` | no | map of tasks to run concurrently, each as a child workflow |
| `fork.compete` | no | `false` (default): wait for all, output is an array in declaration order. `true`: return only the fastest branch's output, cancel the rest |

```yaml
- raiseAlarm:
    fork:
      compete: false
      branches:
        - callNurse:
            call: http
            with: { method: get, endpoint: https://... }
        - multiStep:
            do:
              - wait1: { wait: { seconds: 3 } }
              - wait2: { wait: { seconds: 2 } }
```

**Gotchas**
- Competing mode cancels every other branch; their side effects (HTTP calls
  already sent) are **not** rolled back.
- Cancelling the parent workflow cancels every branch; the fork waits for
  all branches to reach `CANCELED` before the parent closes.

---

## `listen` — await an external event (query / signal / update)

| Field | Required | Notes |
| --- | --- | --- |
| `listen.to.one` / `.any` / `.all` | one of these required | which event(s) to wait for |
| event filter `.with.id` | yes | maps to the Temporal signal/query/update name |
| event filter `.with.type` | yes | `query`, `signal`, or `update` |
| event filter `.with.data` | no | payload shape (ignored for `query`) |
| event filter `.with.acceptIf` | no | (update only) expression gating whether to accept |

```yaml
# Signal — fire-and-forget
- approveListener:
    metadata:
      timeout: 10s   # default 60s
    listen:
      to:
        one:
          with:
            id: approve
            type: signal
```

```yaml
# Query — non-blocking read
- queryState:
    listen:
      to:
        one:
          with:
            id: get_state
            type: query
            data: { progress: ${ $data.progressPercentage } }
```

```yaml
# Update — read/write, can require multiple named updates via `all`
- callDoctor:
    listen:
      to:
        all:
          - with: { id: temperature, type: update, acceptIf: ${ $data.temperature > 38 } }
          - with: { id: bpm, type: update, acceptIf: ${ $data.bpm < 60 or $data.bpm > 100 } }
```

**Gotchas**
- Default timeout is 60s; a timeout raises a
  `https://zigflow.dev/spec/1.0.0/errors/timeout` error (same type `raise`
  uses) — wrap in `try` to catch, or set `metadata.timeout` for long waits.
- Queries never block the workflow.
- Signal/update payloads land at `$data.<taskName>`, not `$output`.

---

## `raise` — explicitly fail with a structured error

| Field | Required |
| --- | --- |
| `raise.error.type` | yes — URI, prefer [standard types](#standard-error-types) |
| `raise.error.status` | yes — HTTP-style status code |
| `raise.error.instance` | no |
| `raise.error.title` | no — string or expression |
| `raise.error.detail` | no — string or expression |

```yaml
- bug:
    raise:
      error:
        type: https://zigflow.dev/spec/1.0.0/errors/communication
        status: 400
```

### Standard error types

| Type suffix | Default status | Use for |
| --- | :---: | --- |
| `errors/configuration` | 400 | bad config/env/params |
| `errors/validation` | 400 | input/schema validation |
| `errors/expression` | 400 | bad runtime expression |
| `errors/authentication` | 401 | auth failures |
| `errors/authorization` | 403 | insufficient permissions |
| `errors/timeout` | 408 | task/external-call timeout |
| `errors/communication` | 500 | network/external-service errors |
| `errors/runtime` | 500 | unexpected runtime failures |

**Gotcha**: an uncaught `raise` always terminates the current path — wrap
in `try` to catch it.

---

## `run` — shell, script, container, or child workflow

**Trex-specific: `run: shell` and `run: script` are rejected by
`internal/validator`'s policy check** (`PolicyViolation`) — do not use them
in workflows published to this server. `run: container` and `run: workflow`
are not restricted by trex's policy (but see their own runtime prerequisites
below).

| Field | Required | Notes |
| --- | --- | --- |
| `run.container` | one of these four required | see below |
| `run.script` | | **rejected by trex policy** |
| `run.shell` | | **rejected by trex policy** |
| `run.workflow` | | run another Zigflow workflow as a child |
| `await` | no | workflow-only; `false` = fire-and-forget, can't read the result. Default `true`. |

### `run: container`

```yaml
- container:
    run:
      container:
        image: alpine
        pullPolicy: ifNotPresent   # ifNotPresent | always | never
        arguments: [env]
        environment: { hello: world }
        lifetime:
          cleanup: always   # always | never
```

Runtime (`docker` default, or `kubernetes`) is selected on the **worker**
via `--container-runtime`, not in the YAML. Kubernetes mode needs
in-cluster API access + RBAC to manage Jobs and read pod logs.

### `run: workflow`

```yaml
- callChildWorkflow1:
    run:
      workflow:
        type: child-workflow1        # looked up by name on the SAME task queue
        input: ${ ... }               # optional, validated against the child's input schema
```

`namespace`/`version` fields exist for OWS compatibility only and are
ignored by Zigflow.

---

## `set` — write data into `$data`

| Field | Required |
| --- | --- |
| `set` | yes — a map of key/value pairs |

```yaml
- baseData:
    export:
      as: ${ . }
    set:
      progress: 0
      envvar: ${ $env.EXAMPLE_ENVVAR }
      uuid: ${ uuid }                 # safe here — wrapped in a Temporal side effect
      inputUserId: ${ $input.userId }
```

Writes land flat at `$data.<key>` — **not** nested under `$data.<setTaskName>`
(unlike `call`/`run` activity tasks). See
[zigflow-syntax-reference.md](./zigflow-syntax-reference.md) Rule 2.

**Always generate non-deterministic values (`uuid`, `timestamp`,
`timestamp_iso8601`) inside a `set` task.** Using them directly in another
task (e.g. an HTTP body) produces a different value on Temporal replay and
raises a Non-Determinism Error.

---

## `switch` — conditional branching

| Field | Required |
| --- | --- |
| `switch` | yes — ordered list of cases |
| case `.when` | no — boolean expression; omit for the (single allowed) default case |
| case `.then` | yes — a [flow directive](#flow-directives) |

```yaml
- routeOrder:
    switch:
      - electronic:
          when: ${ $input.orderType == "electronic" }
          then: processElectronicOrder
      - physical:
          when: ${ $input.orderType == "physical" }
          then: processPhysicalOrder
      - default:
          then: handleUnknownType
```

**Gotchas**
- Cases evaluate in declaration order; first truthy `when` wins.
- No default case + no match → falls through to the next task in `do:`
  silently. Always include an explicit default.
- `then` targets must exist in the same scope/depth.
- A `switch` with no `output.as` of its own leaves `$output` unchanged from
  the previous task (it's a routing task, not a data-producing one) — add
  `output.as` if you need to change it.

---

## `try` / `catch` — error handling

| Field | Required |
| --- | --- |
| `try` | yes — tasks to attempt, run as a child workflow |
| `catch.do` | yes — tasks to run if `try` raises |
| `catch.as` | no — `$data` key for the caught error, default `error` |

```yaml
- user:
    try:
      - getUser:
          call: http
          output:
            as: { user: ${ . } }
          with: { method: get, endpoint: https://.../users/2000 }
    catch:
      do:
        - setError:
            output:
              as: { error: ${ . } }
            set:
              message: some error
```

**Gotchas**
- `catch` catches **everything** — no filtering by error type; inspect the
  caught error object inside the catch block if you need to branch on it.
- `try`'s body runs as its own child workflow with independent history;
  inner tasks' own retry policies still apply before `catch` triggers.

---

## `wait` — durable pause

Compiles to a Temporal Durable Timer either way.

### Duration form (OWS-portable)

```yaml
- pause:
    wait:
      seconds: 5   # days | hours | minutes | seconds | milliseconds — literal or expression
```

Each field accepts a runtime expression (Zigflow extension):
`wait: { seconds: ${ $input.cooldownSeconds } }`.

### `until` form (Zigflow extension)

```yaml
- waitForDeadline:
    wait:
      until: 2026-12-31T23:59:59Z    # RFC 3339 string, or an expression resolving to one
```

`until` and the duration fields are mutually exclusive — mixing them fails
validation.

**Gotchas**
- A past `until` is a silent no-op (continues immediately, logged at debug
  only).
- Wrong-typed expressions fail loudly (no coercion) — good, but check your
  types.
- **`uuid`/`timestamp`/`timestamp_iso8601` are rejected inside a `wait`
  expression** (non-deterministic on replay). Compute them in a preceding
  `set` task and reference the result.
- No maximum duration, but very long timers grow workflow history.

---

## Idempotency keys

Zigflow does not generate or enforce idempotency keys. For any operation
with side effects that Temporal might retry at-least-once, generate one in
a `set` task and pass it explicitly:

```yaml
- createIdempotencyKey:
    set:
      idempotencyKey: ${ uuid }
- callApi:
    call: http
    with:
      method: POST
      endpoint: https://api.example.com/orders
      headers:
        x-idempotency-key: ${ $data.idempotencyKey }
```

Choose the key's scope deliberately: `${ uuid }` (unique per execution),
`${ $data.workflow.workflow_execution_id }` (stable for the run), or
`${ $input.orderId }` (stable across executions of the same logical order).

---

## Determinism model (why all this matters)

Zigflow compiles YAML into an ordinary Temporal workflow at worker startup;
Temporal replays it from event history on any restart. Determinism is
enforced structurally, not by convention:

1. Control flow (`do`, `for`, `switch`, `try`, `fork`) is data — the same
   YAML always produces the same workflow shape.
2. Side effects (`call:`, `run: container/script/shell/workflow`) are
   Temporal activities or child workflows — recorded once, replayed from
   history without re-executing.
3. Validation runs fully before any Temporal connection is attempted;
   invalid or non-deterministic constructs are rejected at that point, not
   at runtime.

`zigflow run -f workflow.yaml` sequence: load YAML → validate → connect to
Temporal → register the compiled workflow on `document.taskQueue` → poll
for tasks. **Editing the YAML while a worker is running has no effect until
restart** — trex's own `internal/zigflowadapter`/`internal/workflowworker`
re-does this build step on every publish instead, which is why trex doesn't
need a standalone `zigflow run` process for definitions it hosts itself.

## Sources

All from the Zigflow source repo (`../zigflow/docs/docs/`), current as of
this writing and not yet fully mirrored on zigflow.dev's public site:

- `dsl/tasks/intro.md` — task overview, runtime expression variables/functions, flow directives, idempotency keys
- `dsl/tasks/call.md`, `do.md`, `for.md`, `fork.md`, `listen.md`, `raise.md`, `run.md`, `set.md`, `switch.md`, `try.md`, `wait.md` — one file per task type
- `concepts/overview.md`, `how-zigflow-runs.md`, `durable-execution-in-yaml.md` — execution model and determinism
- `concepts/data-and-expressions.md`, `data-flow.md` — see [zigflow-syntax-reference.md](./zigflow-syntax-reference.md), which covers these in depth
