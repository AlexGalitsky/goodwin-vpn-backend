# Goodwin VPN Protocol — плоскость

Канон поведения клиента: [goodwin-vpn-client `docs/goodwin-protocol.md`](https://github.com/AlexGalitsky/goodwin-vpn-client/blob/main/docs/goodwin-protocol.md).

Этот файл — что **отдаёт plane** (Saturn = reference). Тело `/sub` и правило **404 ≠ пустое 200** не менять.

---

## Что включено

На https `PUBLIC_SUB_BASE` (например `https://saturn.goodwin.website`):

| Метод | Auth | Ответ |
|--------|------|--------|
| `GET /sub/{token}` | token | share-ссылки; на **200** заголовок `Goodwin-VPN` |
| `GET /privacy` | нет | HTML политики |
| `GET /gw/v1/service` | нет | каталог + `features` |
| `GET /gw/v1/geo/manifest` | нет | паки |
| `GET /gw/v1/geo/packs/{id}` | нет | JSON пака; неизвестный id → **404** |

Локально `PUBLIC_SUB_BASE=http://127.0.0.1:8080` — заголовка нет, `privacy` в каталоге пустой (клиент http `/sub` не импортирует).

Нет в v1: `GET /gw/v1/profile/{token}`, `json-profile`, `/.well-known/goodwin-vpn`.

---

## Заголовок

Только если `PUBLIC_SUB_BASE` — https origin:

```http
Goodwin-VPN: v1; base="https://saturn.goodwin.website"
```

Код: `sub.ServiceHeader` / `PublicHTTPSOrigin` в `internal/sub/render.go`. Ставится в `subscription` после успешного render, не на 404.

Клиент требует same-origin с URL подписки.

---

## Каталог

`GET /gw/v1/service` → `sub.ServiceDocumentFor`:

```json
{
  "protocol": "goodwin-vpn",
  "version": 1,
  "name": "Goodwin VPN",
  "privacy": "https://saturn.goodwin.website/privacy",
  "features": ["geo-packs"]
}
```

`features` содержит `geo-packs`, когда скомпилирован пак `ads`. Иначе `[]`.

---

## Geo packs

Источник (не GitHub geosite): `internal/geo/allowlist/`

| Файл | Роль |
|------|------|
| `catalog.json` | `"version": "YYYY.MM.DD"` |
| `ads.json` | `domains[]`, `suffixes[]`, `cidrs[]` — человеческий список, без ведущей точки |

При старте plane компилирует канонический JSON (`id`, отсортированные поля, suffixes с `.`), считает sha256 **этих байт**, отдаёт его как тело пака.

Лимиты: 512 KiB, 2000 записей, id `[a-z][a-z0-9-]{0,31}`.

Проверка без деплоя:

```bash
node tools/build_geo_packs.mjs --check
```

Добавить суффикс рекламы: строка в `ads.json` → `--check` → коммит → bootstrap панели. Клиенты подтянут новый sha256 при следующем prefetch (после Refresh `/sub`). Не класть туда пользовательские исключения вроде `myip.ru` — это direct в приложении, не ads.

---

## `/sub` и TrustTunnel

VLESS/Hy2 ссылки **синтезируются** из UUID/пароля + last Apply. `tt://` **чеканится на ноде** (`trusttunnel_endpoint --format deeplink`) и лежит в `tt_links`.

Disable / revoke / delete уже делают `applyGroupNodes`. Создание пользователя без Apply даёт в `/sub` VLESS/Hy2 (синтез) и **пустой TT** (нет строки в `tt_links`). После деплоя фикса create тоже Apply; до него — кнопка Apply на titan/mimas.

---

## Проверка Saturn

```bash
curl -sI https://saturn.goodwin.website/sub/<token> | grep -i goodwin-vpn
# Goodwin-VPN: v1; base="https://saturn.goodwin.website"

curl -s https://saturn.goodwin.website/gw/v1/service
curl -s https://saturn.goodwin.website/gw/v1/geo/manifest
curl -s https://saturn.goodwin.website/gw/v1/geo/packs/ads | wc -c
curl -s -o /dev/null -w '%{http_code}\n' https://saturn.goodwin.website/gw/v1/geo/packs/nope
# 404

curl -s -o /dev/null -w '%{http_code}\n' https://saturn.goodwin.website/sub/does-not-exist
# 404
```

Живой токен: 200, share-ссылки, заголовок. Disable/квота: **пустое 200**, не 404.

Ноды (titan/mimas) для G0–G3 не обновлять.

---

## Совместимость

Старые клиенты без G1 заголовок игнорируют. Новые без `geo-packs` остаются на tiny ads list. Чужой `/sub` без заголовка не должен стать хуже.
