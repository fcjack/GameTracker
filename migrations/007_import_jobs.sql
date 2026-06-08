CREATE TABLE IF NOT EXISTS import_jobs (
    id               BIGSERIAL    PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         VARCHAR(20)  NOT NULL CHECK (provider IN ('steam', 'xbox')),
    status           VARCHAR(20)  NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    total_count      INT          NOT NULL DEFAULT 0,
    processed_count  INT          NOT NULL DEFAULT 0,
    imported_count   INT          NOT NULL DEFAULT 0,
    skipped_count    INT          NOT NULL DEFAULT 0,
    error_message    TEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_import_jobs_user_provider
    ON import_jobs (user_id, provider, created_at DESC);
