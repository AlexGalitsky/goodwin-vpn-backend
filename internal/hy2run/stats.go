package hy2run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/xrayrun"
)

const TrafficAddr = "127.0.0.1:19999"

type trafficCounters struct {
	Tx int64 `json:"tx"`
	Rx int64 `json:"rx"`
}

func QueryTraffic(ctx context.Context, addr string) ([]xrayrun.UserBytes, error) {
	if strings.TrimSpace(addr) == "" {
		addr = TrafficAddr
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/traffic", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("hy2 traffic %s", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return ParseTraffic(raw), nil
}

func ParseTraffic(raw []byte) []xrayrun.UserBytes {
	var wrap map[string]trafficCounters
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil
	}
	out := make([]xrayrun.UserBytes, 0, len(wrap))
	for id, c := range wrap {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out = append(out, xrayrun.UserBytes{
			Email:    id,
			Uplink:   c.Rx,
			Downlink: c.Tx,
		})
	}
	return out
}
