---
name: verify
description: Verify that implemented code changes match their specification. Use after a spec has been implemented.
---

# Spec Verification Skill

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

Invoke the verify-agent subagent (located at `.cursor/agents/verify-agent.md`) to perform the verification:

- Use Cursor's subagent delegation to invoke `/verify-agent`
- Pass the spec file path as context
- The subagent will analyze code changes, verify compliance, run tests, and generate a detailed report
- Wait for the subagent to complete and return its verification report

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
