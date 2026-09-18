// Package agents talks to the managed agents control plane: discovering
// agents (stateless, using whichever token the caller supplies) and
// dispatching A2A calls to them. See specs/agent-discovery-and-registration.md.
package agents

// Agent is a discovered managed-agents platform agent, trimmed to the
// fields the workflow server needs to register it as a Temporal activity.
type Agent struct {
	ID      string // platform agent ID, e.g. "decorous-word-8592"
	Name    string // human-assigned name, e.g. "coding-generalist"
	Type    string
	Enabled bool
}

// Slug is the activity/task-queue-safe identifier for an agent: its
// platform ID, which is already a DNS/queue-safe slug.
func (a Agent) Slug() string {
	return a.ID
}

// TaskQueue is the Temporal task queue an agent's activity worker polls.
func (a Agent) TaskQueue() string {
	return "agent-" + a.Slug()
}
