# Goodwin VPN plane

Control plane for the Flutter client (`goodwin-vpn-client` `app/`).  
Subscription URLs in, share links out.

| | |
|--|--|
| **Ops** (install, backup, Apply) | [`docs/ops.md`](./docs/ops.md) |
| **Протокол plane** | [`docs/goodwin-protocol.md`](./docs/goodwin-protocol.md) |
| **Клиентский контракт** | [goodwin-vpn-client protocol](https://github.com/AlexGalitsky/goodwin-vpn-client/blob/main/docs/goodwin-protocol.md) |
| **Очередь** | [client work-plan](https://github.com/AlexGalitsky/goodwin-vpn-client/blob/main/docs/work-plan.md) |
| **Docs index** | [`docs/README.md`](./docs/README.md) |

HTTPS `GET /sub/{token}` добавляет `Goodwin-VPN: v1; base="https://…"`.  
**404 (revoke) ≠ пустое 200** — не менять. Geo: `node tools/build_geo_packs.mjs --check`.

## Local

```bash
docker compose up -d db
go test ./...
SEED_DEV=1 go run ./cmd/plane   # :8080   ADMIN_PASSWORD=change-me
go run ./cmd/agent              # :19400
cd admin && npm install && npm run dev   # :5173
```

Admin: http://127.0.0.1:5173 · password `change-me`.  
Публично: `GET /privacy`, `GET /support`, `GET /gw/v1/service`. Клиент отвергает `http://` `/sub`.

Установка на VPS, ноды, backup — **[`docs/ops.md`](./docs/ops.md)**.
