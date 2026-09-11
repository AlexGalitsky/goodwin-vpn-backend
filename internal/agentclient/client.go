package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/desired"
	"website.goodwin.vpn/plane/internal/execcmd"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 180 * time.Second},
	}
}

type Health struct {
	OK         bool   `json:"ok"`
	Hostname   string `json:"hostname"`
	AllowExec  bool   `json:"allow_exec"`
	Version    string `json:"version"`
	XrayListen bool   `json:"xray_listen"`
	Hy2Listen  bool   `json:"hy2_listen"`
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	var h Health
	if err := c.do(ctx, http.MethodGet, "/v1/health", nil, 15*time.Second, &h); err != nil {
		return Health{}, err
	}
	return h, nil
}

type execReq struct {
	Shell      string `json:"shell"`
	TimeoutSec int    `json:"timeout_sec"`
}

func (c *Client) Apply(ctx context.Context, st desired.State) (desired.ApplyResult, error) {
	var res desired.ApplyResult
	err := c.do(ctx, http.MethodPut, "/v1/desired", st, 180*time.Second, &res)
	return res, err
}

func (c *Client) Exec(ctx context.Context, shell string, timeout time.Duration) (execcmd.Result, error) {
	sec := int(timeout / time.Second)
	if sec <= 0 {
		sec = 30
	}
	var res execcmd.Result
	err := c.do(ctx, http.MethodPost, "/v1/exec", execReq{Shell: shell, TimeoutSec: sec}, timeout+5*time.Second, &res)
	return res, err
}

func (c *Client) do(ctx context.Context, method, path string, body any, timeout time.Duration, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	httpClient := c.HTTP
	if timeout > 0 {
		clone := *c.HTTP
		clone.Timeout = timeout
		httpClient = &clone
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("agent %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
