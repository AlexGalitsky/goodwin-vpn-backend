package sub

import (
	"fmt"
	"strings"
	"time"
)

const (
	QuotaResetNone  = ""
	QuotaResetDay   = "day"
	QuotaResetWeek  = "week"
	QuotaResetMonth = "month"
)

// ParseQuotaReset accepts day/week/month, or empty/none/lifetime for a one-shot total.
func ParseQuotaReset(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none", "lifetime", "never":
		return QuotaResetNone, nil
	case "day", "daily":
		return QuotaResetDay, nil
	case "week", "weekly":
		return QuotaResetWeek, nil
	case "month", "monthly":
		return QuotaResetMonth, nil
	default:
		return "", fmt.Errorf("quota_reset")
	}
}

// PeriodStart is the UTC boundary of the period that contains now.
// Day: midnight. Week: Monday 00:00. Month: the 1st 00:00.
func PeriodStart(now time.Time, kind string) time.Time {
	now = now.UTC()
	switch kind {
	case QuotaResetDay:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case QuotaResetWeek:
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		d := now.AddDate(0, 0, -(wd - 1))
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	case QuotaResetMonth:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
}

// PeriodDue is true when quota_period_start should move to PeriodStart(now).
// A nil start with a non-empty kind is due (first adopt after enabling a period).
func PeriodDue(kind string, periodStart *time.Time, now time.Time) bool {
	switch kind {
	case QuotaResetDay, QuotaResetWeek, QuotaResetMonth:
	default:
		return false
	}
	start := PeriodStart(now, kind)
	if periodStart == nil || periodStart.IsZero() {
		return true
	}
	return periodStart.UTC().Before(start)
}
