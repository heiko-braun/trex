CREATE TABLE IF NOT EXISTS definitions (
    id          BIGSERIAL PRIMARY KEY,
    tenant      TEXT NOT NULL,
    name        TEXT NOT NULL,
    build_id    TEXT NOT NULL,
    yaml        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant, name, build_id)
);

CREATE INDEX idx_definitions_tenant_name_created_at
    ON definitions (tenant, name, created_at DESC);
