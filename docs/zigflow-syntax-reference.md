# Zigflow Syntax Reference — Data Flow and `for` Loops

*Compact reference distilled from `docs/concepts/data-and-expressions.md` and
`docs/concepts/data-flow.md` in the Zigflow source repo, plus live testing
against a real Temporal server. Written after several silent-failure
incidents while building `workflows/pod-memory-trend.workflow.yaml` — read
this before writing new `for`/`set`/`export` logic to avoid repeating them.*

## The three data channels

| Variable | Backing field | Lifetime | Written by |
| --- | --- | --- | --- |
| `$output` | `state.Output` | Overwritten by every task | `output.as` (or the raw task result if no `output.as`) |
| `$context` | `state.Context` | Persists until explicitly replaced | `export.as` |
| `$data` | `state.Data` | Accumulates for the whole run, keyed by task name | `set` tasks (flat) and activity/`call` tasks (nested under task name) |

Mental model: `$output` is a baton passed hand to hand (only the latest task
holds it); `$context` is a shared whiteboard (each `export` **replaces** it
wholesale unless you merge); `$data` is an accumulating log (never replaced,
but only two task types write to it).

## Rule 1: `export.as` replaces `$context`, it does not merge

```yaml
# WRONG — this task's export wipes out every other $context key
export:
  as:
    myNewKey: ${ .SomeField }

# RIGHT — merge with the spread operator
export:
  as: '${ $context + { myNewKey: .SomeField } }'
```

Every `export.as` in a workflow that has more than one exporting task should
use the `$context + { ... }` merge form. A bare object literal silently
discards every key set by earlier tasks.

## Rule 2: `set` writes flat into `$data`; `call`/`run` write nested under the task name

```yaml
- fetchUser:
    call: http
    with: { method: get, endpoint: ... }
    # -> $data.fetchUser holds the response body

- setValue:
    set:
      userId: 42
    # -> $data.userId (NOT $data.setValue.userId)
```

Do not write `$data.<setTaskName>.<key>` — it will resolve to `null`.

## Rule 3: a `for` loop needs TWO separate `export.as` blocks — one inside for `while`, one outside for everything after

This is the one that costs the most debugging time, because it looks
redundant until you remove either half.

Inside a `for` task's `do:` block, each iteration runs as a **child
workflow**. A nested `export.as` on the task inside `do:` writes to that
child's own `$context`, which Zigflow propagates from **iteration to
iteration** (so `while`, checked before the next iteration starts, can read
it) — but this `$context` is **loop-private** and is discarded the moment
the loop exits. It is required for a `while` condition that depends on
something an inner task computed, but it is invisible to any task that
runs after the loop.

```yaml
- pollTask:
    for:
      in: ${ 60 }
    while: ${ ($context.done // false) == false and $data.index < 59 }
    # (2) OUTER export — required to surface anything past the loop.
    #     `.` here is the loop's aggregated result: an array of every
    #     iteration's raw task output, in order. Use `.[-1]` for the last.
    export:
      as: '${ $context + { done: (.[-1].done // $context.done), result: (.[-1].result // $context.result) } }'
    do:
      - pollOnce:
          call: activity
          with: { name: poll-agent-task, taskQueue: ..., arguments: [...] }
          # (1) INNER export — required for `while` to see the latest
          #     iteration's result at all. Discarded once the loop exits.
          export:
            as: '${ $context + { done: .Done, result: .Result } }'
```

Confirmed live against a real Temporal dev server: dropping the inner
export makes `while` see a value that's always stale (observed: a loop that
should stop after ~8 iterations instead ran 30+ times, because the
condition it depends on was never updated between iterations). Dropping
the outer export makes the result unreachable after the loop exits
(observed: the sibling task's read of it silently evaluated to
empty/`null` even though the loop ran and finished correctly). You need
both, every time.

## Rule 4: the document-level `output:` block is a schema hint only — never applied at runtime

```yaml
output:
  as: ${ "some computed string" }   # <-- silently never evaluated
```

The workflow's actual return value is `$output` after the **last task in
`do:`** runs — same rule as any other task boundary. To shape what the
workflow returns, put `output.as` on the final task itself:

```yaml
do:
  - ...
  - formatResult:
      output:
        as: ${ "Memory trend:\n" + $context.opsBuddyResult + ... }
      set:
        done: true   # any task body works; set is a convenient no-op carrier
```

The top-level `output.schema` (if present) still validates the shape of
whatever `output.as` on the last task produces — that part of the
document-level `output:` block is real, only `output.as` at that level is
inert.

## Quick checklist for a new `for` + poll-loop pattern

- [ ] Every `export.as` after the first uses `$context + { ... }`, not a bare object.
- [ ] A `for` loop whose `while` depends on a value computed inside `do:` has a nested `export.as` on that inner task (feeds `while` between iterations).
- [ ] The `for` task itself ALSO has its own `export.as` reading `.[-1].<field>` for anything the rest of the workflow needs after the loop — the inner export alone does not survive loop exit.
- [ ] Nothing downstream reads `$data.<forTaskName>` directly — it's unreachable outside the loop (see Rule 2's `set` vs `call` distinction; `for` writes neither).
- [ ] The workflow's real return value comes from `output.as` on the literal last task in the top-level `do:` list, not from the document-level `output:` block.
- [ ] `set` task values are read back as `$data.<key>`, never `$data.<setTaskName>.<key>`.

## Sources

- `docs/concepts/data-and-expressions.md`, `docs/concepts/data-flow.md`,
  `docs/dsl/tasks/for.md` in the Zigflow source repo (`../zigflow/docs/docs/`)
  — read these first; they are authoritative and (as of this writing) not
  yet mirrored on zigflow.dev's public site.
- `pkg/zigflow/tasks/task_builder_for.go`, `task_builder_do.go`,
  `pkg/utils/state.go` in the Zigflow Go module — ground truth for the
  `$output`/`$context`/`$data` → `state.Output`/`state.Context`/`state.Data`
  binding (see `State.GetAsMap()`).
