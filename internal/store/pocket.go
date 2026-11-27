package store

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// What Pocket Search, Pocket Day and Pocket Hub read (the /pocket routes in
// internal/server/capabilities.go). Every query is limited to the trips the
// person may see: their own and the ones shared with them, exactly as
// ListTrips does.

// tripVisible takes the person's id twice.
const tripVisible = `(t.owner_id = ? OR t.id IN (SELECT trip_id FROM trip_members WHERE user_id = ?))`

// tripLive leaves out what the first page leaves out: archived and cancelled trips.
const tripLive = `t.archived = 0 AND t.status <> '` + StatusCancelled + `'`

// TripsOn returns the trips the person is on that day (the day falls between
// the trip's dates), earliest start first. Archived and cancelled trips are
// left out.
func (s *Store) TripsOn(ctx context.Context, userID int64, day string) ([]Trip, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+tripCols+` FROM trips t WHERE `+tripVisible+` AND `+tripLive+`
		AND t.start_date <> '' AND t.start_date <= ? AND t.end_date >= ? ORDER BY t.start_date, t.id`, userID, userID, day, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trip{}
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// NextTrip returns the first trip that starts after the day, or ErrNotFound.
// Archived and cancelled trips are left out.
func (s *Store) NextTrip(ctx context.Context, userID int64, day string) (*Trip, error) {
	return scanTrip(s.DB.QueryRowContext(ctx, `SELECT `+tripCols+` FROM trips t WHERE `+tripVisible+` AND `+tripLive+`
		AND t.start_date > ? ORDER BY t.start_date, t.id LIMIT 1`, userID, userID, day))
}

// ---- search ----

// searchWords is the most words of a search that are looked for; the rest is ignored.
const searchWords = 8

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// SearchTerms splits what was typed into the words to look for: the ones
// with a letter or a digit in them. None means there is nothing to find.
func SearchTerms(q string) []string {
	out := []string{}
	for _, w := range strings.Fields(q) {
		if strings.IndexFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
			continue
		}
		if out = append(out, w); len(out) == searchWords {
			break
		}
	}
	return out
}

