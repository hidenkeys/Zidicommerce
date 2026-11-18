CREATE TABLE IF NOT EXISTS password_recovery (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token_digest bytea NOT NULL UNIQUE,
    issued_at timestamptz NOT NULL DEFAULT now()
);
