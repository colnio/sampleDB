-- Migration 002: Add api_tokens table
CREATE TABLE IF NOT EXISTS api_tokens (
    token_id     SERIAL PRIMARY KEY,
    user_id      INT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    token_hash   VARCHAR(64) NOT NULL UNIQUE,
    name         VARCHAR(100),
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    revoked      BOOLEAN NOT NULL DEFAULT false
);
