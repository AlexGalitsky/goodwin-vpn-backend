package sub

import "testing"

func TestIncludeNode(t *testing.T) {
	ok := []string{"vless"}
	cases := []struct {
		status  string
		applied []string
		alive   bool
		want    bool
	}{
		{"ready", ok, true, true},
		{"ready", ok, false, false},
		{"offline", ok, true, true},
		{"offline", ok, false, false},
		{"degraded", ok, true, true},
		{"pending", ok, true, false},
		{"failed", ok, true, false},
		{"ready", nil, true, false},
		{"enrolled", ok, true, false},
	}
	for _, tc := range cases {
		got := IncludeNode(tc.status, tc.applied, tc.alive)
		if got != tc.want {
			t.Fatalf("%s applied=%v alive=%v: got %v want %v", tc.status, tc.applied, tc.alive, got, tc.want)
		}
	}
}
