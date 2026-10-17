package store

import (
	"context"
)

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "Pocket Travel"

// GetSettings loads all settings with defaults applied.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := Settings{Title: DefaultTitle, HomeCurrency: "EUR", DateFormat: "auto", TimeFormat: "auto", DistanceUnit: "km", FXAuto: true, MapTiles: "osm"}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		switch k {
		case "title":
			if v != "" {
				st.Title = v
			}
		case "home_currency":
			if v != "" {
				st.HomeCurrency = v
			}
		case "date_format":
			if v != "" {
				st.DateFormat = v
			}
		case "time_format":
			if v != "" {
				st.TimeFormat = v
			}
		case "distance_unit":
			if v == "mi" {
				st.DistanceUnit = "mi"
			}
		case "fx_auto":
			st.FXAuto = v != "0"
		case "firefly_url":
			st.FireflyURL = v
		case "firefly_token":
			st.FireflyToken = v
		case "dawarich_url":
			st.DawarichURL = v
		case "dawarich_api_key":
			st.DawarichAPIKey = v
		case "immich_url":
			st.ImmichURL = v
		case "immich_api_key":
			st.ImmichAPIKey = v
		case "immich_public_url":
			st.ImmichPublicURL = v
		case "map_tiles":
			if v != "" {
				st.MapTiles = v
			}
		case "setup_complete":
			st.SetupComplete = v == "1"
		}
	}
	return st, rows.Err()
}

// SaveSettings persists all settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	b := map[bool]string{true: "1", false: "0"}
	kv := map[string]string{
		"title":             st.Title,
		"home_currency":     st.HomeCurrency,
		"date_format":       st.DateFormat,
		"time_format":       st.TimeFormat,
		"distance_unit":     st.DistanceUnit,
		"fx_auto":           b[st.FXAuto],
		"firefly_url":       st.FireflyURL,
		"firefly_token":     st.FireflyToken,
		"dawarich_url":      st.DawarichURL,
		"dawarich_api_key":  st.DawarichAPIKey,
		"immich_url":        st.ImmichURL,
		"immich_api_key":    st.ImmichAPIKey,
		"immich_public_url": st.ImmichPublicURL,
		"map_tiles":         st.MapTiles,
		"setup_complete":    b[st.SetupComplete],
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---- generic cache ----

// CacheGet returns a cached value and when it was fetched.
func (s *Store) CacheGet(ctx context.Context, key string) (string, string, bool) {
	var v, at string
	if err := s.DB.QueryRowContext(ctx, `SELECT value, fetched_at FROM cache WHERE key = ?`, key).Scan(&v, &at); err != nil {
		return "", "", false
	}
	return v, at, true
}

// CachePut stores a value.
func (s *Store) CachePut(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO cache (key, value, fetched_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, fetched_at = excluded.fetched_at`, key, value, Now())
	return err
}

// CacheDelete removes every key with the prefix.
func (s *Store) CacheDelete(ctx context.Context, prefix string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM cache WHERE key LIKE ? || '%'`, prefix)
	return err
}

// ---- fx cache ----

// GetFXRate returns a cached rate.
func (s *Store) GetFXRate(ctx context.Context, base, quote, date string) (float64, string, bool) {
	var r float64
	var src string
	if err := s.DB.QueryRowContext(ctx, `SELECT rate, source FROM fx_cache WHERE base = ? AND quote = ? AND date = ?`, base, quote, date).Scan(&r, &src); err != nil {
		return 0, "", false
	}
	return r, src, true
}

// PutFXRate caches a rate.
func (s *Store) PutFXRate(ctx context.Context, base, quote, date string, rate float64, source string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO fx_cache (base, quote, date, rate, source) VALUES (?, ?, ?, ?, ?) ON CONFLICT(base, quote, date) DO UPDATE SET rate = excluded.rate, source = excluded.source`, base, quote, date, rate, source)
	return err
}
