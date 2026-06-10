ALTER TABLE user_games
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS dropped_at;
