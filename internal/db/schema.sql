CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS check_against (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT '',
    normalized_title TEXT NOT NULL UNIQUE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_library (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT '',
    normalized_title TEXT NOT NULL UNIQUE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS matches (
    id SERIAL PRIMARY KEY,
    library_book_id INTEGER NOT NULL REFERENCES user_library(id) ON DELETE CASCADE,
    check_book_id INTEGER NOT NULL REFERENCES check_against(id) ON DELETE CASCADE,
    matched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (library_book_id, check_book_id)
);

CREATE INDEX IF NOT EXISTS idx_check_against_title_trgm ON check_against USING gin (normalized_title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_user_library_title_trgm ON user_library USING gin (normalized_title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_matches_library ON matches(library_book_id);
CREATE INDEX IF NOT EXISTS idx_matches_check ON matches(check_book_id);
