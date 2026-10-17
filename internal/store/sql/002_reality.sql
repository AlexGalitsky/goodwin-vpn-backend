-- +goose Up
CREATE TABLE IF NOT EXISTS plane_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

ALTER TABLE nodes ADD COLUMN IF NOT EXISTS applied_families TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE nodes DROP COLUMN IF EXISTS applied_families;
DROP TABLE IF EXISTS plane_settings;
