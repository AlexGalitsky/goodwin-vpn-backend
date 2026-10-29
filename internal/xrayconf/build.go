package xrayconf

import (
	"encoding/json"
	"fmt"
	"strings"

	"website.goodwin.vpn/plane/internal/desired"
	"website.goodwin.vpn/plane/internal/hy2conf"
	"website.goodwin.vpn/plane/internal/stack"
)

const GRPCService = "goodwin"
const StatsPort = 10085

func ResolveInbounds(v desired.VLESS) []desired.XrayInbound {
	if len(v.Inbounds) > 0 {
		return v.Inbounds
	}
	port := v.Port
	if port <= 0 {
		port = 443
	}
	network := v.Network
	if network == "" {
		network = "grpc"
	}
	svc := v.ServiceName
	if svc == "" {
		svc = GRPCService
	}
	return []desired.XrayInbound{{
		Protocol:    stack.FamilyVLESS,
		Security:    "reality",
		Network:     network,
		Port:        port,
		ServiceName: svc,
		Label:       network,
	}}
}

func PrimaryPort(v desired.VLESS) int {
	ins := ResolveInbounds(v)
	if len(ins) == 0 {
		if v.Port > 0 {
			return v.Port
		}
		return 443
	}
	return ins[0].Port
}

func AllPorts(v desired.VLESS) []int {
	ins := ResolveInbounds(v)
	out := make([]int, 0, len(ins))
	seen := map[int]bool{}
	for _, in := range ins {
		if in.Port > 0 && !seen[in.Port] {
			seen[in.Port] = true
			out = append(out, in.Port)
		}
	}
	return out
}

func Build(v desired.VLESS) ([]byte, error) {
	ins := ResolveInbounds(v)
	if len(ins) == 0 {
		return nil, fmt.Errorf("no xray inbounds")
	}
	cert, key := v.Cert, v.Key
	if (cert == "" || key == "") && strings.TrimSpace(v.Hostname) != "" {
		c, k := hy2conf.CertPaths(v.Hostname)
		if cert == "" {
			cert = c
		}
		if key == "" {
			key = k
		}
	}

	inbounds := make([]any, 0, len(ins)+1)
	for i, in := range ins {
		tag := in.Label
		if tag == "" {
			tag = fmt.Sprintf("in-%d", i)
		}
		tag = "xray-" + tag
		built, err := buildInbound(tag, in, v, cert, key)
		if err != nil {
			return nil, err
		}
		inbounds = append(inbounds, built)
	}
	inbounds = append(inbounds, map[string]any{
		"tag":      "api",
		"listen":   "127.0.0.1",
		"port":     StatsPort,
		"protocol": "dokodemo-door",
		"settings": map[string]any{"address": "127.0.0.1"},
	})

	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"dns": map[string]any{
			"servers":       []string{"1.1.1.1", "8.8.8.8"},
			"queryStrategy": "UseIPv4",
		},
		"inbounds": inbounds,
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

func buildInbound(tag string, in desired.XrayInbound, v desired.VLESS, cert, key string) (map[string]any, error) {
	proto := strings.ToLower(in.Protocol)
	if proto == "ss" {
		proto = "shadowsocks"
	}
	network := in.Network
	if network == "" {
		network = "tcp"
	}

	switch proto {
	case "shadowsocks":
		method := in.Method
		if method == "" {
			method = stack.DefaultSSMethod
		}
		clients := make([]map[string]any, 0, len(v.SSUsers))
		for _, u := range v.SSUsers {
			clients = append(clients, map[string]any{
				"password": u.Password,
				"email":    u.Email,
			})
		}
		return map[string]any{
			"tag":      tag,
			"listen":   "0.0.0.0",
			"port":     in.Port,
			"protocol": "shadowsocks",
			"settings": map[string]any{
				"method":   method,
				"password": firstSSPassword(v.SSUsers),
				"clients":  clients,
				"network":  "tcp,udp",
			},
		}, nil
	case "trojan":
		clients := make([]map[string]any, 0, len(v.TrojanUsers))
		for _, u := range v.TrojanUsers {
			clients = append(clients, map[string]any{
				"password": u.Password,
				"email":    u.Email,
			})
		}
		stream, err := streamSettings(in, v, cert, key)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"tag":            tag,
			"listen":         "0.0.0.0",
			"port":           in.Port,
			"protocol":       "trojan",
			"settings":       map[string]any{"clients": clients},
			"streamSettings": stream,
			"sniffing":       sniffing(),
		}, nil
	case "vmess":
		clients := make([]map[string]any, 0, len(v.Clients))
		for _, c := range v.Clients {
			clients = append(clients, map[string]any{
				"id":      c.ID,
				"email":   c.Email,
				"alterId": 0,
			})
		}
		stream, err := streamSettings(in, v, cert, key)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"tag":            tag,
			"listen":         "0.0.0.0",
			"port":           in.Port,
			"protocol":       "vmess",
			"settings":       map[string]any{"clients": clients},
			"streamSettings": stream,
			"sniffing":       sniffing(),
		}, nil
	case "vless":
		clients := make([]map[string]any, 0, len(v.Clients))
		for _, c := range v.Clients {
			client := map[string]any{
				"id":    c.ID,
				"email": c.Email,
			}
			flow := in.Flow
			if flow == "" {
				flow = c.Flow
			}
			// Vision only on this inbound when configured; never on grpc.
			if flow != "" && network == "tcp" {
				client["flow"] = flow
			}
			clients = append(clients, client)
		}
		stream, err := streamSettings(in, v, cert, key)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"tag":      tag,
			"listen":   "0.0.0.0",
			"port":     in.Port,
			"protocol": "vless",
			"settings": map[string]any{
				"clients":    clients,
				"decryption": "none",
			},
			"streamSettings": stream,
			"sniffing":       sniffing(),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol %q", in.Protocol)
	}
}

