package sysd

// Extra is appended to core systemd [Service] blocks. ProtectSystem=strict
// still allows reading /etc/letsencrypt; write only the agent prefix.
const Extra = `KillMode=control-group
TimeoutStopSec=20
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
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
