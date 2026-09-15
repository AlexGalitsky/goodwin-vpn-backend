# Жёсткий аудит архитектуры — Goodwin VPN

**Дата:** 2026-09-11 (вечер)  
**Область:** plane (`VPN/`), agent, admin; стык с `goodwin-vpn-client` `app/`.  
**Не область:** `dozvoleno/`, Marzban-фичи из `llm-chat.md`.  
**Тон:** можно ли отдать второму оператору и чужому пользователю, не «Connect с твоего телефона живой».

Интерактивная сводка: canvas `architecture-audit.canvas.tsx`. Бэклог фич: [`roadmap.md`](../roadmap.md). Фазы P0–P8: [`plan.md`](../plan.md).

---

## Вердикт

**Архитектуру не переписывать.** Saturn (plane + Postgres + Caddy) / titan+mimas (agent + официальные ядра) / клиент только `GET /sub` — правильная схема. Nest, Sing-Box «всё в одном», панель на exit-ноде — это откат.

Это **рабочий lab на две ноды**, не продукт. Дыры не в выборе стека, а в том, что админка врёт про Disable, квоты не считаются, Hy2/TT могут пережить revoke последнего пользователя, а `:19400` торчит HTTP с root-exec.

Рефакторинг `server.go` / `App.tsx` **не первый шаг**. Сначала закрыть ложь Apply/`/sub`, потом тесты на этот путь, потом пилить файлы.

Оценка «отдать чужим»: **нет** — своим двум нодам при firewall на 19400 — **да, с оговорками ниже**.

| Слой | Оценка | Одной строкой |
|------|--------|----------------|
| Схема plane/agent/`desired` | **B** | Это и есть дизайн. Не трогать контракт. |
| Apply / entitlement | **C** | Disable и пустой Hy2/TT расходятся с `/sub`. |
| Секреты и периметр | **D+** | Токен и REALITY private в plaintext; CORS `*`; exec на 0.0.0.0. |
| Тесты критического пути | **F** | API-тест — статика админки. Revoke→Apply→`/sub` нет. |
| Admin UI | **C** | После P8 пользоваться можно. Один файл, 0 тестов. |
| Клиент `app/` | **C+ / B−** | Подписка и TUN живые. KS и Linux — не VPN-продукт. |

---

## Что оставить как есть

Не «рефакторить ради чистоты»:

1. `desired.State` / `PUT /v1/desired` / `ApplyResult`.
2. `/sub/{token}`: неизвестный токен → **404**; disable/expire/quota/нет нод → **пустое 200** + userinfo. Клиент Refresh завязан на это.
3. Формы ссылок: `vless://` gRPC REALITY, `hysteria2://` без obfs, `tt://?` с endpoint.
4. Панель и exit на разных IP. REALITY dest ≠ hostname ноды.
5. Официальные бинарники, не форки. Не Clash, не `ss://`.
6. Exec включён — твоё решение; закрыть нужно **сеть до агента**, не кнопку в Tools.

---

## Надо ли рефакторить

| Движение | Делать? | Когда |
|----------|---------|--------|
| Вынести `applyStoredNode` / `renderUser` из `server.go` (1566 строк) | Да, позже | После интеграционного теста fake-agent + `/sub`. |
| Разрезать `App.tsx` (1191) по вкладкам | По дороге | Вместе со следующим UI, не отдельным спринтом. |
| Per-node REALITY ключи | Нет сейчас | Одна утечка = весь флот. Имеет смысл на 3-й регион или после инцидента. |
| mTLS plane↔agent | Если 19400 нельзя закрыть firewall | Не вместо firewall. |
| Очередь Apply / mutex на ноду | Да, дёшево | Пока два админа не жмут Apply вместе — P1. |
| Переезд на k8s / Redis / Nest | Нет | Две VPS. |
| Слить три ядра в Sing-Box | Нет | Клиент — три рантайма (два Go c-shared + TT). |

---

## P0 — исправить, это ложь продукта

### 1. Disable не снимает доступ с ноды

`PATCH /v1/users/:id` пишет `disabled` в Postgres и **не** вызывает Apply. `/sub` пустеет → Refresh в приложении чистит список. UUID/пароль на Xray/Hy2/TT остаются, пока кто-то не нажмёт Apply (или revoke/delete).

Expire и over-quota — тот же класс: entitlement только на `/sub`.

**Сделать:** тот же `applyGroupNodes`, что на revoke. Иначе в UI написать «скрыт из подписки, туннель жив до Apply».

**Сделано 2026-09-11:** `PATCH` по status/expire/quota вызывает `applyGroupNodes`.

### 2. Последний пользователь Hy2/TT может остаться на ноде

`allowEmpty` чистит только VLESS. Hy2/TT в desired попадают лишь при `len(users) > 0`. Агент, не получив `hy2`/`tt`, **не переписывает** эти сервисы. Revoke/delete последнего юзера может вернуть 400 `no vless/hy2/tt users` и ничего не тронуть.

