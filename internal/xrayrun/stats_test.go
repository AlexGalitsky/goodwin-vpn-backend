package xrayrun

import "testing"

func TestParseUserStats(t *testing.T) {
	raw := []byte(`{
  "stat": [
    {"name": "inbound>>>vless-reality>>>traffic>>>uplink", "value": 99},
    {"name": "user>>>11111111-2222-3333-4444-555555555555>>>traffic>>>uplink", "value": "100"},
    {"name": "user>>>11111111-2222-3333-4444-555555555555>>>traffic>>>downlink", "value": 250},
    {"name": "user>>>aaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee>>>traffic>>>uplink", "value": 7}
  ]
}`)
	got := ParseUserStats(raw)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].Email != "11111111-2222-3333-4444-555555555555" || got[0].Uplink != 100 || got[0].Downlink != 250 {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].Email != "aaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" || got[1].Uplink != 7 || got[1].Downlink != 0 {
		t.Fatalf("second %+v", got[1])
	}
}

func TestParseUserStatsLogPreamble(t *testing.T) {
	raw := []byte("warning: skip\n{\"stat\":[{\"name\":\"user>>>u1>>>traffic>>>downlink\",\"value\":3}]}")
	got := ParseUserStats(raw)
	if len(got) != 1 || got[0].Email != "u1" || got[0].Downlink != 3 {
		t.Fatalf("%+v", got)
	}
}

func TestDelta(t *testing.T) {
	if Delta(10, 15) != 5 {
		t.Fatal("grow")
	}
	if Delta(20, 3) != 3 {
		t.Fatal("restart")
	}
	if Delta(0, 0) != 0 {
		t.Fatal("zero")
	}
}
