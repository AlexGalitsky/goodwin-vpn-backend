package hy2run

import "testing"

func TestParseTraffic(t *testing.T) {
	got := ParseTraffic([]byte(`{
		"11111111-2222-3333-4444-555555555555": {"tx": 20, "rx": 10},
		" ": {"tx": 1, "rx": 1}
	}`))
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if got[0].Email != "11111111-2222-3333-4444-555555555555" {
		t.Fatal(got[0].Email)
	}
	if got[0].Uplink != 10 || got[0].Downlink != 20 {
		t.Fatalf("rx→uplink tx→downlink %+v", got[0])
	}
}
