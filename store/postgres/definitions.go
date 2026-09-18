package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/heiko-braun/trex/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending migrations to the database at connStr.
func RunMigrations(connStr string) error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrations source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, connStr)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// DefinitionStore implements store.DefinitionStore using pgxpool.
type DefinitionStore struct {
	pool *pgxpool.Pool
}

var _ store.DefinitionStore = (*DefinitionStore)(nil)

// NewDefinitionStore creates a new Postgres-backed DefinitionStore.
func NewDefinitionStore(pool *pgxpool.Pool) *DefinitionStore {
	return &DefinitionStore{pool: pool}
}

func (s *DefinitionStore) Create(ctx context.Context, def *store.Definition) (*store.Definition, error) {
	const q = `
		INSERT INTO definitions (tenant, name, build_id, yaml, status)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant, name, build_id) DO NOTHING
		RETURNING tenant, name, build_id, yaml, status, created_at
	`
	row := s.pool.QueryRow(ctx, q, def.Tenant, def.Name, def.BuildID, def.YAML, def.Status)
	created, err := scanDefinition(row)
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("insert definition: %w", err)
	}

	// ON CONFLICT DO NOTHING means no row was returned: the exact
	// (tenant, name, build_id) already exists. Re-publishing identical
	// YAML is idempotent, so fetch and return the existing row instead
	// of treating this as an error.
	const existingQ = `
		SELECT tenant, name, build_id, yaml, status, created_at
		FROM definitions
		WHERE tenant = $1 AND name = $2 AND build_id = $3
	`
	existing, err := scanDefinition(s.pool.QueryRow(ctx, existingQ, def.Tenant, def.Name, def.BuildID))
	if err != nil {
		return nil, fmt.Errorf("fetch existing definition after conflict: %w", err)
	}
	return existing, nil
}

func (s *DefinitionStore) GetCurrent(ctx context.Context, tenant, name string) (*store.Definition, error) {
	const q = `
		SELECT tenant, name, build_id, yaml, status, created_at
		FROM definitions
		WHERE tenant = $1 AND name = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	def, err := scanDefinition(s.pool.QueryRow(ctx, q, tenant, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get current definition: %w", err)
	}
	return def, nil
}

func (s *DefinitionStore) List(ctx context.Context) ([]*store.Definition, error) {
	const q = `
		SELECT tenant, name, build_id, yaml, status, created_at
		FROM definitions
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list definitions: %w", err)
	}
	defer rows.Close()

	var defs []*store.Definition
	for rows.Next() {
		def, err := scanDefinition(rows)
		if err != nil {
			return nil, fmt.Errorf("scan definition: %w", err)
		}
		defs = append(defs, def)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list definitions: %w", err)
	}
	return defs, nil
}

func scanDefinition(row pgx.Row) (*store.Definition, error) {
	var def store.Definition
	if err := row.Scan(&def.Tenant, &def.Name, &def.BuildID, &def.YAML, &def.Status, &def.CreatedAt); err != nil {
		return nil, err
	}
	return &def, nil
}
