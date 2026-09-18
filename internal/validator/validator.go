// Package validator checks a workflow definition against Zigflow's own
// schema/determinism rules and this server's policy restrictions before it
// is ever persisted.
package validator

import (
	"errors"
	"fmt"

	owsmodel "github.com/open-workflow-specification/sdk-go/v4/model"
	"github.com/zigflow/zigflow/pkg/zigflow"
)

// PolicyViolation reports a disallowed construct found in a workflow
// definition, identified by the task key it was found under.
type PolicyViolation struct {
	TaskKey string
	Reason  string
}

func (v *PolicyViolation) Error() string {
	return fmt.Sprintf("policy violation in task %q: %s", v.TaskKey, v.Reason)
}

// Validate runs a workflow definition through Zigflow's schema and
// determinism checks, then this server's policy checks (no run:shell,
// no run:script). It returns the first error found; callers that need to
// distinguish error classes can unwrap with errors.Is against
// zigflow.ErrSchemaValidation, zigflow.ErrNonDeterministicExpression, or
// check for *PolicyViolation.
func Validate(yamlBytes []byte) error {
	if err := zigflow.ValidateBytes(yamlBytes); err != nil {
		return err
	}

	wf, err := zigflow.LoadFromBytes(yamlBytes)
	if err != nil {
		return err
	}

	return checkPolicy(wf.Do)
}

// checkPolicy walks every task in the workflow's task tree (recursing into
// do/for/try/fork containers) and rejects any run:shell or run:script task.
func checkPolicy(tasks *owsmodel.TaskList) error {
	if tasks == nil {
		return nil
	}
	for _, item := range *tasks {
		if item == nil || item.Task == nil {
			continue
		}
		if err := checkTask(item.Key, item.Task); err != nil {
			return err
		}
	}
	return nil
}

func checkTask(key string, task owsmodel.Task) error {
	switch t := task.(type) {
	case *owsmodel.RunTask:
		if t.Run.Shell != nil {
			return &PolicyViolation{TaskKey: key, Reason: "run:shell is not allowed"}
		}
		if t.Run.Script != nil {
			return &PolicyViolation{TaskKey: key, Reason: "run:script is not allowed"}
		}
		return nil
	case *owsmodel.DoTask:
		return checkPolicy(t.Do)
	case *owsmodel.ForTask:
		return checkPolicy(t.Do)
	case *owsmodel.ForkTask:
		return checkPolicy(t.Fork.Branches)
	case *owsmodel.TryTask:
		if err := checkPolicy(t.Try); err != nil {
			return err
		}
		if t.Catch != nil {
			return checkPolicy(t.Catch.Do)
		}
		return nil
	default:
		// call/set/switch/wait/raise/event tasks carry no nested task list
		// that could hide a run:shell/run:script, so there's nothing to walk.
		return nil
	}
}

// IsNonDeterministic reports whether err (or a wrapped cause) is Zigflow's
// non-deterministic-expression error.
func IsNonDeterministic(err error) bool {
	return errors.Is(err, zigflow.ErrNonDeterministicExpression)
}

// IsSchemaInvalid reports whether err (or a wrapped cause) is a Zigflow
// schema validation error.
func IsSchemaInvalid(err error) bool {
	return errors.Is(err, zigflow.ErrSchemaValidation)
}

// AsPolicyViolation reports whether err is a policy violation raised by
// this package's own checks (as opposed to a Zigflow-level error).
func AsPolicyViolation(err error) (*PolicyViolation, bool) {
	var pv *PolicyViolation
	if errors.As(err, &pv) {
		return pv, true
	}
	return nil, false
}
