ALTER TABLE linked_accounts DROP CONSTRAINT linked_accounts_provider_check;
ALTER TABLE linked_accounts
    ADD CONSTRAINT linked_accounts_provider_check
        CHECK (provider IN ('steam', 'xbox', 'epic'));

ALTER TABLE import_jobs DROP CONSTRAINT import_jobs_provider_check;
ALTER TABLE import_jobs
    ADD CONSTRAINT import_jobs_provider_check
        CHECK (provider IN ('steam', 'xbox', 'epic'));

ALTER TABLE games
    ADD COLUMN IF NOT EXISTS epic_catalog_item_id VARCHAR(255) UNIQUE,
    ADD COLUMN IF NOT EXISTS epic_namespace VARCHAR(50) NOT NULL DEFAULT 'egs';
