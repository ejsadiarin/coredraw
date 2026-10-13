-- +goose Up
CREATE TABLE IF NOT EXISTS boards (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    title TEXT NOT NULL,
    snapshot JSONB NOT NULL DEFAULT '{}',
    snapshot_version INTEGER NOT NULL DEFAULT 1,
    doc_md TEXT NOT NULL DEFAULT '',
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS boards_user_id_updated_at_idx ON boards (user_id, updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS boards;
