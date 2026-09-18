---
name: verify
description: Verify that implemented code changes match their specification. Use after a spec has been implemented.
allowed-tools: Read, Bash, Task, TodoWrite, Edit
---

# Spec Verification Command

You coordinate the verification of implemented code changes against their specification by delegating the work to a specialized subagent.

## When to Activate

- When user says `/verify` or "verify the implementation"
- After implementation is complete and user wants to verify spec compliance
- When user wants to check if code matches a specific spec

## Workflow

### 1. Identify Spec to Review

**Check for user input:**
- If the user provided arguments (e.g., `/verify feature-name`), use that spec file
- If no arguments, look for recently implemented specs:
  - List all files in `specs/` directory
  - Check their `status` field in the front-matter
  - Look for specs with `status: implemented` or recent modifications

**Ask user to confirm:**
- Present the identified spec and ask: "I will review the implementation of '{spec-name}'. Is this correct?"
- If user says no, ask: "Which spec would you like me to review?"

### 2. Invoke Verification Subagent

**Launch the subagent:**

When executing this skill, invoke a subagent via the Anthropic API with the following setup:

- System prompt: .claude/agents/verify-agent.md
- Pass the spec file path as the user message
- Parse the subagent's response and use it to complete the task

### 3. Handle Fixable Issues

**After receiving the verification report:**

- Display the full report to the user
- If the report includes a "Fixable Issues" section with one or more issues:
  - Ask the user: "Would you like me to fix these issues?"
  - If user says yes or confirms:
    - Work through each fixable issue one by one
    - Use appropriate tools (Edit, Write, Bash) to implement the fixes
    - After all fixes are applied, re-run the test suite
    - Report the results to the user
  - If user says no or declines:
    - End the verification process
- If no fixable issues are found:
  - End the verification process after displaying the report
