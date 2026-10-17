-- +goose Up
ALTER TABLE groups ADD COLUMN IF NOT EXISTS device_limit INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE groups DROP COLUMN IF EXISTS device_limit;
