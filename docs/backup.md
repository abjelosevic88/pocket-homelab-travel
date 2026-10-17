# Backup

Two things to keep:

1. **The database** `DATA_DIR/pocket-travel.db` (WAL mode — never copy the file while the app runs; use one of these instead):
   - `pocket-travel backup` (or `docker exec pocket-travel pocket-travel backup`) writes a consistent copy via `VACUUM INTO` to `DATA_DIR/backups/`.
   - `GET /api/v1/backup` downloads the same thing (Settings → Backup & data).
2. **Uploaded files** `DATA_DIR/files/` — documents, trip covers, place images. Plain files, copy them as they are.

Restore: stop the app, replace `pocket-travel.db` (remove any `-wal`/`-shm` next to it), put `files/` back, start.

The `cache` and `fx_cache` tables are rebuilt on demand; Firefly rows re-sync from Firefly.
