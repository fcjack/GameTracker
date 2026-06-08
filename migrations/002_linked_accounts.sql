CREATE TABLE IF NOT EXISTS linked_accounts (
    id                    BIGSERIAL     PRIMARY KEY,
    user_id               BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider              VARCHAR(20)   NOT NULL CHECK (provider IN ('steam', 'xbox')),
    external_id           VARCHAR(255)  NOT NULL,
    access_token_enc      TEXT,
    refresh_token_enc     TEXT,
    token_expires_at      TIMESTAMPTZ,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, provider)
);
