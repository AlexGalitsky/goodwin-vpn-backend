package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var sqlFS embed.FS

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

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
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
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
	ID                 uuid.UUID
	Name               string
	Protocols          []string
	QuotaBytes         int64
	ExpireDefaultHours int
	QuotaReset         string
}

type User struct {
	ID               uuid.UUID
	GroupID          uuid.UUID
	DisplayName      string
	VlessUUID        string `json:"-"`
	Hy2Password      string `json:"-"`
	TTUser           string `json:"-"`
	TTPassword       string `json:"-"`
	Upload           int64
	Download         int64
	Total            int64
	Expire           *time.Time
	Status           string
	SubToken         string `json:"-"`
	Note             string
	QuotaReset       string
	QuotaPeriodStart *time.Time
}

type AuditEvent struct {
	ID     uuid.UUID
	At     time.Time
	Actor  string
	Action string
	NodeID *uuid.UUID
	Detail string
}

const userCols = `id, group_id, display_name, vless_uuid, hy2_password, tt_user, tt_password,
			upload, download, total, expire, status, sub_token, note, quota_reset, quota_period_start`

func scanUser(sc interface{ Scan(dest ...any) error }) (User, error) {
	var u User
	err := sc.Scan(&u.ID, &u.GroupID, &u.DisplayName, &u.VlessUUID, &u.Hy2Password, &u.TTUser, &u.TTPassword,
		&u.Upload, &u.Download, &u.Total, &u.Expire, &u.Status, &u.SubToken, &u.Note, &u.QuotaReset, &u.QuotaPeriodStart)
	return u, err
}

type Node struct {
	ID              uuid.UUID
	Name            string
	IPv4            string
	IPv6            string
	Hostname        string
	ControlPort     int
	Families        []string
	AppliedFamilies []string        `json:"applied_families"`
	PortsJSON       json.RawMessage `json:"ports"`
	Status          string
	AgentToken      string `json:"-"`
}

func (s *Store) CreateGroup(ctx context.Context, name string, protocols []string, quotaBytes int64, expireHours int) (Group, error) {
	return s.CreateGroupReset(ctx, name, protocols, quotaBytes, expireHours, "")
}

func (s *Store) CreateGroupReset(ctx context.Context, name string, protocols []string, quotaBytes int64, expireHours int, quotaReset string) (Group, error) {
	g := Group{ID: uuid.New(), Name: name, Protocols: protocols, QuotaBytes: quotaBytes, ExpireDefaultHours: expireHours, QuotaReset: quotaReset}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO groups (id, name, protocols, quota_bytes, expire_default_hours, quota_reset) VALUES ($1,$2,$3,$4,$5,$6)`,
		g.ID, g.Name, g.Protocols, g.QuotaBytes, g.ExpireDefaultHours, g.QuotaReset,
	)
	return g, err
}

func (s *Store) UpdateGroup(ctx context.Context, g Group) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE groups SET name=$2, protocols=$3, quota_bytes=$4, expire_default_hours=$5, quota_reset=$6 WHERE id=$1`,
		g.ID, g.Name, g.Protocols, g.QuotaBytes, g.ExpireDefaultHours, g.QuotaReset,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, protocols, quota_bytes, expire_default_hours, quota_reset FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Protocols, &g.QuotaBytes, &g.ExpireDefaultHours, &g.QuotaReset); err != nil {
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
			upload, download, total, expire, status, sub_token, note, quota_reset, quota_period_start
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		u.ID, u.GroupID, u.DisplayName, u.VlessUUID, u.Hy2Password, u.TTUser, u.TTPassword,
		u.Upload, u.Download, u.Total, u.Expire, u.Status, u.SubToken, u.Note, u.QuotaReset, u.QuotaPeriodStart,
	)
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) User(ctx context.Context, id uuid.UUID) (User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) UserBySubToken(ctx context.Context, token string) (User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE sub_token=$1`, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) SetUserStatus(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET status=$2 WHERE id=$1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateUser(ctx context.Context, u User) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users SET display_name=$2, note=$3, status=$4, upload=$5, download=$6, total=$7, expire=$8,
			quota_reset=$9, quota_period_start=$10
		WHERE id=$1`,
		u.ID, u.DisplayName, u.Note, u.Status, u.Upload, u.Download, u.Total, u.Expire,
		u.QuotaReset, u.QuotaPeriodStart,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RotateSubToken(ctx context.Context, id uuid.UUID, newToken string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET sub_token=$2 WHERE id=$1`, id, newToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeUser(ctx context.Context, id uuid.UUID, newToken string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET status='revoked', sub_token=$2 WHERE id=$1`, id, newToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM tt_links WHERE user_id=$1`, id); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountUsersInGroup(ctx context.Context, groupID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE group_id=$1`, groupID).Scan(&n)
	return n, err
}

