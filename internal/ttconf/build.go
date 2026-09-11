package ttconf

import (
	"fmt"
	"strconv"
	"strings"

	"website.goodwin.vpn/plane/internal/desired"
	"website.goodwin.vpn/plane/internal/hy2conf"
)

func CertPaths(hostname string) (string, string) {
	return hy2conf.CertPaths(hostname)
}

func VPN(port int, credentialsPath string) []byte {
	if port <= 0 {
		port = 8443
	}
	return []byte(fmt.Sprintf(`listen_address = "0.0.0.0:%d"
ipv6_available = false
allow_private_network_connections = false
credentials_file = %s

[listen_protocols.http1]
upload_buffer_size = 32768

[listen_protocols.http2]
initial_connection_window_size = 8388608
initial_stream_window_size = 131072
max_concurrent_streams = 1000

[listen_protocols.quic]
recv_udp_payload_size = 1350
send_udp_payload_size = 1350
enable_early_data = true

[forward_protocol]
direct = {}
`, port, strconv.Quote(credentialsPath)))
}

func Hosts(hostname, cert, key string) []byte {
	return []byte(fmt.Sprintf(`[[main_hosts]]
hostname = %s
cert_chain_path = %s
private_key_path = %s
`, strconv.Quote(hostname), strconv.Quote(cert), strconv.Quote(key)))
}

func Credentials(users []desired.TTUser) ([]byte, error) {
	var b strings.Builder
	n := 0
	for _, u := range users {
		user := strings.TrimSpace(u.Username)
		pass := strings.TrimSpace(u.Password)
		if user == "" || pass == "" {
			continue
		}
		fmt.Fprintf(&b, "[[client]]\nusername = %s\npassword = %s\n\n", strconv.Quote(user), strconv.Quote(pass))
		n++
	}
	if n == 0 {
		return nil, fmt.Errorf("tt credentials: no users")
	}
	return []byte(b.String()), nil
}