func firstSSPassword(users []desired.SSUser) string {
	if len(users) == 0 {
		return ""
	}
	return users[0].Password
}

func sniffing() map[string]any {
	return map[string]any{
		"enabled":      true,
		"destOverride": []string{"http", "tls", "quic"},
		"routeOnly":    true,
	}
}

func streamSettings(in desired.XrayInbound, v desired.VLESS, cert, key string) (map[string]any, error) {
	network := in.Network
	if network == "" {
		network = "tcp"
	}
	security := in.Security
	stream := map[string]any{
		"network":  network,
		"security": security,
	}
	switch security {
	case "reality":
		stream["realitySettings"] = map[string]any{
			"show":        false,
			"dest":        v.Reality.Dest,
			"target":      v.Reality.Dest,
			"xver":        0,
			"serverNames": []string{v.Reality.SNI},
			"privateKey":  v.Reality.PrivateKey,
			"shortIds":    []string{v.Reality.ShortID},
		}
	case "tls":
		if cert == "" || key == "" {
			return nil, fmt.Errorf("tls inbound %s missing cert/key (hostname LE)", in.Label)
		}
		stream["tlsSettings"] = map[string]any{
			"certificates": []any{
				map[string]any{
					"certificateFile": cert,
					"keyFile":         key,
				},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported security %q", security)
	}
	switch network {
	case "grpc":
		svc := in.ServiceName
		if svc == "" {
			svc = GRPCService
		}
		stream["grpcSettings"] = map[string]any{"serviceName": svc}
	case "ws":
		path := in.Path
		if path == "" {
			path = stack.WSPath
		}
		headers := map[string]any{}
		if in.Host != "" {
			headers["Host"] = in.Host
		} else if v.Reality.SNI != "" && security == "reality" {
			headers["Host"] = v.Reality.SNI
		} else if v.Hostname != "" {
			headers["Host"] = v.Hostname
		}
		ws := map[string]any{"path": path}
		if len(headers) > 0 {
			ws["headers"] = headers
		}
		stream["wsSettings"] = ws
	case "tcp":
		// plain tcp
	default:
		return nil, fmt.Errorf("unsupported network %q", network)
	}
	return stream, nil
}
