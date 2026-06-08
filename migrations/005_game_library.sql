CREATE TABLE IF NOT EXISTS categories (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    igdb_value  INT          UNIQUE
);

CREATE TABLE IF NOT EXISTS games (
    id           BIGSERIAL    PRIMARY KEY,
    igdb_id      BIGINT       UNIQUE,
    category_id  BIGINT       NOT NULL REFERENCES categories(id),
    name         VARCHAR(255) NOT NULL,
    cover_url    TEXT,
    platforms    TEXT[]       NOT NULL DEFAULT '{}',
    release_year INT,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_games (
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id    BIGINT      NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    status     VARCHAR(20) NOT NULL DEFAULT 'owned'
                   CHECK (status IN ('owned', 'playing', 'completed', 'dropped')),
    tags       TEXT[]      NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, game_id)
);

INSERT INTO categories (name, igdb_value) VALUES
    ('Main Game', 0),
    ('DLC / Add-on', 1),
    ('Expansion', 2),
    ('Bundle', 3),
    ('Standalone Expansion', 4),
    ('Mod', 5),
    ('Episode', 6),
    ('Season', 7),
    ('Remake', 8),
    ('Remaster', 9),
    ('Expanded Game', 10),
    ('Port', 11),
    ('Fork', 12),
    ('Pack', 13),
    ('Update', 14)
ON CONFLICT (igdb_value) DO NOTHING;
