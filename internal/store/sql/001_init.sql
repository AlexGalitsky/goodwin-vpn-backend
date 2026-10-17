-- +goose Up
CREATE TABLE groups (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    protocols TEXT[] NOT NULL DEFAULT ARRAY['vless', 'hy2', 'tt']::TEXT[],
    quota_bytes BIGINT NOT NULL DEFAULT 0,
    expire_default_hours INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id UUID PRIMARY KEY,
    group_id UUID NOT NULL REFERENCES groups (id) ON DELETE RESTRICT,
    display_name TEXT NOT NULL DEFAULT '',
    vless_uuid TEXT NOT NULL,
    hy2_password TEXT NOT NULL DEFAULT '',
    tt_user TEXT NOT NULL DEFAULT '',
    tt_password TEXT NOT NULL DEFAULT '',
    upload BIGINT NOT NULL DEFAULT 0,
    download BIGINT NOT NULL DEFAULT 0,
    total BIGINT NOT NULL DEFAULT 0,
    expire TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'active',
    sub_token TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE nodes (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    ipv4 TEXT NOT NULL DEFAULT '',
    ipv6 TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL DEFAULT '',
    control_port INT NOT NULL DEFAULT 19400,
    families TEXT[] NOT NULL DEFAULT '{}',
    ports JSONB NOT NULL DEFAULT '{}'::JSONB,
    status TEXT NOT NULL DEFAULT 'pending',
    agent_token TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE node_groups (
    node_id UUID NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    group_id UUID NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    PRIMARY KEY (node_id, group_id)
);

CREATE TABLE audit_events (
    id UUID PRIMARY KEY,
    at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    node_id UUID,
    detail TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS node_groups;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS groups;
