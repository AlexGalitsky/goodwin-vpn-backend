-- +goose Up
CREATE TABLE IF NOT EXISTS traffic_cursor (
    node_id UUID NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    uplink BIGINT NOT NULL DEFAULT 0,
    downlink BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (node_id, user_id)
);

-- +goose Down
DROP TABLE IF EXISTS traffic_cursor;
