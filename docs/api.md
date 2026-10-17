# API

All endpoints live under `/api/v1`. Authenticate with the session cookie (the UI) or a token from Settings → API tokens: `Authorization: Bearer pt_…` (or `X-API-Key`). Amounts are integers in minor units (`1250` = 12.50 EUR, `1500` = ¥1500).

## Trips

| Method | Path | |
|---|---|---|
| GET | `/trips?archived=0&q=` | Trips the caller can see (`all=1` for admins) with counts |
| POST | `/trips` | Create: `name`, `destination`, `start_date`, `end_date`, `currency`, `budget`, `firefly_tag`, … |
| GET/PUT/DELETE | `/trips/{id}` | |
| GET | `/trips/{id}/overview` | Everything: trip, places, lodging, transport, expenses, checklists, journal, documents, budget, integrations |
| PUT | `/trips/{id}/members` | `{members:[{user_id, role}]}` (owner only) |
| POST | `/trips/{id}/cover` | multipart `file` |
| POST | `/trips/{id}/duplicate` | `{name, start_date}` |
| POST | `/trips/{id}/refresh` | Drop caches, re-sync Firefly |

## Items

`/trips/{id}/places`, `/lodging`, `/transport`, `/expenses`, `/journal`, `/documents` — `GET` list, `POST` create, `PUT /{itemId}` update (only sent fields change), `DELETE /{itemId}`.

- `PUT /trips/{id}/places/reorder` `{day:"2026-08-02", ids:[…]}` moves places onto a day in order (`day:""` = unscheduled).
- `POST /trips/{id}/places/{pid}/image`, `/lodging/{lid}/image` — multipart `file`.
- `/trips/{id}/checklists` `POST {name, kind, template, items[]}`; `POST /checklists/{cid}/items {text}` (multi-line = several items); `PUT /checklists/items/{iid} {done, text}`.
- `POST /trips/{id}/documents` — multipart `file`, `kind`, `name`.

## Money & integrations

| Method | Path | |
|---|---|---|
| GET | `/trips/{id}/budget` | Summary: planned, actual, remaining, per day, by category, by day |
| POST | `/trips/{id}/firefly/sync` | Pull tagged withdrawals now |
| GET | `/firefly/tags` | |
| GET | `/trips/{id}/dawarich/route` | Simplified polyline `points:[[lat,lng,ts]]`, `distance_km`, `days_km` |
| GET | `/trips/{id}/dawarich/timeline` | Dawarich's day summaries with journeys and visits |
| GET | `/trips/{id}/photos` | Immich assets in the date range |
| GET | `/trips/{id}/photos/{assetId}/thumb?size=thumbnail\|preview` | Proxied thumbnail |
| GET | `/trips/{id}/weather` | Daily weather for the trip dates |
| GET | `/fx?from=USD&to=EUR&date=` | Exchange rate |
| GET | `/currencies` | ISO 4217 list |
| POST | `/integrations/test` | `{which, url, key}` (admin) |

Add `?refresh=1` to the Dawarich / Immich / weather endpoints to bypass the cache.

## AI (needs an OpenAI key)

| Method | Path | |
|---|---|---|
| POST | `/trips/{id}/ai/suggest` | `{interests, near, category, count}` → `{suggestions:[{name, category, why, area, lat, lng, address, website, geocoded, duration_min, best_time, exists}]}` |
| POST | `/trips/{id}/ai/plan` | `{pace: relaxed\|\|packed, notes, reschedule}` → `{items:[{place_id, name, day_date, start_time, duration_min, note}], summary, unplaced}`; `{apply:true, items}` writes it |
| POST | `/trips/{id}/ai/describe` | `{place_id}` or `{name, address, category}` → `{description, tips, duration_min, best_time}` |
| POST | `/trips/{id}/ai/packing` | `{notes, name, create}` → groups of items; `create:true` makes the checklist |
| POST | `/trips/{id}/ai/recap` | `{style: story\|summary\|social, save}` → `{text}` |

## Outside world

`GET /geo/search?q=` (Nominatim), `GET /geo/reverse?lat&lng`, `GET /geo/discover?lat&lng&radius=3000&category=food` (Overpass), `GET /geo/wiki?q=` (Wikipedia summary), `GET /geo/weather?lat&lng&start&end` (Open-Meteo).

## Exports & imports

- `GET /trips/{id}/export/calendar.ics`, `/export/trip.gpx?route=1`, `/export/trip.json`
- `GET /cal/{id}.ics?token=pt_…` — calendar subscription URL for phones
- `POST /import/adventurelog/collections {url, username, password}` → list; `POST /import/adventurelog {…, collections:[ids], images:true}`
- `POST /import/trip` — a JSON export

## Misc

`GET /home` (home page data), `GET /dashboard` (flat numbers for a Homepage widget: `trips`, `upcoming`, `days_traveled`, `places`, `next_trip_text`), `GET /meta` (category lists), `/users`, `/settings`, `/settings/system`, `/tokens`, `GET /backup` (SQLite download), `/healthz`, `/readyz`, `/metrics`.
