# Control plane — план продукта

**Дата:** 2026-09-11  
**Клиент:** универсальный Flutter `app/` (`goodwin-vpn-client`).  
**Сервер:** этот репозиторий.  
**Не входит:** `dozvoleno/`, ЛК, биллинг, OTP, Sing-Box.

Цель: админ поднимает VPS, ставит ноду скриптом из git, в админке указывает IP и токен, выбирает стек без конфликтов портов, назначает группы. Пользователь в `app/` вставляет HTTPS-подписку и подключается.

Клиент **не менять**, пока плоскость не отдаёт тело, которое уже проходит `subscription_parser_test` / `subscription_client_test`.

---

## Не делать

- Писать VLESS / Hysteria / TrustTunnel с нуля — только официальные бинарники.
- Клонировать Flutter-репо на VPS — там agent + ядра.
- Clash YAML, `ss://`, xhttp / httpupgrade, Hysteria obfs.
- Self-signed для TrustTunnel (клиент их не принимает).
- Панель и exit-нода на одном IP.

---

## Стек

| Часть | Стек |
|--------|------|
| API `cmd/plane` | Go, Postgres 16, `pgx`, goose-миграции |
| Agent `cmd/agent` | тот же Go-модуль, static binary, systemd |
| Admin `admin/` | Vite + React + TypeScript |
| Инсталлятор | `tools/bootstrap_vpn_node.sh` (ставит Node) → `node tools/install_vpn_node.mjs` |
| Ядра на ноде | официальные релизы: Xray-core, hysteria, `trusttunnel_endpoint` |
| TLS панели | Caddy: отдельно админка и публичный `/sub` |

Один модуль, общие типы `DesiredState`. Redis / k8s / Nest — не в v1.

Админ в v1: пароль из env + cookie, IP allowlist позже.

---

## Удалённые команды на VPS (dev)

Пока флот маленький, агент умеет выполнить команду на хосте по запросу из админки. Это нужно и для отладки Apply, и чтобы не ходить на каждую машину по SSH.

| | |
|--|--|
| Админка | экран ноды: поле команды + вывод stdout/stderr/exit |
| Plane | `POST /v1/nodes/:id/exec` (сессия админа), аудит в `audit_events` |
| Agent | `POST /v1/exec`, `Authorization: Bearer <node token>` |
| Таймаут | 30 с по умолчанию, максимум 120 с, вывод ≤ 1 MiB |
| Флаг | `allow_exec: true` в конфиге агента (инсталлятор включает в dev) |

Команда идёт через `/bin/sh -c` на ноде. Агент, скорее всего, root (порты 443) — не оставлять `allow_exec` на продакшен-ноде с чужими админами. Enroll-токен после P1 заменить на mTLS; до этого plane хранит токен ноды, чтобы стучаться на `:19400` (только для dev).

---

## Два репозитория

| Репо | Роль |
|------|------|
| `goodwin-vpn-client` | Клиент. Контракт подписки заморожен тестами. |
| Этот (`VPN` / plane) | Админка, API, agent, инсталлятор, рендер подписки. |

Связь только по HTTPS: Admin → Plane API → Node agent → Xray / Hy2 / TT. Пользовательский трафик на панель не идёт, только `GET /sub/{token}`.

---

## Контракт подписки

Источник: `app/lib/session/subscription_*.dart`, парсеры `goodwin_vpn_core`.

- URL только `https`; redirect не должен уходить на http.
- UA `goodwin-vpn/1.0`, тело ≤ 1 MiB.
- Тело: по одной share-ссылке на строку (или Base64 этого текста).
- Схемы: `vless`, `vmess`, `trojan`, `hysteria2`, `hy2`, `tt`.
- Заголовки: `profile-title`, `profile-update-interval`, `subscription-userinfo` (`upload; download; total; expire`).
- VLESS: `type` только tcp/ws/grpc; REALITY — `pbk`, `sid`, `sni`, `fp`, `flow=xtls-rprx-vision`.
- Hy2: пароль + `sni` = SAN сертификата, без obfs.
- TT: `tt://?` TLV v1 с официального `trusttunnel_endpoint -c … --format deeplink`.

