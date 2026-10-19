-- Everything is per person. Trips already carry owner_id and every row under
-- a trip (places, place_photos, lodging, transport, expenses, checklists,
-- checklist_items, journal, documents, activity) is reached only through its
-- trip, so it inherits the trip's owner; trip_members stays the one explicit
-- way to share a trip. What was box-wide and personal moves to user_settings:
-- preferences and the Firefly / Dawarich / Immich / OpenAI credentials, which
-- were configured by the owner (the first admin) and stay theirs. Members
-- start with none.
CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

INSERT OR IGNORE INTO user_settings (user_id, key, value)
  SELECT (SELECT COALESCE((SELECT MIN(id) FROM users WHERE role = 'admin'), (SELECT MIN(id) FROM users))), key, value
  FROM settings
  WHERE key NOT IN ('title', 'setup_complete', 'fx_auto')
    AND EXISTS (SELECT 1 FROM users);

DELETE FROM settings WHERE key NOT IN ('title', 'setup_complete', 'fx_auto') AND EXISTS (SELECT 1 FROM users);

CREATE INDEX IF NOT EXISTS trips_owner ON trips(owner_id);
CREATE INDEX IF NOT EXISTS trip_members_user ON trip_members(user_id);

-- Route and photo caches were keyed by trip alone; they are now keyed by trip
-- and person (each person's own Dawarich track and Immich library).
DELETE FROM cache WHERE key LIKE 'trip:%:dawarich-%' OR key LIKE 'trip:%:immich-%';
