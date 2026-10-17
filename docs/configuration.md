# Configuration

Everything is an environment variable with a sensible default. Integration settings saved in the UI (Settings → Integrations) take precedence over the environment once set.

| Variable | Default | Meaning |
|---|---|---|
| `HOST` / `PORT` | `0.0.0.0` / `8080` | Listen address |
| `BASE_URL` | | Public URL, used in calendar exports |
| `DATA_DIR` | `./data` | SQLite file, uploaded files, backups |
| `DB_PATH` | `$DATA_DIR/pocket-travel.db` | |
| `FILES_DIR` | `$DATA_DIR/files` | Documents, covers, place images |
| `MAX_UPLOAD_BYTES` | `52428800` | Largest upload (50 MB) |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | `debug`, `warn`, `error`; `json` |
| `TRUST_PROXY` | `true` | Use `X-Forwarded-For` |
| `METRICS_ENABLED` | `true` | `/metrics` for Prometheus |
| `AUTH_MODE` | `local` | `local` (email + password), `proxy` (trust a header), `none` |
| `AUTH_PROXY_HEADER` / `AUTH_PROXY_EMAIL_HEADER` | `Remote-User` / `Remote-Email` | For `proxy` mode |
| `SESSION_TTL` | `720h` | |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | | First admin, created only when no users exist |
| `DEFAULT_CURRENCY` | `EUR` | Home currency until changed in Settings |
| `FX_PROVIDER` | `auto` | `auto`, `frankfurter`, `erapi`, `none` |
| `FIREFLY_URL` / `FIREFLY_TOKEN` | | Firefly III base URL + personal access token |
| `DAWARICH_URL` / `DAWARICH_API_KEY` | | Dawarich base URL + API key |
| `IMMICH_URL` / `IMMICH_API_KEY` | | Immich base URL + API key |
| `IMMICH_PUBLIC_URL` | | Browser-facing Immich URL for "Open in Immich" links when `IMMICH_URL` is internal |
| `USER_AGENT` | `PocketTravel/1.0 (+repo)` | Sent to Nominatim, Overpass, Open-Meteo, Wikipedia |
| `DEMO_DATA` | `false` | Seed three example trips on first start |
| `TZ` | | Container timezone (day boundaries for Dawarich and "today") |

## Integrations

- **Firefly III** — create a personal access token under Options → Profile → OAuth. Tag the withdrawals of a trip (e.g. `lisbon 2027`) and set the same tag on the trip. Every withdrawal with the tag is synced as a *spent* expense; the user may re-categorise them locally, everything else follows Firefly. Sync runs every 30 minutes for running and upcoming trips, on every trip save, and on demand (Budget → Sync).
- **Dawarich** — Account → API key. The trip's date range (in the trip's timezone, or the server's) is fetched as slim points, simplified for drawing, and summed per day. The timeline endpoint supplies journeys and stops. Cached for an hour while the trip runs, a week after.
- **Immich** — Account settings → API keys. Photos are searched by `takenAfter`/`takenBefore`; thumbnails are proxied through the app so the key stays on the server.

Inside Docker, reach services on the same host with `http://host.docker.internal:<port>` (the compose file adds the `host-gateway` alias). Services that force HTTPS redirects are fine too — the Dawarich client sends `X-Forwarded-Proto: https`.
