CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'admin',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- A trip is the unit everything hangs off: dates, a home currency and budget,
-- and the switches that connect it to Firefly (a tag), Dawarich and Immich
-- (the date range). Members see the trip; the owner and editors change it.
CREATE TABLE trips (
  id           INTEGER PRIMARY KEY,
  owner_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  destination  TEXT NOT NULL DEFAULT '',
  description  TEXT NOT NULL DEFAULT '',
  start_date   TEXT NOT NULL DEFAULT '',
  end_date     TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'planning',   -- planning | booked | active | completed | cancelled
  currency     TEXT NOT NULL DEFAULT 'EUR',
  budget       INTEGER NOT NULL DEFAULT 0,          -- minor units of currency, 0 = none
  color        TEXT NOT NULL DEFAULT '',
  cover_path   TEXT NOT NULL DEFAULT '',
  link         TEXT NOT NULL DEFAULT '',
  notes        TEXT NOT NULL DEFAULT '',
  lat          REAL,
  lng          REAL,
  timezone     TEXT NOT NULL DEFAULT '',
  firefly_tag  TEXT NOT NULL DEFAULT '',
  dawarich     INTEGER NOT NULL DEFAULT 1,
  immich       INTEGER NOT NULL DEFAULT 1,
  archived     INTEGER NOT NULL DEFAULT 0,
  external_id  TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX trips_dates ON trips(start_date, end_date);

CREATE TABLE trip_members (
  trip_id    INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT 'editor',          -- owner | editor | viewer
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (trip_id, user_id)
);

-- Places: everything you might go to. A place with a day becomes part of the
-- itinerary; without one it sits in the ideas list.
CREATE TABLE places (
  id           INTEGER PRIMARY KEY,
  trip_id      INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  category     TEXT NOT NULL DEFAULT 'sight',
  description  TEXT NOT NULL DEFAULT '',
  address      TEXT NOT NULL DEFAULT '',
  lat          REAL,
  lng          REAL,
  link         TEXT NOT NULL DEFAULT '',
  phone        TEXT NOT NULL DEFAULT '',
  rating       REAL,
  price        INTEGER NOT NULL DEFAULT 0,
  currency     TEXT NOT NULL DEFAULT '',
  day_date     TEXT NOT NULL DEFAULT '',             -- YYYY-MM-DD, '' = unscheduled
  start_time   TEXT NOT NULL DEFAULT '',             -- HH:MM
  duration_min INTEGER NOT NULL DEFAULT 0,
  status       TEXT NOT NULL DEFAULT 'idea',         -- idea | planned | booked | visited | skipped
  visited_at   TEXT NOT NULL DEFAULT '',
  booking_ref  TEXT NOT NULL DEFAULT '',
  tags         TEXT NOT NULL DEFAULT '',
  notes        TEXT NOT NULL DEFAULT '',
  image_path   TEXT NOT NULL DEFAULT '',
  sort_order   INTEGER NOT NULL DEFAULT 0,
  external_id  TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX places_trip ON places(trip_id, day_date, sort_order);

CREATE TABLE lodging (
  id                 INTEGER PRIMARY KEY,
  trip_id            INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  name               TEXT NOT NULL,
  type               TEXT NOT NULL DEFAULT 'hotel',  -- hotel | apartment | hostel | house | camping | friends | other
  address            TEXT NOT NULL DEFAULT '',
  lat                REAL,
  lng                REAL,
  check_in           TEXT NOT NULL DEFAULT '',       -- RFC3339 or YYYY-MM-DD
  check_out          TEXT NOT NULL DEFAULT '',
  reservation_number TEXT NOT NULL DEFAULT '',
  link               TEXT NOT NULL DEFAULT '',
  phone              TEXT NOT NULL DEFAULT '',
  price              INTEGER NOT NULL DEFAULT 0,
  currency           TEXT NOT NULL DEFAULT '',
  paid               INTEGER NOT NULL DEFAULT 0,
  rating             REAL,
  notes              TEXT NOT NULL DEFAULT '',
  image_path         TEXT NOT NULL DEFAULT '',
  external_id        TEXT NOT NULL DEFAULT '',
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX lodging_trip ON lodging(trip_id, check_in);

CREATE TABLE transport (
  id          INTEGER PRIMARY KEY,
  trip_id     INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  type        TEXT NOT NULL DEFAULT 'flight',       -- flight | train | bus | car | ferry | taxi | bike | walk | other
  name        TEXT NOT NULL DEFAULT '',
  carrier     TEXT NOT NULL DEFAULT '',
  number      TEXT NOT NULL DEFAULT '',             -- flight / train number
  from_name   TEXT NOT NULL DEFAULT '',
  from_lat    REAL,
  from_lng    REAL,
  to_name     TEXT NOT NULL DEFAULT '',
  to_lat      REAL,
  to_lng      REAL,
  depart_at   TEXT NOT NULL DEFAULT '',             -- local wall time, 2026-08-28T07:30
  arrive_at   TEXT NOT NULL DEFAULT '',
  booking_ref TEXT NOT NULL DEFAULT '',
  seat        TEXT NOT NULL DEFAULT '',
  link        TEXT NOT NULL DEFAULT '',
  price       INTEGER NOT NULL DEFAULT 0,
  currency    TEXT NOT NULL DEFAULT '',
  paid        INTEGER NOT NULL DEFAULT 0,
  distance_km REAL NOT NULL DEFAULT 0,
  notes       TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX transport_trip ON transport(trip_id, depart_at);

-- Expenses: planned costs and what was actually spent. Firefly rows are
-- synced in with source='firefly' and keyed by the transaction id so a
-- re-sync updates instead of duplicating. amount_home is the amount in the
-- trip currency at fx_rate.
CREATE TABLE expenses (
  id          INTEGER PRIMARY KEY,
  trip_id     INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  category    TEXT NOT NULL DEFAULT 'other',
  amount      INTEGER NOT NULL DEFAULT 0,
  currency    TEXT NOT NULL DEFAULT 'EUR',
  amount_home INTEGER NOT NULL DEFAULT 0,
  fx_rate     REAL NOT NULL DEFAULT 1,
  date        TEXT NOT NULL DEFAULT '',
  kind        TEXT NOT NULL DEFAULT 'planned',      -- planned | actual
  source      TEXT NOT NULL DEFAULT 'manual',       -- manual | firefly | item
  external_id TEXT NOT NULL DEFAULT '',
  link_kind   TEXT NOT NULL DEFAULT '',             -- place | lodging | transport
  link_id     INTEGER NOT NULL DEFAULT 0,
  notes       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX expenses_external ON expenses(trip_id, source, external_id) WHERE external_id <> '';
CREATE INDEX expenses_trip ON expenses(trip_id, date);

CREATE TABLE checklists (
  id         INTEGER PRIMARY KEY,
  trip_id    INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'todo',           -- packing | todo | shopping | custom
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE checklist_items (
  id           INTEGER PRIMARY KEY,
  checklist_id INTEGER NOT NULL REFERENCES checklists(id) ON DELETE CASCADE,
  text         TEXT NOT NULL,
  done         INTEGER NOT NULL DEFAULT 0,
  assignee_id  INTEGER,
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX checklist_items_list ON checklist_items(checklist_id, sort_order);

CREATE TABLE journal (
  id         INTEGER PRIMARY KEY,
  trip_id    INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date       TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  mood       TEXT NOT NULL DEFAULT '',
  lat        REAL,
  lng        REAL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX journal_trip ON journal(trip_id, date);

-- Documents: tickets, bookings, insurance, passports… stored as files under
-- FILES_DIR/<trip>/ and optionally linked to one item.
CREATE TABLE documents (
  id         INTEGER PRIMARY KEY,
  trip_id    INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  filename   TEXT NOT NULL,
  mime       TEXT NOT NULL DEFAULT '',
  bytes      INTEGER NOT NULL DEFAULT 0,
  kind       TEXT NOT NULL DEFAULT 'other',          -- ticket | booking | insurance | id | map | other
  link_kind  TEXT NOT NULL DEFAULT '',
  link_id    INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX documents_trip ON documents(trip_id);

CREATE TABLE activity (
  id         INTEGER PRIMARY KEY,
  trip_id    INTEGER NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  user_id    INTEGER,
  action     TEXT NOT NULL,
  subject    TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX activity_trip ON activity(trip_id, id DESC);

-- Cached answers from Dawarich, Immich, Firefly, Open-Meteo… keyed by what was asked.
CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  fetched_at TEXT NOT NULL
);

CREATE TABLE fx_cache (
  base   TEXT NOT NULL,
  quote  TEXT NOT NULL,
  date   TEXT NOT NULL,
  rate   REAL NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (base, quote, date)
);
