package store

import (
	"context"
	"fmt"
	"strings"
)

// SplitCodes turns "PT,ES" into [PT ES] (upper case, no blanks or repeats).
func SplitCodes(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, c := range strings.Split(s, ",") {
		c = strings.ToUpper(strings.TrimSpace(c))
		if len(c) == 2 && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// JoinCodes is the inverse of SplitCodes.
func JoinCodes(codes []string) string {
	return strings.Join(SplitCodes(strings.Join(codes, ",")), ",")
}

// Located is a row with coordinates whose country is not known yet.
type Located struct {
	Table string // places | lodging | trips
	ID    int64
	Lat   float64
	Lng   float64
}

// MissingCountries returns up to limit located rows without a country code,
// stays and trips first (they decide the days per country).
func (s *Store) MissingCountries(ctx context.Context, limit int) ([]Located, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT 'lodging', id, lat, lng FROM lodging WHERE country_code = '' AND lat IS NOT NULL AND lng IS NOT NULL
		UNION ALL SELECT 'trips', id, lat, lng FROM trips WHERE country_code = '' AND lat IS NOT NULL AND lng IS NOT NULL
		UNION ALL SELECT 'places', id, lat, lng FROM places WHERE country_code = '' AND lat IS NOT NULL AND lng IS NOT NULL
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Located{}
	for rows.Next() {
		var l Located
		if err := rows.Scan(&l.Table, &l.ID, &l.Lat, &l.Lng); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetCountryCode stores the country of a row, unless its coordinates moved
// since the lookup started ("-" marks a spot that is in no country, e.g. at sea).
func (s *Store) SetCountryCode(ctx context.Context, l Located, code string) error {
	switch l.Table {
	case "places", "lodging", "trips":
	default:
		return fmt.Errorf("no country for %s", l.Table)
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE `+l.Table+` SET country_code = ? WHERE id = ? AND lat = ? AND lng = ?`, code, l.ID, l.Lat, l.Lng)
	return err
}
