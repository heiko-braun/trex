package validator

import (
	"strings"
	"testing"
)

const validWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
`

const schemaInvalidWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - ping:
      call: http
      with: {}
`

const nonDeterministicWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
        headers:
          X-Request-Id: ${ uuid }
`

const topLevelShellWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - runIt:
      run:
        shell:
          command: /bin/sh
          arguments:
            - -c
            - echo hi
`

const topLevelScriptWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - runIt:
      run:
        script:
          language: python
          code: print("hi")
`

const nestedInForWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - loopIt:
      for:
        in: "${ [1,2,3] }"
      do:
        - runIt:
            run:
              shell:
                command: /bin/sh
                arguments:
                  - -c
                  - echo hi
`

const nestedInTryWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - tryIt:
      try:
        - runIt:
            run:
              shell:
                command: /bin/sh
                arguments:
                  - -c
                  - echo hi
      catch:
        do:
          - handleIt:
              set:
                x: 1
`

const nestedInCatchWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - tryIt:
      try:
        - noop:
            set:
              x: 1
      catch:
        do:
          - runIt:
              run:
                shell:
                  command: /bin/sh
                  arguments:
                    - -c
                    - echo hi
`

const nestedInForkWorkflow = `
document:
  dsl: 1.0.0
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - forkIt:
      fork:
        branches:
          - runIt:
              run:
                script:
                  language: python
                  code: print("hi")
`

func TestValidate_AcceptsValidWorkflow(t *testing.T) {
	if err := Validate([]byte(validWorkflow)); err != nil {
		t.Fatalf("expected valid workflow to pass, got: %v", err)
	}
}

func TestValidate_RejectsSchemaInvalid(t *testing.T) {
	err := Validate([]byte(schemaInvalidWorkflow))
	if err == nil {
		t.Fatal("expected schema validation error, got nil")
	}
	if !IsSchemaInvalid(err) {
		t.Errorf("expected IsSchemaInvalid, got: %v", err)
	}
}

func TestValidate_RejectsNonDeterministicExpression(t *testing.T) {
	err := Validate([]byte(nonDeterministicWorkflow))
	if err == nil {
		t.Fatal("expected non-determinism error, got nil")
	}
	if !IsNonDeterministic(err) {
		t.Errorf("expected IsNonDeterministic, got: %v", err)
	}
}

func TestValidate_RejectsTopLevelShell(t *testing.T) {
	assertPolicyViolation(t, topLevelShellWorkflow, "runIt", "shell")
}

func TestValidate_RejectsTopLevelScript(t *testing.T) {
	assertPolicyViolation(t, topLevelScriptWorkflow, "runIt", "script")
}

func TestValidate_RejectsShellNestedInFor(t *testing.T) {
	assertPolicyViolation(t, nestedInForWorkflow, "runIt", "shell")
}

func TestValidate_RejectsShellNestedInTry(t *testing.T) {
	assertPolicyViolation(t, nestedInTryWorkflow, "runIt", "shell")
}

func TestValidate_RejectsShellNestedInCatch(t *testing.T) {
	assertPolicyViolation(t, nestedInCatchWorkflow, "runIt", "shell")
}

func TestValidate_RejectsScriptNestedInFork(t *testing.T) {
	assertPolicyViolation(t, nestedInForkWorkflow, "runIt", "script")
}

func assertPolicyViolation(t *testing.T, yaml, wantKey, wantReasonSubstr string) {
	t.Helper()
	err := Validate([]byte(yaml))
	if err == nil {
		t.Fatal("expected policy violation, got nil")
	}
	pv, ok := AsPolicyViolation(err)
	if !ok {
		t.Fatalf("expected *PolicyViolation, got: %v", err)
	}
	if pv.TaskKey != wantKey {
		t.Errorf("TaskKey = %q, want %q", pv.TaskKey, wantKey)
	}
	if !strings.Contains(pv.Reason, wantReasonSubstr) {
		t.Errorf("Reason = %q, want substring %q", pv.Reason, wantReasonSubstr)
	}
}
