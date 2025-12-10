-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
                       uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       name TEXT NOT NULL,
                       surname TEXT NOT NULL,
                       email VARCHAR(255) UNIQUE NOT NULL, -- Unique email address
                       password_hash VARCHAR(255) NOT NULL, -- Bcrypt hashed password
                       created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
                       updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users CASCADE;
-- +goose StatementEnd
