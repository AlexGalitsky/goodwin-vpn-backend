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

Dev user after seed: `http://127.0.0.1:8080/sub/dev-sub-token` (404 until a node is enrolled and attached to group `dev`). The Flutter app **rejects http://** — for a real import set `PUBLIC_SUB_BASE=https://…`.

Public privacy policy (no auth): `GET /privacy` → `https://saturn.goodwin.website/privacy`. The Flutter app opens this URL from Settings → Privacy.

## Panel VPS (API + admin, not the exit node)

Debian/Ubuntu, **root**. First install and later updates are the same command (git pull + rebuild + restart):

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane.sh | sudo env CERT_DOMAIN=saturn.goodwin.website bash
```

Пароль админки пишется в `/etc/goodwin-vpn-plane.env` и при повторном запуске **не сбрасывается**.

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

Copy IPv4 + token into admin → New node (pick **one** group, hostname = `CERT_DOMAIN`). Then **Exec** on the node card (`uname -a`, `ss -lntp`). Apply.

Повторный запуск bootstrap на ноде **сохраняет token** и обновляет agent. Не нужен новый Enroll.

## Two nodes (P4)

Готово, когда две ноды в разных группах и мёртвый agent пропадает из `/sub`.

## Hysteria2 (P5)

REALITY остаётся на **TCP 443**. Hy2 слушает **UDP 443** с сертификатом Let's Encrypt (`sni` = hostname ноды, без obfs).

1. Обновить панель:

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane.sh | sudo env CERT_DOMAIN=saturn.goodwin.website bash
```

2. Обновить **обе** ноды (token не сменится). UDP 443 должен быть открыт:

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo env CERT_DOMAIN=titan.goodwin.website bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo env CERT_DOMAIN=mimas.goodwin.website bash
```

3. Проверить сертификат: `ls /etc/letsencrypt/live/titan.goodwin.website/` (и то же для mimas). Если нет — тот же bootstrap с `CERT_DOMAIN`.
4. Admin → Nodes → Apply на titan и mimas. Status `ready`, Preview содержит `hysteria2://…` с `sni=` hostname.
5. В `app/` Refresh и Connect по Hy2. VLESS на тех же нодах должен продолжать работать.

## TrustTunnel (P6)

TT на **8443** (TCP+UDP). 443 остаётся REALITY+Hy2 — форма/API не дают посадить TT на тот же порт.

1. Обновить панель и **обе** ноды (token сохранится) — те же bootstrap-команды, что в P5.
2. TCP **и** UDP 8443 должны быть открыты.
3. Apply на titan и mimas. Preview: третья строка `tt://?…` (официальный deeplink, не self-signed).
4. В `app/` Refresh и Connect по TrustTunnel. VLESS и Hy2 не должны сломаться.

## Quotas, expire, revoke (P7)

Только панель (ноды не трогать). Disable по-прежнему даёт **пустое 200** — Refresh в приложении снимает ноды. Revoke меняет токен: старый URL отвечает **404** (клиент ноды не трогает, пока пользователь сам не обновит ссылку).

1. Обновить панель:

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane.sh | sudo env CERT_DOMAIN=saturn.goodwin.website bash
```

2. Groups: квота GiB и срок (сутки / неделя / без срока) для новых пользователей. Протоколы группы — чекбоксы.
3. Users: срок при создании, «+ сутки / + неделя», новая ссылка (ротация), отзыв, удаление.
4. Инструменты: exec на ноде и аудит. Exec не выключаем.

## Operator console (P8)

Панель (ноды не трогать):

```bash
curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane.sh | sudo env CERT_DOMAIN=saturn.goodwin.website bash
```

Обзор, поиск пользователей, чекбоксы групп на ноде, удаление пустой группы / пользователя / ноды из панели.

`allow_exec` is on by default. Leave it on for this fleet.
