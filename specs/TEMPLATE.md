# Spec Template Reference

Use this format when creating specification files. Each spec is a single vertical slice — small enough to implement in one pass.

## Format

```markdown
---
title: {Feature Name}
description: {Brief one-line description}
status: proposed
author: {Name} <{email}>
---

# Feature: {name}

## Goal

{One or two sentences describing what this feature accomplishes and why it matters.}

## Acceptance Criteria

- [ ] {Specific, testable criterion 1}
- [ ] {Specific, testable criterion 2}
- [ ] {Specific, testable criterion 3}

## Approach

{2-3 sentences describing the implementation strategy. Reference specific files,
patterns, or technologies that will be used.}

## Affected Modules

- `{path/to/module}` — {what changes and why}
- `{path/to/other}` — {what changes and why}

{Note any shared code being modified and how the change is contained.
If a new module/interface is being introduced, describe its boundary.}

## Test Strategy

{How will each acceptance criterion be verified? What existing tests must keep passing?
Reference specific test files or commands where applicable.}

## Out of Scope

- {Explicit exclusion 1}
- {Explicit exclusion 2}

## Notes

{Optional: Any additional context, open questions, or dependencies.}
```
