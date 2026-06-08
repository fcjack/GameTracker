DROP INDEX IF EXISTS idx_games_steam_app_id;

DO $$ BEGIN
    ALTER TABLE games ADD CONSTRAINT games_steam_app_id_key UNIQUE (steam_app_id);
EXCEPTION
    WHEN duplicate_object THEN NULL;
    WHEN duplicate_table THEN NULL;
END $$;
