# Ops — plane и ноды

Канон установки и бэкапа. Продуктовая рамка P0–P8: [`archive/plan-2026-09-11.md`](./archive/plan-2026-09-11.md). Очередь: [client work-plan](https://github.com/AlexGalitsky/goodwin-vpn-client/blob/main/docs/work-plan.md).

## Local

```bash
docker compose up -d db
go test ./...
SEED_DEV=1 go run ./cmd/plane   # :8080   ADMIN_PASSWORD=change-me
go run ./cmd/agent              # :19400
cd admin && npm install && npm run dev   # :5173
```

Клиент **не импортирует** `http://` `/sub`. Для реального импорта: `PUBLIC_SUB_BASE=https://…`.

Публично без auth: `GET /privacy`, `GET /support`, `GET /gw/v1/service`, geo packs. Контракт: [`goodwin-protocol.md`](./goodwin-protocol.md).

## Panel VPS (Saturn)

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane.sh | sudo env CERT_DOMAIN=saturn.goodwin.website bash
```

Идемпотентно: `git pull` + rebuild + restart. Пароль в `/etc/goodwin-vpn-plane.env` при повторе **не** сбрасывается. Listen plane: `127.0.0.1:8080` за Caddy.

## Node VPS (exit)

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo env CERT_DOMAIN=titan.goodwin.website SATURN_IP=x.x.x.x bash
```

Нужен `SATURN_IP` / `--allow-from` для публичного `:19400`. Token печатается один раз; повторный bootstrap **сохраняет** token. `CERT_DOMAIN` — Let's Encrypt (порт 80).

Локальная сборка:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/agent ./cmd/agent
scp bin/agent tools/install_vpn_node.mjs root@VPS:/tmp/
sudo node /tmp/install_vpn_node.mjs --bin /tmp/agent --allow-from SATURN_IP
```

В админке: New node → группа + hostname = `CERT_DOMAIN` → Apply. После смены hostname/стека/групп — **Сохранить**, потом Apply (dirty блокирует Apply all).

## Стек портов (типичный max)

| Сервис | Порт |
|--------|------|
| VLESS REALITY | TCP 443 |
| Hysteria2 | UDP 443 |
| TrustTunnel | 8443 TCP+UDP |
| Agent | 19400 только с Saturn |

## Backup / restore (Postgres)

Timer: `goodwin-plane-pgdump.timer` (03:17). Файлы: `/var/backups/goodwin-plane/plane-YYYYMMDD.sql.gz` (14 дней).

```bash
sudo node /opt/goodwin-vpn-plane/pg_dump_plane.mjs
sudo node /opt/goodwin-vpn-plane/pg_dump_plane.mjs --restore /var/backups/goodwin-plane/plane-YYYYMMDD.sql.gz
# → база plane_restore (live plane не трогается)
```

На прод только после свежего dump и проверки копии:

```bash
sudo systemctl stop goodwin-vpn-plane
sudo node /opt/goodwin-vpn-plane/pg_dump_plane.mjs --restore … --i-mean-live
sudo systemctl start goodwin-vpn-plane
```

Потеря DB = REALITY keys + sub tokens + traffic cursors.

## После Apply ядер

`systemctl cat goodwin-xray` должен содержать `ProtectSystem=strict` и `Restart=on-failure` (не голый root unit).
