-- +goose Up
-- +goose StatementBegin
CREATE TABLE messages
(
    uuid        UUID PRIMARY KEY         DEFAULT gen_random_uuid(),
    chat_uuid   TEXT NOT NULL DEFAULT 'global',
    sender_uuid UUID NOT NULL,
    sender_name TEXT NOT NULL DEFAULT 'аноним',
    content     TEXT NOT NULL,
    is_read     BOOLEAN                  DEFAULT false,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_uuid ON messages(chat_uuid, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender_uuid ON messages(sender_uuid);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS chats CASCADE;
-- +goose StatementEnd