**Сделать:** пустой desired останавливает или перезаписывает Hy2/TT нулевым списком (agent сегодня требует users — это тоже надо менять). Это важнее Settings dest/SNI из roadmap 1.1.

**Сделано 2026-09-11:** Apply всегда шлёт wanted-семьи, в том числе `users: []`. Agent останавливает Hy2/TT без сертификата. `PUT {}` по-прежнему 400.

### 3. Квоты — бутафория

`upload`/`download` админ вводит руками. Ядра не репортят. `Entitled` сравнивает сумму с `total`, которая никогда не растёт.

**Сделать:** statsquery Xray (roadmap 2.2). До этого в UI не показывать «израсходовано», либо явно «вручную».

### 4. Агент `0.0.0.0:19400` HTTP + Bearer, exec по умолчанию

Apply тащит REALITY **private** key и пароли. `POST /v1/exec` — `/bin/sh -c` от root. `agent.json` пишется без `0600` (у plane env — `0600`).

Exec **не выключаем**. Закрываем порт: только IP Saturn (или WG). `chmod 0600` на `agent.json`. Сравнение токена — constant-time.

**Частично 2026-09-11:** `agent.json` chmod `0600`; сравнение токена — constant-time. Firewall `:19400` — ops, не код.

### 5. CORS: любой `Origin` + `credentials`

`internal/api/server.go` `cors()` отражает Origin. Админка шлёт cookie. Для same-origin за Caddy это лишнее; для украденной сессии в браузере — дыра.

**Сделать:** allowlist = `PUBLIC_SUB_BASE`.

**Сделано 2026-09-11:** allowlist = origin `PUBLIC_SUB_BASE` + Vite `http://127.0.0.1:5173` / `http://localhost:5173`. Без Origin CORS-заголовков нет (Flutter `/sub`).

---

## P1 — долг, не смена архитектуры

- `/sub` на каждый запрос пробят все ноды группы, таймаут 2 с, без кэша. На 10 нодах подписки будут «мигать».
- Один REALITY private на все exit.
- Нет mutex на Apply одной ноды.
- Пароль админки один, в env, без rate limit. Для lab нормально.
- `SEED_DEV=0` на Saturn (инсталлятор). Локально дефолт `1` — не путать.
- `audit_events.node_id` без FK.
- Тесты: `Entitled`, golden `/sub` body, conf builders — да. Enroll/Apply/revoke как система — нет. `server_test.go` отдаёт `index.html`.

---

## Клиент (перепроверка P0 от утра)

Не дублировать `docs/audit-prod-2026-09-11.md` целиком. Что **осталось правдой**:

| Было P0 | Сейчас |
|---------|--------|
| Секреты в SharedPreferences | Смягчено: AES-GCM + keystore; fallback в plaintext если keystore умер. |
| Debug-подпись release | Закрыто (`key.properties`). |
| `kHysteriaSniOverrides` | Убран из кода (дока может врать). |
| Home «Protected» на Linux | Копирайт поправлен на Local SOCKS. TUN на Linux по-прежнему нет. |
| IPv6 Apple/Windows | В коде появился default IPv6 route; поле не проверяли. |
| Kill Switch | Всё ещё системный Always-on + `START_NOT_STICKY`. TT KS выключен навсегда. |
| Revoke → 404 | Клиент **не** чистит ноды. Это контракт, не баг парсера. Каталог врёт, пока не re-import. |

Xray↔Hy2 = restart процесса: два Go c-shared в одном процессе. Это не рефакторинг UI.

Клиентский рефакторинг «под продукт» — отдельный репо и не блокирует plane, кроме одного продукта: **Disable должен совпадать с тем, что происходит на ноде**.

---

## Что добавить / исправить — порядок (честный, не wishlist)

Вставить **перед** roadmap 1.1, если выбирать одну ветку:

0. **Empty Apply чистит Hy2/TT + disable/expire вызывают Apply** — иначе P7/P8 врут.  
1. CORS на `PUBLIC_SUB_BASE`, `chmod 0600` agent.json, firewall 19400 → Saturn.  
2. Дальше [`roadmap.md`](../roadmap.md): dest/SNI форма, torrent/SMTP, health+сертификат, живой трафик Xray.

Не начинать split файлов, WARP, DNS-01, биллинг, Clash.

---

## Архитектура одной схемой

```
телефон  --HTTPS /sub/{token}-->  Caddy:443 Saturn  --> plane:8080
админ    --cookie same-origin-->  Caddy:443 Saturn  --> plane:8080 --> Postgres

plane --HTTP Bearer--> titan:19400 agent --> systemd: xray, hy2, tt
                   --> mimas:19400 agent --> ...
```

Слабое звено — средняя стрелка (plaintext desired + exec). Верхние две — нормальные. Нижние ядра — официальные, так и должно быть.

---

## Итог одной фразой

Схему не менять. Не пилить `server.go` ради эстетики. Починить Apply так, чтобы Disable/revoke/пустая группа совпадали с тем, что слушает 443/8443, закрыть 19400 и CORS — потом roadmap, не наоборот.
