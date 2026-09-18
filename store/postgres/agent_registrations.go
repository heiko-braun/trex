package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/heiko-braun/trex/store"
)

// AgentRegistrationStore implements store.AgentRegistrationStore using
// pgxpool.
type AgentRegistrationStore struct {
	pool *pgxpool.Pool
}

var _ store.AgentRegistrationStore = (*AgentRegistrationStore)(nil)

// NewAgentRegistrationStore creates a new Postgres-backed
// AgentRegistrationStore.
func NewAgentRegistrationStore(pool *pgxpool.Pool) *AgentRegistrationStore {
	return &AgentRegistrationStore{pool: pool}
}

func (s *AgentRegistrationStore) Register(ctx context.Context, reg *store.AgentRegistration) (*store.AgentRegistration, error) {
	const q = `
		INSERT INTO agent_registrations (agent_id, name, task_queue)
		VALUES ($1, $2, $3)
		ON CONFLICT (agent_id) DO NOTHING
		RETURNING agent_id, name, task_queue, registered_at
	`
	row := s.pool.QueryRow(ctx, q, reg.AgentID, reg.Name, reg.TaskQueue)
	created, err := scanAgentRegistration(row)
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("insert agent registration: %w", err)
	}

	// ON CONFLICT DO NOTHING means no row was returned: this agent is
	// already registered. Registering it again is idempotent, so return
	// the existing row instead of treating this as an error.
	const existingQ = `
		SELECT agent_id, name, task_queue, registered_at
		FROM agent_registrations
		WHERE agent_id = $1
	`
	existing, err := scanAgentRegistration(s.pool.QueryRow(ctx, existingQ, reg.AgentID))
	if err != nil {
		return nil, fmt.Errorf("fetch existing agent registration after conflict: %w", err)
	}
	return existing, nil
}

func (s *AgentRegistrationStore) Unregister(ctx context.Context, agentID string) error {
	const q = `DELETE FROM agent_registrations WHERE agent_id = $1`
	if _, err := s.pool.Exec(ctx, q, agentID); err != nil {
		return fmt.Errorf("delete agent registration: %w", err)
	}
	return nil
}

func (s *AgentRegistrationStore) List(ctx context.Context) ([]*store.AgentRegistration, error) {
	const q = `
		SELECT agent_id, name, task_queue, registered_at
		FROM agent_registrations
		ORDER BY registered_at ASC
	`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list agent registrations: %w", err)
	}
	defer rows.Close()

	var regs []*store.AgentRegistration
	for rows.Next() {
		reg, err := scanAgentRegistration(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent registration: %w", err)
		}
		regs = append(regs, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list agent registrations: %w", err)
	}
	return regs, nil
}

func scanAgentRegistration(row pgx.Row) (*store.AgentRegistration, error) {
	var reg store.AgentRegistration
	if err := row.Scan(&reg.AgentID, &reg.Name, &reg.TaskQueue, &reg.RegisteredAt); err != nil {
		return nil, err
	}
	return &reg, nil
}