func (s *Store) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	n, err := s.CountUsersInGroup(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteNode(ctx context.Context, id uuid.UUID) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM tt_links WHERE node_id=$1`, id); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM nodes WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) LastAudit(ctx context.Context, action string) (AuditEvent, error) {
	var e AuditEvent
	err := s.pool.QueryRow(ctx, `
		SELECT id, at, actor, action, node_id, detail FROM audit_events
		WHERE action=$1 ORDER BY at DESC LIMIT 1`, action).Scan(
		&e.ID, &e.At, &e.Actor, &e.Action, &e.NodeID, &e.Detail,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuditEvent{}, ErrNotFound
	}
	return e, err
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
	if n.Families == nil {
		n.Families = []string{}
	}
	if n.AppliedFamilies == nil {
		n.AppliedFamilies = []string{}
	}
	if len(n.PortsJSON) == 0 {
		n.PortsJSON = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO nodes (id, name, ipv4, ipv6, hostname, control_port, families, applied_families, ports, status, agent_token)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		n.ID, n.Name, n.IPv4, n.IPv6, n.Hostname, n.ControlPort, n.Families, n.AppliedFamilies, n.PortsJSON, n.Status, n.AgentToken,
	)
	return n, err
}

func (s *Store) UpdateNode(ctx context.Context, n Node) error {
	if n.Families == nil {
		n.Families = []string{}
	}
	if n.AppliedFamilies == nil {
		n.AppliedFamilies = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE nodes SET name=$2, ipv4=$3, ipv6=$4, hostname=$5, control_port=$6,
			families=$7, applied_families=$8, ports=$9, status=$10, agent_token=$11
		WHERE id=$1`,
		n.ID, n.Name, n.IPv4, n.IPv6, n.Hostname, n.ControlPort, n.Families, n.AppliedFamilies, n.PortsJSON, n.Status, n.AgentToken,
	)
	return err
}

func (s *Store) UpdateNodeStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET status=$2 WHERE id=$1 AND status<>$2`, id, status)
	return err
}

func (s *Store) Node(ctx context.Context, id uuid.UUID) (Node, error) {
	var n Node
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, ipv4, ipv6, hostname, control_port, families, applied_families, ports, status, agent_token
		FROM nodes WHERE id=$1`, id).Scan(
		&n.ID, &n.Name, &n.IPv4, &n.IPv6, &n.Hostname, &n.ControlPort, &n.Families, &n.AppliedFamilies, &n.PortsJSON, &n.Status, &n.AgentToken,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	return n, err
}

func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, ipv4, ipv6, hostname, control_port, families, applied_families, ports, status, agent_token
		FROM nodes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Name, &n.IPv4, &n.IPv6, &n.Hostname, &n.ControlPort, &n.Families, &n.AppliedFamilies, &n.PortsJSON, &n.Status, &n.AgentToken); err != nil {
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

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, at, actor, action, node_id, detail FROM audit_events
		ORDER BY at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.At, &e.Actor, &e.Action, &e.NodeID, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.pool.QueryRow(ctx, `SELECT value FROM plane_settings WHERE key=$1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO plane_settings (key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`, key, value)
	return err
}

