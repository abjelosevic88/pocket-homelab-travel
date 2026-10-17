-- Several photos per place. Uploads and imports keep working through
-- places.image_path, which now mirrors the first photo.
CREATE TABLE place_photos (
  id         INTEGER PRIMARY KEY,
  place_id   INTEGER NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  path       TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT 'upload',      -- upload | import | wikipedia | commons
  credit     TEXT NOT NULL DEFAULT '',
  license    TEXT NOT NULL DEFAULT '',
  source_url TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX place_photos_place ON place_photos(place_id, sort_order, id);
INSERT INTO place_photos (place_id, path, source, sort_order)
  SELECT id, image_path, CASE WHEN external_id <> '' THEN 'import' ELSE 'upload' END, 0 FROM places WHERE image_path <> '';
