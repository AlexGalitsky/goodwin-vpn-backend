package sub

import (
	"testing"
	"time"
)

func TestEntitled(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name          string
		status        string
		expire        *time.Time
		up, down, tot int64
		want          bool
	}{
		{"active", "active", nil, 0, 0, 0, true},
		{"disabled", "disabled", nil, 0, 0, 0, false},
		{"revoked", "revoked", nil, 0, 0, 0, false},
		{"expired", "active", &past, 0, 0, 0, false},
		{"future", "active", &future, 0, 0, 0, true},
		{"over quota", "active", nil, 50, 50, 100, false},
		{"under quota", "active", nil, 40, 50, 100, true},
		{"unlimited with traffic", "active", nil, 1, 1, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Entitled(tc.status, tc.expire, tc.up, tc.down, tc.tot, now); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