// likeWords builds one "(col LIKE ? …)" per word. SQLite's LIKE ignores case
// for a–z only, so a word with other letters (š, é, ж) is also tried in lower
// case, in capitals and capitalised.
func likeWords(col string, words []string) ([]string, []any) {
	var clauses []string
	var args []any
	for _, w := range words {
		forms := []string{w}
		if strings.IndexFunc(w, func(r rune) bool { return r >= utf8.RuneSelf }) >= 0 {
			lower := strings.ToLower(w)
			first, n := utf8.DecodeRuneInString(lower)
			for _, f := range []string{lower, strings.ToUpper(w), string(unicode.ToUpper(first)) + lower[n:]} {
				seen := false
				for _, have := range forms {
					seen = seen || have == f
				}
				if !seen {
					forms = append(forms, f)
				}
			}
		}
		parts := make([]string, 0, len(forms))
		for _, f := range forms {
			parts = append(parts, col+` LIKE ? ESCAPE '\'`)
			args = append(args, "%"+likeEscaper.Replace(f)+"%")
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}
	return clauses, args
}

// tripWhenRank orders trips for a search: the one the person is on (0), the
// ones still to come (1), the ones without dates yet (2), the past ones (3),
// then archived and cancelled trips (4). It takes the day twice.
const tripWhenRank = `CASE WHEN t.archived = 1 OR t.status = '` + StatusCancelled + `' THEN 4
	WHEN t.start_date = '' THEN 2 WHEN t.start_date > ? THEN 1 WHEN t.end_date >= ? THEN 0 ELSE 3 END`

// The words a search looks through.
const (
	tripNameText  = `(t.name || ' ' || t.destination)`
	tripText      = `(t.name || ' ' || t.destination || ' ' || t.description)`
	lodgingText   = `(l.name || ' ' || l.address || ' ' || l.reservation_number)`
	transportText = `(x.name || ' ' || x.carrier || ' ' || x.number || ' ' || x.from_name || ' ' || x.to_name || ' ' || x.booking_ref)`
	placeText     = `(p.name || ' ' || p.address)`
)

// FoundTrip is a trip a search found.
type FoundTrip struct {
	Trip
	// When is tripWhenRank: 0 on now, 1 to come, 2 no dates, 3 past, 4 archived or cancelled.
	When int
	// ByName: the words are in the name and destination (not only in the description).
	ByName bool
}

// InTrip says which trip a found stay, leg or place belongs to.
type InTrip struct {
	TripName string
	When     int
}

// FoundLodging, FoundTransport and FoundPlace are things inside trips a search found.
type (
	FoundLodging struct {
		Lodging
		InTrip
	}
	FoundTransport struct {
		Transport
		InTrip
	}
	FoundPlace struct {
		Place
		InTrip
	}
)

// scanWith reads a row's usual columns and then some more.
type scanWith struct {
	row   interface{ Scan(...any) error }
	extra []any
}

func (s scanWith) Scan(dst ...any) error { return s.row.Scan(append(dst, s.extra...)...) }

// qualify puts a table alias in front of every column of a list.
func qualify(alias, cols string) string {
	return alias + "." + strings.ReplaceAll(cols, ", ", ", "+alias+".")
}

// whenOrder puts what is on now or still to come soonest first (what has no
// date yet after what has one) and everything else newest first.
func whenOrder(date string) string {
	return `CASE WHEN w <= 1 THEN ` + date + ` = '' END, CASE WHEN w <= 1 THEN ` + date + ` END, ` + date + ` DESC`
}

// SearchTrips finds the trips the person can see that have every word in the
// name, the destination or the description: name and destination matches
// first, and within each the trip they are on, the ones to come (soonest
// first), then the past ones (newest first).
func (s *Store) SearchTrips(ctx context.Context, userID int64, words []string, today string, limit int) ([]FoundTrip, error) {
	if len(words) == 0 {
		return []FoundTrip{}, nil
	}
	byName, nameArgs := likeWords(tripNameText, words)
	all, allArgs := likeWords(tripText, words)
	args := append(append([]any{today, today}, nameArgs...), userID, userID)
	args = append(append(args, allArgs...), limit)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+tripCols+`, `+tripWhenRank+` AS w, (`+strings.Join(byName, " AND ")+`) AS n
		FROM trips t WHERE `+tripVisible+` AND `+strings.Join(all, " AND ")+`
		ORDER BY n DESC, w, `+whenOrder("t.start_date")+`, t.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FoundTrip{}
	for rows.Next() {
		var f FoundTrip
		t, err := scanTrip(scanWith{rows, []any{&f.When, &f.ByName}})
		if err != nil {
			return nil, err
		}
		f.Trip = *t
		out = append(out, f)
	}
	return out, rows.Err()
}

// searchInside builds the query for things inside trips: every word is in the
// thing's own text or its trip's name and destination, and at least one is in
// its own text (so a trip's name alone does not bring up everything in it).
func searchInside(cols, table, alias, text, date string, userID int64, words []string, today string, limit int) (string, []any) {
	own, ownArgs := likeWords(text, words)
	all, allArgs := likeWords(`(`+text+` || ' ' || `+tripNameText+`)`, words)
	args := append(append([]any{today, today, userID, userID}, ownArgs...), allArgs...)
	return `SELECT ` + qualify(alias, cols) + `, t.name, ` + tripWhenRank + ` AS w FROM ` + table + ` ` + alias + ` JOIN trips t ON t.id = ` + alias + `.trip_id
		WHERE ` + tripVisible + ` AND (` + strings.Join(own, " OR ") + `) AND ` + strings.Join(all, " AND ") + `
		ORDER BY w, ` + whenOrder(date) + `, ` + alias + `.id DESC LIMIT ?`, append(args, limit)
}

// SearchLodging finds stays by name, address or reservation number.
func (s *Store) SearchLodging(ctx context.Context, userID int64, words []string, today string, limit int) ([]FoundLodging, error) {
	out := []FoundLodging{}
	if len(words) == 0 {
		return out, nil
	}
	q, args := searchInside(lodgingCols, "lodging", "l", lodgingText, "l.check_in", userID, words, today, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f FoundLodging
		l, err := scanLodging(scanWith{rows, []any{&f.TripName, &f.When}})
		if err != nil {
			return nil, err
		}
		f.Lodging = *l
		out = append(out, f)
	}
	return out, rows.Err()
}

// SearchTransport finds legs by name, carrier, number, from, to or booking reference.
func (s *Store) SearchTransport(ctx context.Context, userID int64, words []string, today string, limit int) ([]FoundTransport, error) {
	out := []FoundTransport{}
	if len(words) == 0 {
		return out, nil
	}
	q, args := searchInside(transportCols, "transport", "x", transportText, "x.depart_at", userID, words, today, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f FoundTransport
		x, err := scanTransport(scanWith{rows, []any{&f.TripName, &f.When}})
		if err != nil {
			return nil, err
		}
		f.Transport = *x
		out = append(out, f)
	}
	return out, rows.Err()
}

// SearchPlaces finds places by name or address.
func (s *Store) SearchPlaces(ctx context.Context, userID int64, words []string, today string, limit int) ([]FoundPlace, error) {
	out := []FoundPlace{}
	if len(words) == 0 {
		return out, nil
	}
	q, args := searchInside(placeCols, "places", "p", placeText, "p.day_date", userID, words, today, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f FoundPlace
		p, err := scanPlace(scanWith{rows, []any{&f.TripName, &f.When}})
		if err != nil {
			return nil, err
		}
		f.Place = *p
		out = append(out, f)
	}
	return out, rows.Err()
}
