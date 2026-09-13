-- +goose Up
ALTER TABLE traffic_cursor ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'xray';
ALTER TABLE traffic_cursor DROP CONSTRAINT IF EXISTS traffic_cursor_pkey;
ALTER TABLE traffic_cursor ADD PRIMARY KEY (node_id, user_id, source);

-- +goose Down
ALTER TABLE traffic_cursor DROP CONSTRAINT IF EXISTS traffic_cursor_pkey;
ALTER TABLE traffic_cursor DROP COLUMN IF EXISTS source;
ALTER TABLE traffic_cursor ADD PRIMARY KEY (node_id, user_id);
