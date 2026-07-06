ALTER TABLE games DROP COLUMN IF EXISTS epic_namespace;
ALTER TABLE games DROP COLUMN IF EXISTS epic_catalog_item_id;

ALTER TABLE import_jobs DROP CONSTRAINT import_jobs_provider_check;
ALTER TABLE import_jobs
    ADD CONSTRAINT import_jobs_provider_check
        CHECK (provider IN ('steam', 'xbox'));

ALTER TABLE linked_accounts DROP CONSTRAINT linked_accounts_provider_check;
ALTER TABLE linked_accounts
    ADD CONSTRAINT linked_accounts_provider_check
        CHECK (provider IN ('steam', 'xbox'));
