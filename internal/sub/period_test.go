package sub

import (
	"testing"
	"time"
)

func TestParseQuotaReset(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", QuotaResetNone, false},
		{" none ", QuotaResetNone, false},
		{"lifetime", QuotaResetNone, false},
		{"day", QuotaResetDay, false},
		{"Daily", QuotaResetDay, false},
		{"week", QuotaResetWeek, false},
		{"month", QuotaResetMonth, false},
		{"hourly", "", true},
		{"year", "", true},
	}
	for _, tc := range cases {
		got, err := ParseQuotaReset(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q: want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %q %v want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestPeriodStart(t *testing.T) {
	sunday := time.Date(2026, 9, 13, 15, 4, 5, 0, time.UTC)
	monday := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		now  time.Time
		kind string
		want time.Time
	}{
		{"day", sunday, QuotaResetDay, time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)},
		{"week sunday → monday prior", sunday, QuotaResetWeek, time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)},
		{"week monday", monday, QuotaResetWeek, monday},
		{"month", sunday, QuotaResetMonth, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"none", sunday, QuotaResetNone, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PeriodStart(tc.now, tc.kind); !got.Equal(tc.want) {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestPeriodDue(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	today := PeriodStart(now, QuotaResetDay)
	yesterday := today.AddDate(0, 0, -1)
	cases := []struct {
		name  string
		kind  string
		start *time.Time
		want  bool
	}{
		{"lifetime", "", nil, false},
		{"adopt nil", QuotaResetDay, nil, true},
		{"same day", QuotaResetDay, &today, false},
		{"rolled", QuotaResetDay, &yesterday, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PeriodDue(tc.kind, tc.start, now); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
