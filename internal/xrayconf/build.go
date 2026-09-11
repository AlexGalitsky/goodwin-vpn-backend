package xrayconf

import (
	"encoding/json"

	"website.goodwin.vpn/plane/internal/desired"
)

const GRPCService = "goodwin"
const StatsPort = 10085

func Build(v desired.VLESS) ([]byte, error) {
	clients := make([]map[string]any, 0, len(v.Clients))
	for _, c := range v.Clients {
		client := map[string]any{
			"id":    c.ID,
			"email": c.Email,
		}
		if c.Flow != "" {
			client["flow"] = c.Flow
		}
		clients = append(clients, client)
	}
	network := v.Network
	if network == "" {
		network = "grpc"
	}
	svc := v.ServiceName
	if svc == "" {
		svc = GRPCService
	}
	stream := map[string]any{
		"network":  network,
		"security": "reality",
		"realitySettings": map[string]any{
			"show":        false,
			"dest":        v.Reality.Dest,
			"target":      v.Reality.Dest,
			"xver":        0,
			"serverNames": []string{v.Reality.SNI},
			"privateKey":  v.Reality.PrivateKey,
			"shortIds":    []string{v.Reality.ShortID},
		},
	}
	if network == "grpc" {
		stream["grpcSettings"] = map[string]any{"serviceName": svc}
	}
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"dns": map[string]any{
			"servers":       []string{"1.1.1.1", "8.8.8.8"},
			"queryStrategy": "UseIPv4",
		},
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
				"streamSettings": stream,
				"sniffing": map[string]any{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
					"routeOnly":    true,
				},
			},
			map[string]any{
				"tag":      "api",
				"listen":   "127.0.0.1",
				"port":     StatsPort,
				"protocol": "dokodemo-door",
				"settings": map[string]any{"address": "127.0.0.1"},
			},
		},
		"outbounds": []any{
			map[string]any{
				"protocol": "freedom",
				"tag":      "direct",
				"settings": map[string]any{"domainStrategy": "UseIPv4"},
			},
			map[string]any{"protocol": "blackhole", "tag": "block"},
		},
		"stats": map[string]any{},
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"StatsService"},
		},
		"policy": map[string]any{
			"levels": map[string]any{
				"0": map[string]any{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
		},
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules": []any{
				map[string]any{
					"type":        "field",
					"inboundTag":  []string{"api"},
					"outboundTag": "api",
				},
				map[string]any{
					"type":        "field",
					"protocol":    []string{"bittorrent"},
					"outboundTag": "block",
				},
				map[string]any{
					"type":        "field",
					"port":        "25,465,587",
					"outboundTag": "block",
				},
			},
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}
