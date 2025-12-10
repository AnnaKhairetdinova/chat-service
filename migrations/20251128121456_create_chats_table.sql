-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS chats (
                                     uuid         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type         TEXT NOT NULL CHECK (type IN ('direct', 'group')),
    name         TEXT,
    creator_uuid UUID REFERENCES users(uuid) ON DELETE SET NULL,
    participants TEXT NOT NULL DEFAULT '[]',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );

CREATE INDEX IF NOT EXISTS idx_chats_participants ON chats USING GIN ((participants::jsonb));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS chats;
-- +goose StatementEnd