func (s *Store) UsersInGroup(ctx context.Context, groupID uuid.UUID) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users WHERE group_id=$1 AND status='active'`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) Group(ctx context.Context, id uuid.UUID) (Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx, `SELECT id, name, protocols, quota_bytes, expire_default_hours, quota_reset FROM groups WHERE id=$1`, id).Scan(
		&g.ID, &g.Name, &g.Protocols, &g.QuotaBytes, &g.ExpireDefaultHours, &g.QuotaReset,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	return g, err
}

func (s *Store) SeedDev(ctx context.Context) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM groups`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	g, err := s.CreateGroup(ctx, "dev", []string{"vless", "hy2", "tt"}, 0, 0)
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

func (s *Store) ReplaceTTLinks(ctx context.Context, nodeID uuid.UUID, links map[uuid.UUID]string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tt_links WHERE node_id=$1`, nodeID); err != nil {
		return err
	}
	for uid, link := range links {
		if strings.TrimSpace(link) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tt_links (node_id, user_id, link) VALUES ($1,$2,$3)`, nodeID, uid, link); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteTTLinksForNode(ctx context.Context, nodeID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tt_links WHERE node_id=$1`, nodeID)
	return err
}

func (s *Store) DeleteTTLinksForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tt_links WHERE user_id=$1`, userID)
	return err
}

func (s *Store) TTLink(ctx context.Context, nodeID, userID uuid.UUID) (string, error) {
	var link string
	err := s.pool.QueryRow(ctx, `SELECT link FROM tt_links WHERE node_id=$1 AND user_id=$2`, nodeID, userID).Scan(&link)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return link, err
}

const (
	TrafficXray = "xray"
	TrafficHy2  = "hy2"
)

func (s *Store) ApplyXrayTraffic(ctx context.Context, nodeID, userID uuid.UUID, uplink, downlink int64) (addedUp, addedDown int64, u User, err error) {
	return s.ApplyTraffic(ctx, nodeID, userID, TrafficXray, uplink, downlink)
}

func (s *Store) ApplyTraffic(ctx context.Context, nodeID, userID uuid.UUID, source string, uplink, downlink int64) (addedUp, addedDown int64, u User, err error) {
	if source == "" {
		source = TrafficXray
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	u, err = scanUser(tx.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1 FOR UPDATE`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, User{}, ErrNotFound
		}
		return 0, 0, User{}, err
	}

	var prevUp, prevDown int64
	qerr := tx.QueryRow(ctx, `SELECT uplink, downlink FROM traffic_cursor WHERE node_id=$1 AND user_id=$2 AND source=$3 FOR UPDATE`, nodeID, userID, source).Scan(&prevUp, &prevDown)
	if qerr != nil && !errors.Is(qerr, pgx.ErrNoRows) {
		return 0, 0, User{}, qerr
	}
	addedUp = uplink - prevUp
	if addedUp < 0 {
		addedUp = uplink
	}
	addedDown = downlink - prevDown
	if addedDown < 0 {
		addedDown = downlink
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO traffic_cursor (node_id, user_id, source, uplink, downlink) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (node_id, user_id, source) DO UPDATE SET uplink=EXCLUDED.uplink, downlink=EXCLUDED.downlink`,
		nodeID, userID, source, uplink, downlink); err != nil {
		return 0, 0, User{}, err
	}
	if addedUp != 0 || addedDown != 0 {
		if _, err := tx.Exec(ctx, `UPDATE users SET upload=upload+$2, download=download+$3 WHERE id=$1`, userID, addedUp, addedDown); err != nil {
			return 0, 0, User{}, err
		}
		u.Upload += addedUp
		u.Download += addedDown
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, User{}, err
	}
	return addedUp, addedDown, u, nil
}

// ResetQuotaPeriod moves quota_period_start. When zeroCounters is true, upload/download
// become 0. traffic_cursor is left alone so the next stats poll only credits the delta.
func (s *Store) ResetQuotaPeriod(ctx context.Context, id uuid.UUID, start time.Time, zeroCounters bool) error {
	q := `UPDATE users SET quota_period_start=$2 WHERE id=$1`
	if zeroCounters {
		q = `UPDATE users SET upload=0, download=0, quota_period_start=$2 WHERE id=$1`
	}
	tag, err := s.pool.Exec(ctx, q, id, start)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
