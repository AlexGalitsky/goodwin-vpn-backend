package sysd

import "strings"

// Extra is appended to core systemd [Service] blocks. ProtectSystem=strict
// still allows reading /etc/letsencrypt; write only the agent prefix.
const Extra = `KillMode=control-group
TimeoutStopSec=20
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
LockPersonality=true
RestrictSUIDSGID=true
RestrictRealtime=true
ReadWritePaths=/opt/goodwin-vpn-agent
`

const PlaneExtra = `KillMode=control-group
TimeoutStopSec=20
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/goodwin-vpn-plane
`

// Hardened reports whether a unit is not a bare root service.
func Hardened(unit string) bool {
	return strings.Contains(unit, "ProtectSystem=strict") &&
		strings.Contains(unit, "NoNewPrivileges=true") &&
		strings.Contains(unit, "Restart=on-failure") &&
		strings.Contains(unit, "KillMode=control-group")
}
