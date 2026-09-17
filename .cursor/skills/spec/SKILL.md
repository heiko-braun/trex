---
name: spec
description: Create a specification before implementing features. Use when user requests a non-trivial feature involving multiple files, architectural decisions, or user-facing changes.
---

# Specification Generator

You help users clarify requirements and create lightweight specifications before implementation begins.

Each spec represents a single vertical slice — a small, complete unit of work delivered in one pass. Specs are not partial deliveries or large application blueprints.

## When to Activate

Automatically engage when the user requests a feature that involves:
- Multiple files or components
- Architectural decisions
- User-facing changes
- Integration with external systems

Do NOT activate for:
- Simple bug fixes with obvious solutions
- Single-line changes
- Documentation updates
- Dependency updates

## Workflow

### Phase 1: Clarify (3-5 questions max)

Ask questions ONE AT A TIME. Do not batch multiple questions. Wait for each answer before proceeding.

Suggested questions (adapt to context):
1. What problem does this solve? Who benefits?
2. What's the simplest version that would be useful?
3. Any constraints? (performance, compatibility, existing patterns to follow)
4. What should explicitly be OUT of scope?
5. **Modularity**: What existing modules or interfaces will this touch? Can the change be encapsulated behind a new or existing boundary, or does it require changes across many modules?

Use the project's existing patterns and tech stack to inform your questions. Reference specific files when relevant.

### Phase 1.5: Scope Check

Before drafting, assess whether the feature is small enough for a single spec:

- **More than 5 acceptance criteria?** Likely too big — suggest splitting into multiple specs.
- **Touches many unrelated modules?** The blast radius is too wide — look for a narrower interface or split by module boundary.
- **Cannot be described in 2-3 sentences of approach?** The feature may need decomposition.

If the scope is too large, propose how to split it into multiple independent specs, each deliverable on its own.

### Phase 2: Draft Spec

Write a brief spec to `/specs/{feature-name}.md` using the template in `/specs/TEMPLATE.md`.

**Front-matter**: Include YAML front-matter at the top with:
- `title`: Feature name (extracted from user discussion)
- `description`: One-line summary (extracted from user discussion)
- `status: proposed`
- `author`: Get from git config using `git config user.name` and `git config user.email` in format "Name <email>"

**Writing style**: Write in compressed, direct prose. No full sentences where a phrase will do. Omit articles, filler words, and transitional language. Each bullet or sentence should carry new information — no restating the goal, no summarizing what was already said. Aim for the minimum words that preserve meaning.

Keep content concise:
- Goal: 1-2 sentences
- Acceptance Criteria: 3-5 checkboxes
- Approach: 2-3 sentences
- Affected Modules: list which modules/files change and where the boundary is
- Test Strategy: how criteria will be verified
- Out of Scope: bullet list

### Phase 3: Confirm

Present the spec summary and ask: "Does this capture what you want? I'll implement once confirmed."

**Only proceed to implementation after explicit approval.**

If the user wants changes, revise the spec and confirm again.

## Reference

See `/specs/TEMPLATE.md` for the spec file format.
