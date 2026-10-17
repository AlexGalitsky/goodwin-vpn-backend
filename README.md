# Goodwin VPN plane

Control plane for the Flutter client.
Subscription URLs in, share links out.

This repository is a distilled public slice of a private monorepo.
The module root is this directory (`cmd/`, `admin/`, `tools/`).

HTTPS `GET /sub/{token}` adds `Goodwin-VPN: v1; base="https://…"`.
**404 (revoke) is not an empty 200.** Geo packs: `node tools/build_geo_packs.mjs --check`.

## Local

```bash
docker compose up -d db
go test ./...
SEED_DEV=1 go run ./cmd/plane   # :8080   ADMIN_PASSWORD=change-me
go run ./cmd/agent              # :19400
cd admin && npm install && npm run dev   # :5173
```

Admin: http://127.0.0.1:5173 · password `change-me`.
Public: `GET /privacy`, `GET /support`, `GET /gw/v1/service`. The client rejects `http://` `/sub`.

## VPS

Plane on a shared host (nginx reverse proxy, distro Postgres, no Docker):

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane_nginx.sh | sudo env CERT_DOMAIN=plane.example.com bash
```

Plane on a dedicated host (Caddy, Postgres in Docker):

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane.sh | sudo env CERT_DOMAIN=plane.example.com bash
```

Exit node (agent only). `SATURN_IP` is the plane address allowed to reach port 19400:

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo env CERT_DOMAIN=node.example.com SATURN_IP=203.0.113.10 bash
```

In admin, enroll the node with that hostname and token, save, then Apply.
Preset max: VLESS REALITY TCP 443, Hysteria2 UDP 443, TrustTunnel TCP+UDP 8443.
