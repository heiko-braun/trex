// Package store defines the domain-level interface for persisting workflow
// definitions. Concrete backends (e.g. store/postgres) implement it.
package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when no definition exists for a given
// tenant/name.
var ErrNotFound = errors.New("definition not found")

// Status is a definition's lifecycle state.
type Status string

const (
	StatusPending Status = "pending"
)

// Definition is a single published, validated workflow definition revision.
type Definition struct {
	Tenant    string
	Name      string
	BuildID   string
	YAML      string
	Status    Status
	CreatedAt time.Time
}

// DefinitionStore persists and retrieves workflow definitions.
type DefinitionStore interface {
	// Create writes a new definition. If a definition with the same
	// Tenant, Name, and BuildID already exists, Create returns it
	// unchanged instead of creating a duplicate (idempotent re-publish).
	Create(ctx context.Context, def *Definition) (*Definition, error)

	// GetCurrent returns the most recently created definition for the
	// given tenant/name, or ErrNotFound if none exists.
	GetCurrent(ctx context.Context, tenant, name string) (*Definition, error)

	// List returns every stored definition, newest first.
	List(ctx context.Context) ([]*Definition, error)
}

// AgentRegistration is a persisted record that an agent has been
// explicitly registered to run as a Temporal worker.
type AgentRegistration struct {
	AgentID      string
	Name         string
	TaskQueue    string
	RegisteredAt time.Time
}

// AgentRegistrationStore persists and retrieves agent registrations, so
// their Temporal workers can be restarted after a server restart without
// needing a fresh discovery call.
type AgentRegistrationStore interface {
	// Register writes a registration for agentID. Registering an
	// already-registered agent is idempotent: it returns the existing
	// row unchanged.
	Register(ctx context.Context, reg *AgentRegistration) (*AgentRegistration, error)

	// Unregister deletes the registration for agentID, if any.
	Unregister(ctx context.Context, agentID string) error

	// List returns every persisted registration.
	List(ctx context.Context) ([]*AgentRegistration, error)
}
