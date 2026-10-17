package hy2conf

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"website.goodwin.vpn/plane/internal/desired"
)

const AuthScript = `#!/usr/bin/env node
const fs = require("fs");
const path = require("path");
const auth = process.argv[3] || "";
const users = JSON.parse(fs.readFileSync(path.join(__dirname, "hy2-users.json"), "utf8"));
const hit = users.find((u) => u.password === auth);
if (!hit) process.exit(1);
process.stdout.write(hit.id + "\n");
`

func CertPaths(hostname string) (cert, key string) {
	host := strings.TrimSpace(hostname)
	base := "/etc/letsencrypt/live/" + host
	return base + "/fullchain.pem", base + "/privkey.pem"
}

func Build(h desired.Hy2, authCmd string) ([]byte, error) {
	if h.Port <= 0 {
		h.Port = 443
	}
	cert, key := h.Cert, h.Key
	if cert == "" || key == "" {
		c, k := CertPaths(h.Hostname)
		if cert == "" {
			cert = c
		}
		if key == "" {
			key = k
		}
	}
	if strings.TrimSpace(authCmd) == "" {
		return nil, fmt.Errorf("hy2 auth command required")
	}
	cfg := fmt.Sprintf(`listen: 0.0.0.0:%d
tls:
  cert: %s
  key: %s
auth:
  type: command
  command: %s
masquerade:
  type: proxy
  proxy:
    url: https://www.cloudflare.com/
    rewriteHost: true
trafficStats:
  listen: 127.0.0.1:19999
acl:
  inline:
    - reject(all, tcp/25)
    - reject(all, tcp/465)
    - reject(all, tcp/587)
`, h.Port, strconv.Quote(cert), strconv.Quote(key), strconv.Quote(authCmd))
	return []byte(cfg), nil
}

func UsersJSON(users []desired.Hy2User) ([]byte, error) {
	if users == nil {
		users = []desired.Hy2User{}
	}
	return json.MarshalIndent(users, "", "  ")
}
