package xrayconf

import (
	"encoding/json"

	"website.goodwin.vpn/plane/internal/desired"
)

func Build(v desired.VLESS) ([]byte, error) {
	clients := make([]map[string]any, 0, len(v.Clients))
	for _, c := range v.Clients {
		flow := c.Flow
		if flow == "" {
			flow = "xtls-rprx-vision"
		}
		clients = append(clients, map[string]any{
			"id":    c.ID,
			"email": c.Email,
			"flow":  flow,
		})
	}
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"tag":      "vless-reality",
				"listen":   "0.0.0.0",
				"port":     v.Port,
				"protocol": "vless",
				"settings": map[string]any{
					"clients":    clients,
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]any{
						"show":        false,
						"dest":        v.Reality.Dest,
						"xver":        0,
						"serverNames": []string{v.Reality.SNI},
						"privateKey":  v.Reality.PrivateKey,
						"shortIds":    []string{v.Reality.ShortID},
					},
				},
				"sniffing": map[string]any{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
				},
			},
		},
		"outbounds": []any{
			map[string]any{"protocol": "freedom", "tag": "direct"},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}