Импорт в приложении: вставка, QR, `goodwin://import?url=`.

---

## Админ-поток

1. VPS с IP, отдельным от панели.
2. Скрипт ставит пустой agent: порт **19400**, одноразовый токен, печать IP. Без inbound до Apply.
3. Админка → New node: имя, IP, порт, токен, hostname (если Hy2/TT), пресет, группы.
4. Конфликт портов или Hy2/TT без hostname → Save нельзя.
5. Enroll: панель → `http(s)://IP:19400` с токеном → health. Позже mTLS, токен сгорает.
6. Apply пушит desired state. Пустые группы = никто не видит ноду.
7. Снятие группы/семьи **удаляет** учётки на ноде, не только прячет URL.
8. Dev: с карточки ноды выполнить команду на VPS (см. выше).

---

## Модель

- **Group:** `protocols[]`, квота/срок по умолчанию, список нод.
- **VpnUser:** uuid, hy2-пароль, tt user/pass, трафик, expire, `sub_token`.
- **Node:** IP/hostname, families, порты, status, токен/mTLS, группы.

Строка в подписке = нода умеет семью **и** группа на ноде **и** группе семья разрешена.

---

## Пресеты портов

TT не может сидеть на 443 вместе с REALITY+Hy2 (ему нужны и TCP, и UDP 443).

| Пресет | VLESS | Hy2 | TT |
|--------|-------|-----|-----|
| **max stack** (дефолт) | 443/tcp | 443/udp | 8443/tcp+udp |
| **TT-first** | 8443/tcp | 8443/udp | 443/tcp+udp |
| **stealth / hy2 / tt** | одна семья на 443 | | |

REALITY dest/SNI — plane-wide профиль, не hostname ноды. Hostname обязателен для ACME Hy2/TT.

---

## Дерево

```
cmd/plane/          # API
cmd/agent/          # нода
internal/           # store, sub renderer, HTTP
admin/              # Vite React
tools/install_vpn_node.mjs
testdata/subscriptions/
plan.md
```

### API v1

Admin: `/v1/auth/login`, `/v1/groups`, `/v1/users`, `/v1/users/:id/revoke`, `/v1/nodes`, `…/enroll`, `…/stack`, `…/groups`, `…/apply`, `…/health`, `…/exec`, preview тела.

Публично: `GET /sub/{token}`.

Agent: `/v1/health`, `/v1/exec` (dev), позже `PUT /v1/desired`.

### Экраны v1

Login; Nodes; enroll; stack+Apply log; **exec на ноде (dev)**; Groups; Users (URL + `goodwin://import?url=`); Preview сырого тела.

Не v1: биллинг, бот, HWID, auto-scale, pull-агент.

---

## Фазы

| Фаза | Суть | Готово когда |
|------|------|----------------|
| **P0** | Каркас + золотые тела | `parseSubscriptionDocument` принимает `/sub/{token}` |
| **P1** | Agent enroll + exec | Токен из инсталлятора, health и команда с админки на VPS |
| **P2** | VLESS REALITY на одной VM | Connect в `app/` с preview-ссылки, IP = нода |
| **P3** | Users + Groups + URL | Refresh в Servers подтягивает имя/disable |
| **P4** | Две ноды, разные группы | Разные наборы ссылок; мёртвая нода не в теле |
| **P5** | Hy2 на 443/udp рядом с REALITY | Connect hy2, SNI = SAN |
| **P6** | TT на 8443, deeplink из endpoint | Три строки в одной подписке, форма ловит конфликт 443 |
| **P7** | Квоты, expire, revoke, аудит Apply | userinfo + 404/пусто после revoke |

Не начинать Hy2/TT, пока P2–P3 не коннектятся с телефона по подписке.

---

## Совместимость с `app/`

Релиз «можно давать URL» только если: парсер клиента зелёный; каждая ссылка — `ShareLinkParser`; нет ss/Clash/obfs/xhttp; HTTPS ≤ 1 MiB; ручной Connect каждой включённой семьи.

Менять `app/` в v1 не нужно. Расширение контракта — сначала тест в клиентском репо, потом plane.
