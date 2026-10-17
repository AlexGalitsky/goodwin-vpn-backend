package sub

import "time"

// Entitled is whether the user should receive share links and stay in node Apply.
// Disabled, revoked, expired, or over quota → empty 200 (not 404). 404 is only an unknown token.
func Entitled(status string, expire *time.Time, upload, download, total int64, now time.Time) bool {
	switch status {
	case "active":
	default:
		return false
	}
	if expire != nil && now.After(*expire) {
		return false
	}
	if total > 0 && upload+download >= total {
		return false
	}
	return true
}
