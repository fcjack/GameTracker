ALTER TABLE user_games
    ADD COLUMN completed_at TIMESTAMPTZ,
    ADD COLUMN dropped_at   TIMESTAMPTZ;
