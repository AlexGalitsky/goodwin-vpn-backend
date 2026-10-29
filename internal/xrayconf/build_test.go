package xrayconf

import (
	"encoding/json"
	"testing"

	"website.goodwin.vpn/plane/internal/desired"
	"website.goodwin.vpn/plane/internal/reality"
)

func TestBuild(t *testing.T) {
	raw, err := Build(desired.VLESS{
		Port: 443,
		Reality: reality.Keys{
			PrivateKey: "priv",
			PublicKey:  "pub",
			ShortID:    "abcd1234",
			Dest:       reality.DefaultDest,
			SNI:        reality.DefaultSNI,
		},
		Clients: []desired.VLESSClient{{ID: "11111111-2222-3333-4444-555555555555", Email: "u1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	inbounds, _ := m["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("inbounds %d", len(inbounds))
	}
	in := inbounds[0].(map[string]any)
	stream := in["streamSettings"].(map[string]any)
	if stream["network"] != "grpc" {
		t.Fatalf("network %v", stream["network"])
	}
	if _, ok := stream["grpcSettings"]; !ok {
		t.Fatal("missing grpcSettings")
	}
	clients := in["settings"].(map[string]any)["clients"].([]any)
	c0 := clients[0].(map[string]any)
	if _, ok := c0["flow"]; ok {
		t.Fatalf("grpc clients must not set vision flow: %+v", c0)
	}
	sniff := in["sniffing"].(map[string]any)
	if sniff["routeOnly"] != true {
		t.Fatalf("sniffing %+v", sniff)
	}
	outs := m["outbounds"].([]any)
	direct := outs[0].(map[string]any)
	if direct["settings"].(map[string]any)["domainStrategy"] != "UseIPv4" {
		t.Fatalf("outbound %+v", direct)
	}
	routing, ok := m["routing"].(map[string]any)
	if !ok {
		t.Fatal("missing routing")
	}
	rules, _ := routing["rules"].([]any)
	if len(rules) != 3 {
		t.Fatalf("rules %d", len(rules))
	}
	apiRule := rules[0].(map[string]any)
	if apiRule["outboundTag"] != "api" {
		t.Fatalf("api rule %+v", apiRule)
	}
	r0 := rules[1].(map[string]any)
	if r0["outboundTag"] != "block" {
		t.Fatalf("bt rule %+v", r0)
	}
	protos, _ := r0["protocol"].([]any)
	if len(protos) != 1 || protos[0] != "bittorrent" {
		t.Fatalf("bt protocol %+v", r0)
	}
	r1 := rules[2].(map[string]any)
	if r1["outboundTag"] != "block" || r1["port"] != "25,465,587" {
		t.Fatalf("smtp rule %+v", r1)
	}
	if _, ok := m["stats"]; !ok {
		t.Fatal("missing stats")
	}
	pol := m["policy"].(map[string]any)
	lvl := pol["levels"].(map[string]any)["0"].(map[string]any)
	if lvl["statsUserUplink"] != true || lvl["statsUserDownlink"] != true {
		t.Fatalf("policy %+v", pol)
	}
	block := outs[1].(map[string]any)
	if block["tag"] != "block" || block["protocol"] != "blackhole" {
		t.Fatalf("block outbound %+v", block)
	}
}

func TestBuildMultiInbound(t *testing.T) {
	raw, err := Build(desired.VLESS{
		Hostname: "node.example.com",
		Cert:     "/tmp/fullchain.pem",
		Key:      "/tmp/privkey.pem",
		Reality: reality.Keys{
			PrivateKey: "priv",
			PublicKey:  "pub",
			ShortID:    "abcd1234",
			Dest:       reality.DefaultDest,
			SNI:        reality.DefaultSNI,
		},
		Clients:     []desired.VLESSClient{{ID: "11111111-2222-3333-4444-555555555555", Email: "u1"}},
		TrojanUsers: []desired.TrojanUser{{ID: "u1", Password: "trojan", Email: "u1"}},
		SSUsers:     []desired.SSUser{{ID: "u1", Password: "sspass", Email: "u1"}},
		Inbounds: []desired.XrayInbound{
			{Protocol: "vless", Security: "reality", Network: "grpc", Port: 443, ServiceName: "goodwin", Label: "grpc"},
			{Protocol: "vless", Security: "reality", Network: "tcp", Port: 9443, Flow: "xtls-rprx-vision", Label: "tcp"},
			{Protocol: "vless", Security: "reality", Network: "ws", Port: 2083, Path: "/goodwin", Label: "ws"},
			{Protocol: "vless", Security: "tls", Network: "tcp", Port: 10443, Label: "vless-tls"},
			{Protocol: "vmess", Security: "tls", Network: "ws", Port: 2096, Path: "/goodwin", Label: "vmess"},
			{Protocol: "trojan", Security: "tls", Network: "tcp", Port: 10444, Label: "trojan"},
			{Protocol: "shadowsocks", Network: "tcp", Port: 8388, Method: "aes-256-gcm", Label: "ss"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	inbounds, _ := m["inbounds"].([]any)
	if len(inbounds) != 8 {
		t.Fatalf("inbounds %d", len(inbounds))
	}
	tcp := inbounds[1].(map[string]any)
	c0 := tcp["settings"].(map[string]any)["clients"].([]any)[0].(map[string]any)
	if c0["flow"] != "xtls-rprx-vision" {
		t.Fatalf("vision flow %+v", c0)
	}
	grpc := inbounds[0].(map[string]any)
	g0 := grpc["settings"].(map[string]any)["clients"].([]any)[0].(map[string]any)
	if _, ok := g0["flow"]; ok {
		t.Fatalf("grpc must not have flow %+v", g0)
	}
	ws := inbounds[2].(map[string]any)
	if _, ok := ws["streamSettings"].(map[string]any)["wsSettings"]; !ok {
		t.Fatal("missing wsSettings")
	}
	ss := inbounds[6].(map[string]any)
	if ss["protocol"] != "shadowsocks" {
		t.Fatalf("ss %+v", ss)
	}
}
