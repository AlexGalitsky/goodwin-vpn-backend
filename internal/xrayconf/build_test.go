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
	if len(inbounds) != 1 {
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
	if len(rules) != 2 {
		t.Fatalf("rules %d", len(rules))
	}
	r0 := rules[0].(map[string]any)
	if r0["outboundTag"] != "block" {
		t.Fatalf("bt rule %+v", r0)
	}
	protos, _ := r0["protocol"].([]any)
	if len(protos) != 1 || protos[0] != "bittorrent" {
		t.Fatalf("bt protocol %+v", r0)
	}
	r1 := rules[1].(map[string]any)
	if r1["outboundTag"] != "block" || r1["port"] != "25,465,587" {
		t.Fatalf("smtp rule %+v", r1)
	}
	block := outs[1].(map[string]any)
	if block["tag"] != "block" || block["protocol"] != "blackhole" {
		t.Fatalf("block outbound %+v", block)
	}
}
