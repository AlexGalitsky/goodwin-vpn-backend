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
}
