# Goodwin VPN plane

Control plane for the universal Flutter client (`goodwin-vpn-client` `app/`).  
Subscription URLs in, share links out. Spec: [`plan.md`](./plan.md).

## Local

```bash
docker compose up -d db
go test ./...
go run ./cmd/plane          # :8080   ADMIN_PASSWORD=change-me
go run ./cmd/agent          # :19400  prints token if AGENT_TOKEN unset
cd admin && npm install && npm run dev   # :5173
```

Admin: http://127.0.0.1:5173 password `change-me`.

Dev user after seed: `http://127.0.0.1:8080/sub/dev-sub-token` (404 until a node is enrolled and attached to group `dev`).

## Node on a VPS (from GitHub)

На чистом Debian/Ubuntu, **root**:

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo env CERT_DOMAIN=titan.goodwin.website bash
```

Скрипт ставит Node.js 22, Go, клонирует репо, собирает agent и вызывает `tools/install_vpn_node.mjs`. Печатает token для админки. `CERT_DOMAIN` — Let's Encrypt на этот hostname (порт 80). Saturn / админку скрипт не ставит.

Приватный репо:

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo env GITHUB_TOKEN=ghp_... CERT_DOMAIN=titan.goodwin.website bash
```

Или локальная сборка:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/agent ./cmd/agent
scp bin/agent tools/install_vpn_node.mjs root@VPS:/tmp/
sudo node /tmp/install_vpn_node.mjs --bin /tmp/agent
```

Copy IPv4 + token into admin → New node. Then **Exec** on the node card (`uname -a`, `ss -lntp`).

`allow_exec` is on by default for development. Turn off with `--no-exec` before giving the panel to anyone else.
