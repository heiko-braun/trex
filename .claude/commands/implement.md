---
name: implement
description: Implement features with phase checkpoints. Use after a spec exists in .claude/specs/ or when implementing a confirmed specification.
allowed-tools: Read, Write, Edit, Bash, Glob, Grep, TodoWrite, AskUserQuestion
---

# Implementation

You implement features as small, complete vertical slices with continuous testing.

## When to Activate

- After the spec skill has created and confirmed a specification
- When user says "implement" and a spec file exists
- When user explicitly references a spec file

## Workflow

### 1. Load Spec & Search for Context

Read the relevant `/specs/{feature}.md` file. If multiple specs exist and it's unclear which one, ask the user.

**Search for related code**: Before writing any code, use the spec's title and key terms to search the codebase for relevant existing code:

```bash
draft search "<spec title and key terms>" --limit 10
```

Review the search results to understand existing patterns, related modules, and potential conflicts. Use these results to inform your implementation approach.

Before writing any code, assess the change:

- **Which modules/files will this touch?** List them. If the spec has an "Affected Modules" section, verify it's still accurate.
- **Are we modifying shared code?** Changing a shared utility, interface, or base class affects every consumer. Flag this to the user.
- **Can we contain the change?** Prefer adding new files/functions over modifying widely-imported ones. Prefer narrow interfaces that isolate the new behaviour from the rest of the codebase.
- **Are we adding new dependencies?** Each dependency is a coupling point. Avoid unless clearly justified.

If the blast radius is wider than expected, flag it: *"This will touch N modules beyond what the spec anticipated. Want to proceed or restructure?"*


### 2. Implement

Implement the spec as **one integrated piece** — types, logic, wiring, and tests together. A small vertical slice doesn't need artificial separation into "foundation" and "core logic" and "integration" phases.

Important: Follow the design principles outlined in .principles/design-principles.md

Use TodoWrite to track progress against the spec's acceptance criteria.

**Design for modularity as you go:**
- Place new behaviour behind clear interfaces — functions, types, modules — so callers don't depend on implementation details.
- Avoid reaching into the internals of other modules. If you need something, use or extend its public interface.
- Keep new files/functions narrowly focused. One responsibility per unit.
- If a change to shared code is unavoidable, make the interface change first, verify existing tests still pass, then build the new behaviour on top.

**Test continuously:**
- After each meaningful change, run existing tests to catch regressions early.
- Write tests for new behaviour as you implement it, not after.
- If the project has a linter or build step, run it periodically — don't wait until the end.

**Follow design principles:**
- 

### 3. Verify & Complete

Before marking the feature complete:

1. Run the **full test suite** — not just new tests.
2. Run the **build/linter** if the project has one.
3. Re-read the spec's acceptance criteria and check each one explicitly.
4. Report: "Criterion met" or "Needs attention: {issue}" for each.

Only mark complete when all criteria pass and the build is green.

After successful implementation:
- **Update spec status** from `proposed` to `implemented`
- Check off completed acceptance criteria in the spec file
- Add any notes about implementation decisions

## Checkpoint Behaviour

Since each spec is a small vertical slice, heavy checkpointing is unnecessary.

**Do checkpoint** (pause and ask the user) when:
- The blast radius turns out wider than expected
- You face a design decision with trade-offs the user should weigh
- A test failure reveals a deeper issue that changes the approach

**Skip checkpoint** for:
- Normal forward progress within the spec
- Minor follow-up fixes
- Formatting/cleanup
- When user explicitly says "continue without asking"

## Recovery

If implementation is interrupted:
- TodoWrite preserves progress
- Spec file shows which criteria are done
- User can say "continue implementing {feature}" to resume
