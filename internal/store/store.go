package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var sqlFS embed.FS

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (filename TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := sqlFS.ReadDir("sql")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		var exists int
		err := s.pool.QueryRow(ctx, `SELECT 1 FROM schema_migrations WHERE filename=$1`, e.Name()).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		raw, err := sqlFS.ReadFile("sql/" + e.Name())
		if err != nil {
			return err
		}
		sql := string(raw)
		if i := strings.Index(sql, "-- +goose Down"); i >= 0 {
			sql = sql[:i]
		}
		if _, err := s.pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migrate %s: %w", e.Name(), err)
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO schema_migrations (filename) VALUES ($1)`, e.Name()); err != nil {
			return err
		}
	}
	return nil
}

type Group struct {
	ID        uuid.UUID
	Name      string
	Protocols []string
}

type User struct {
	ID          uuid.UUID
	GroupID     uuid.UUID
	DisplayName string
	VlessUUID   string
	Hy2Password string
	TTUser      string
	TTPassword  string
	Upload      int64
	Download    int64
	Total       int64
	Status      string
	SubToken    string
}

type Node struct {
	ID          uuid.UUID
	Name        string
	IPv4        string
	IPv6        string
	Hostname    string
	ControlPort int
	Families    []string
	PortsJSON   json.RawMessage `json:"ports"`
	Status      string
	AgentToken  string `json:"-"`
}

func (s *Store) CreateGroup(ctx context.Context, name string, protocols []string) (Group, error) {
	g := Group{ID: uuid.New(), Name: name, Protocols: protocols}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO groups (id, name, protocols) VALUES ($1,$2,$3)`,
		g.ID, g.Name, g.Protocols,
	)
	return g, err
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, protocols FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Protocols); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, u User) (User, error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (
			id, group_id, display_name, vless_uuid, hy2_password, tt_user, tt_password,
			upload, download, total, status, sub_token
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		u.ID, u.GroupID, u.DisplayName, u.VlessUUID, u.Hy2Password, u.TTUser, u.TTPassword,
		u.Upload, u.Download, u.Total, u.Status, u.SubToken,
	)
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, group_id, display_name, vless_uuid, hy2_password, tt_user, tt_password,
			upload, download, total, status, sub_token
		FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.GroupID, &u.DisplayName, &u.VlessUUID, &u.Hy2Password, &u.TTUser, &u.TTPassword,
			&u.Upload, &u.Download, &u.Total, &u.Status, &u.SubToken); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UserBySubToken(ctx context.Context, token string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, group_id, display_name, vless_uuid, hy2_password, tt_user, tt_password,
			upload, download, total, status, sub_token
		FROM users WHERE sub_token=$1`, token).Scan(
		&u.ID, &u.GroupID, &u.DisplayName, &u.VlessUUID, &u.Hy2Password, &u.TTUser, &u.TTPassword,
		&u.Upload, &u.Download, &u.Total, &u.Status, &u.SubToken,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateNode(ctx context.Context, n Node) (Node, error) {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	if n.ControlPort == 0 {
		n.ControlPort = 19400
	}
	if n.Status == "" {
		n.Status = "pending"
	}
	if len(n.PortsJSON) == 0 {
		n.PortsJSON = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO nodes (id, name, ipv4, ipv6, hostname, control_port, families, ports, status, agent_token)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		n.ID, n.Name, n.IPv4, n.IPv6, n.Hostname, n.ControlPort, n.Families, n.PortsJSON, n.Status, n.AgentToken,
	)
	return n, err
}

func (s *Store) UpdateNode(ctx context.Context, n Node) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE nodes SET name=$2, ipv4=$3, ipv6=$4, hostname=$5, control_port=$6,
			families=$7, ports=$8, status=$9, agent_token=$10
		WHERE id=$1`,
		n.ID, n.Name, n.IPv4, n.IPv6, n.Hostname, n.ControlPort, n.Families, n.PortsJSON, n.Status, n.AgentToken,
	)
	return err
}

func (s *Store) Node(ctx context.Context, id uuid.UUID) (Node, error) {
	var n Node
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, ipv4, ipv6, hostname, control_port, families, ports, status, agent_token
		FROM nodes WHERE id=$1`, id).Scan(
		&n.ID, &n.Name, &n.IPv4, &n.IPv6, &n.Hostname, &n.ControlPort, &n.Families, &n.PortsJSON, &n.Status, &n.AgentToken,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	return n, err
}

func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, ipv4, ipv6, hostname, control_port, families, ports, status, agent_token
		FROM nodes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Name, &n.IPv4, &n.IPv6, &n.Hostname, &n.ControlPort, &n.Families, &n.PortsJSON, &n.Status, &n.AgentToken); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) SetNodeGroups(ctx context.Context, nodeID uuid.UUID, groupIDs []uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM node_groups WHERE node_id=$1`, nodeID); err != nil {
		return err
	}
	for _, gid := range groupIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO node_groups (node_id, group_id) VALUES ($1,$2)`, nodeID, gid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) NodeIDsForGroup(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT node_id FROM node_groups WHERE group_id=$1`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) GroupIDsForNode(ctx context.Context, nodeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT group_id FROM node_groups WHERE node_id=$1`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) Audit(ctx context.Context, actor, action string, nodeID *uuid.UUID, detail string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_events (id, actor, action, node_id, detail) VALUES ($1,$2,$3,$4,$5)`,
		uuid.New(), actor, action, nodeID, detail,
	)
	return err
}

func (s *Store) SeedDev(ctx context.Context) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM groups`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	g, err := s.CreateGroup(ctx, "dev", []string{"vless", "hy2", "tt"})
	if err != nil {
		return err
	}
	_, err = s.CreateUser(ctx, User{
		GroupID:     g.ID,
		DisplayName: "dev",
		VlessUUID:   "11111111-2222-3333-4444-555555555555",
		Hy2Password: "dev-hy2",
		TTUser:      "dev",
		TTPassword:  "dev-tt",
		Status:      "active",
		SubToken:    "dev-sub-token",
	})
	return err
}
