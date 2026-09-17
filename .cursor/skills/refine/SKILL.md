---
name: refine
description: Refines existing specs
---

# Refine Existing Spec

Refine an existing specification based on new insights, feedback, or changing requirements.

**Feature to refine:** $ARGUMENTS

## Instructions

1. **Load the existing spec** from `/specs/{feature}.md`
   - If no spec exists for this feature, suggest using `/spec` instead
   - If multiple specs match, ask the user which one to refine

2. **Ask 2-3 focused refinement questions** (one at a time):
   - What aspect needs refinement? (goals, criteria, approach, scope)
   - What new information or feedback has emerged?
   - Are there specific pain points with the current spec?

3. **Check scope and modularity:**
   - Does the refinement keep the spec small enough for a single vertical slice?
   - If criteria are being added, does the total still stay at ~5 or fewer?
   - Does the refinement change which modules are affected? Update the "Affected Modules" section.
   - If the refinement significantly expands scope or blast radius, suggest a separate spec instead.

4. **Update the spec in place**:
   - **Preserve front-matter**: Keep all existing front-matter fields (title, description, author). Keep `status: proposed` (refinements don't change status)
   - Preserve completed acceptance criteria checkboxes
   - Update goals, criteria, or approach as needed
   - Update "Affected Modules" and "Test Strategy" if the changes alter them
   - Add to "Out of Scope" if removing features
   - Add refinement notes to the "Notes" section with timestamp

5. **Show a diff summary**:
   - Highlight what changed (goals, new criteria, removed items, affected modules, etc.)
   - Ask for confirmation before saving

6. **Get user confirmation** before proceeding to implementation
   - If confirmed, use the **implement** skill with the refined spec
   - If not, ask if they want to refine further

## Refinement Guidelines

- **Preserve progress**: Don't uncheck completed criteria unless they're no longer valid
- **Be additive when possible**: Add new criteria rather than rewriting existing ones
- **Document changes**: Always add a timestamped note explaining what was refined and why
- **Validate scope**: Check if refinements are expanding scope significantly - if so, suggest a new spec
- **Validate modularity**: If the refinement introduces new module dependencies or widens the blast radius, flag it explicitly

## Example Refinement Note

```markdown
## Notes

**Refinement 2026-01-25**: Updated approach to use WebSocket instead of polling based on performance testing results. Added new acceptance criterion for connection handling. Blast radius unchanged — change is contained within the `transport` module.
```

## Reminders

- Keep refinements focused and minimal
- Preserve the spec's history through notes
- Suggest new specs for major scope changes
