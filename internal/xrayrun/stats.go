package xrayrun

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const StatsAddr = "127.0.0.1:10085"

type UserBytes struct {
	Email    string `json:"email"`
	Uplink   int64  `json:"uplink"`
	Downlink int64  `json:"downlink"`
}

func QueryUserStats(ctx context.Context, bin, server string) ([]UserBytes, error) {
	if strings.TrimSpace(bin) == "" {
		return nil, nil
	}
	if _, err := os.Stat(bin); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(server) == "" {
		server = StatsAddr
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, bin, "api", "statsquery", "-s", server)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return ParseUserStats(raw), nil
}

func ParseUserStats(raw []byte) []UserBytes {
	raw = bytes.TrimSpace(raw)
	if i := bytes.IndexByte(raw, '{'); i > 0 {
		raw = raw[i:]
	}
	var wrap struct {
		Stat []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"stat"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil
	}
	byEmail := map[string]*UserBytes{}
	var order []string
	for _, st := range wrap.Stat {
		email, dir := parseUserStatName(st.Name)
		if email == "" || dir == "" {
			continue
		}
		n := parseStatValue(st.Value)
		u, ok := byEmail[email]
		if !ok {
			u = &UserBytes{Email: email}
			byEmail[email] = u
			order = append(order, email)
		}
		switch dir {
		case "uplink":
			u.Uplink = n
		case "downlink":
			u.Downlink = n
		}
	}
	out := make([]UserBytes, 0, len(order))
	for _, email := range order {
		out = append(out, *byEmail[email])
	}
	return out
}

func parseUserStatName(name string) (email, dir string) {
	// user>>>{email}>>>traffic>>>uplink|downlink
	parts := strings.Split(name, ">>>")
	if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
		return "", ""
	}
	email = strings.TrimSpace(parts[1])
	dir = parts[3]
	if email == "" || (dir != "uplink" && dir != "downlink") {
		return "", ""
	}
	return email, dir
}

func parseStatValue(raw json.RawMessage) int64 {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return 0
		}
		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil {
		return 0
	}
	return n
}

func Delta(prev, cur int64) int64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}
