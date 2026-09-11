-- +goose Up
CREATE TABLE IF NOT EXISTS tt_links (
    node_id UUID NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    link TEXT NOT NULL,
    PRIMARY KEY (node_id, user_id)
);

-- +goose Down
DROP TABLE IF EXISTS tt_links;
