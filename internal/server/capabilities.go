package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Travel (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They read what the trip pages read, as the request's person: their own
// trips and the ones shared with them. Nothing is changed, and nothing is
// fetched from outside (no weather, route, photos or costs).

func idStr(n int64) string { return strconv.FormatInt(n, 10) }

// dots joins the parts of a line that are there: "Lisbon · 14 May – 18 May 2026".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// wall is a stored stamp's day and time of day: "2026-08-28T07:30" is the
// 28th at 07:30 where it happens; a plain date has no time.
func wall(stamp string) (day, clock string) {
	stamp = strings.TrimSpace(stamp)
	if len(stamp) < 10 {
		return "", ""
	}
	if _, err := store.ParseDate(stamp[:10]); err != nil {
		return "", ""
	}
	return stamp[:10], clockOf(stamp[10:])
}

// clockOf is a valid "HH:MM" out of "07:30", "T07:30" or "T07:30:00+02:00", else "".
func clockOf(s string) string {
	s = strings.TrimLeft(strings.TrimSpace(s), "T ")
	if len(s) < 5 {
		return ""
	}
	if _, err := time.Parse("15:04", s[:5]); err != nil {
		return ""
	}
	return s[:5]
}

// wording writes dates and times the way the person chose in Settings.
type wording struct{ date, time string }

func (s *Server) wordingFor(ctx context.Context, userID int64) wording {
	st, _ := s.store.GetSettings(ctx, userID)
	return wording{date: st.DateFormat, time: st.TimeFormat}
}

// day is a date as the pages show it: "14 Aug 2026", or "14 Aug" without the year.
func (w wording) day(d string, withYear bool) string {
	if len(d) < 10 {
		return ""
	}
	t, err := store.ParseDate(d[:10])
	if err != nil {
		return ""
	}
	layouts := map[string][2]string{
		"dmy-dot": {"02.01.2006", "02.01."}, "dmy-slash": {"02/01/2006", "02/01"}, "mdy-slash": {"01/02/2006", "01/02"},
		"ymd": {"2006-01-02", "01-02"}, "mmmdy": {"Jan 2, 2006", "Jan 2"},
	}
	l, ok := layouts[w.date]
	if !ok {
		l = [2]string{"2 Jan 2006", "2 Jan"}
	}
	if withYear {
		return t.Format(l[0])
	}
	return t.Format(l[1])
}

// span is two dates as the trip's page shows them: "14 Aug – 28 Aug 2026".
func (w wording) span(a, b string) string {
	switch {
	case a == "" && b == "":
		return ""
	case b == "" || a == b:
		return w.day(firstNonEmpty(a, b), true)
	case a == "":
		return w.day(b, true)
	}
	return w.day(a, a[:4] != b[:4]) + " – " + w.day(b, true)
}

// clock is a time of day: "15:00", or "3:00 PM" for someone who reads the clock that way.
func (w wording) clock(hhmm string) string {
	if w.time != "12h" || hhmm == "" {
		return hhmm
	}
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return hhmm
	}
	return t.Format("3:04 PM")
}

// near says which day a coming thing is on: "Today", "Tomorrow", "Sat 14 Aug".
func (w wording) near(day, today string) string {
	t, err := store.ParseDate(day)
	if err != nil {
		return ""
	}
	switch store.DaysBetween(today, day) {
	case 1:
		return "Today"
	case 2:
		return "Tomorrow"
	}
	return t.Format("Mon") + " " + w.day(day, day[:4] != today[:4])
}

// tripPlace is where a trip goes, or its name while no destination is set.
func tripPlace(t *store.Trip) string {
	return firstNonEmpty(strings.TrimSpace(t.Destination), strings.TrimSpace(t.Name))
}

// tripDayLine is a running trip on one of its days, as its page says it:
// "Day 2 of 5 in Lisbon".
func tripDayLine(t *store.Trip, day string) string {
	n := store.DaysBetween(t.StartDate, day)
	if d := strings.TrimSpace(t.Destination); d != "" {
		return fmt.Sprintf("Day %d of %d in %s", n, t.Days, d)
	}
	return fmt.Sprintf("%s, day %d of %d", strings.TrimSpace(t.Name), n, t.Days)
}

// countdownLine is a trip that has not started: "Lisbon in 12 days".
func countdownLine(t *store.Trip, today string) string {
	if n := store.DaysBetween(today, t.StartDate) - 1; n > 1 {
		return fmt.Sprintf("%s in %d days", tripPlace(t), n)
	}
	return tripPlace(t) + " tomorrow"
}

// nights: "1 night", "3 nights", nothing for a stay without both dates.
func nights(in, out string) string {
	switch n := store.DaysBetween(in, out) - 1; {
	case n == 1:
		return "1 night"
	case n > 1:
		return fmt.Sprintf("%d nights", n)
	}
	return ""
}

// legRoute is "Vienna → Lisbon" for a leg that has a name of its own.
func legRoute(x *store.Transport) string {
	if x.Name == "" || (x.FromName == "" && x.ToName == "") {
		return ""
	}
	return strings.TrimSpace(x.FromName + " → " + x.ToName)
}

// ---------- the plan ----------

// planRow is one line of a trip's itinerary: a leg, a check-in or check-out,
// or a place with a day.
type planRow struct {
	id         string
	day        string // YYYY-MM-DD
	time, end  string // HH:MM where it happens, "" = sometime that day
	order      string // where it sits in its day, as the itinerary sorts it
	title, sub string
	kind       string // flight, train…, stay, plan
	note       string // the person's words about a place; a booking's notes (door codes, references) stay in the app
	done       bool   // a place that was visited
	short      string // for the day's one line: "Flight to Lisbon 07:30", "check-in at Hotel Lisboa"
	url        string
}

// plan lists what the trip's itinerary has from one day to another (both
// included; to "" = no end), in the order of the page, and where the person
// sleeps on the first of those days. It reads what the trip's page reads.
func (s *Server) plan(ctx context.Context, t *store.Trip, from, to string, w wording) ([]planRow, string, error) {
	places, err := s.store.ListPlaces(ctx, t.ID)
	if err != nil {
		return nil, "", err
	}
	lodging, err := s.store.ListLodging(ctx, t.ID)
	if err != nil {
		return nil, "", err
	}
	transport, err := s.store.ListTransport(ctx, t.ID)
	if err != nil {
		return nil, "", err
	}
	in := func(day string) bool { return day != "" && day >= from && (to == "" || day <= to) }
	// "Belém Tower 10:00", or just the name when no time is set
	at := func(name, clock string) string { return strings.TrimSpace(name + " " + w.clock(clock)) }
	base := "/trips/" + idStr(t.ID)
	rows := []planRow{}
	for i := range transport {
		x := &transport[i]
		label, kind := transportLabel(x), x.Type
		if kind == "" || kind == "other" {
			kind = "travel"
		}
		dep, depAt := wall(x.DepartAt)
		arr, arrAt := wall(x.ArriveAt)
		row := planRow{id: "transport:" + idStr(x.ID), kind: kind, url: base + "/stay"}
		seat := ""
		if x.Seat != "" {
			seat = "seat " + x.Seat
		}
		switch {
		case in(dep):
			row.day, row.time, row.order, row.title, row.short = dep, depAt, firstNonEmpty(depAt, "00:00"), label, at(label, depAt)
			arrives := ""
			switch {
			case arr == dep && arrAt > depAt && depAt != "":
				row.end = arrAt
			case arr > dep:
				arrives = strings.TrimSpace("arrives " + w.day(arr, false) + " " + w.clock(arrAt))
			}
			row.sub = dots(legRoute(x), x.Carrier, x.Number, seat, arrives)
		case in(arr) && arr != dep:
			// a leg that left on an earlier day and lands on this one
			row.id += ":arrival"
			row.day, row.time, row.order, row.title = arr, arrAt, firstNonEmpty(arrAt, "00:00"), "Arrival · "+label
			row.short = strings.TrimSpace(label + " arrives " + w.clock(arrAt))
			row.sub = dots(legRoute(x), x.Carrier, x.Number, seat)
		default:
			continue
		}
		rows = append(rows, row)
	}
	staying := ""
	for i := range lodging {
		l := &lodging[i]
		ci, ciAt := wall(l.CheckIn)
		co, coAt := wall(l.CheckOut)
		if ci != "" && co != "" && ci <= from && from < co {
			staying = l.Name
		}
		row := planRow{id: "stay:" + idStr(l.ID), kind: "stay", url: base + "/stay"}
		// the itinerary puts a check-in in the afternoon and a check-out in the morning
		if in(ci) && ci == co {
			row.day, row.time, row.order, row.title, row.sub = ci, ciAt, firstNonEmpty(ciAt, "15:00"), "Check in and out · "+l.Name, l.Address
			row.short = "check-in at " + l.Name
			rows = append(rows, row)
			continue
		}
		if in(ci) {
			r := row
			r.id += ":in"
			r.day, r.time, r.order, r.title, r.sub = ci, ciAt, firstNonEmpty(ciAt, "15:00"), "Check in · "+l.Name, dots(l.Address, nights(ci, co))
			if r.short = "check-in " + w.clock(ciAt); ciAt == "" {
				r.short = "check-in at " + l.Name
			}
			rows = append(rows, r)
		}
		if in(co) {
			r := row
			r.id += ":out"
			r.day, r.time, r.order, r.title, r.sub = co, coAt, firstNonEmpty(coAt, "10:00"), "Check out · "+l.Name, l.Address
			if r.short = "check-out " + w.clock(coAt); coAt == "" {
				r.short = "check-out from " + l.Name
			}
			rows = append(rows, r)
		}
	}
	for i := range places {
		p := &places[i]
		if !in(p.DayDate) || p.Status == "skipped" {
			continue
		}
		start := clockOf(p.StartTime)
		row := planRow{id: "place:" + idStr(p.ID), day: p.DayDate, time: start, order: firstNonEmpty(start, fmt.Sprintf("99:%03d", p.SortOrder)),
			title: p.Name, sub: p.Address, kind: "plan", note: p.Notes, done: p.Status == "visited", short: at(p.Name, start), url: base + "/itinerary"}
		if start != "" && p.DurationMin > 0 {
			a, _ := time.Parse("15:04", start)
			if b := a.Add(time.Duration(p.DurationMin) * time.Minute); b.Day() == a.Day() {
				row.end = b.Format("15:04")
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].day != rows[j].day {
			return rows[i].day < rows[j].day
		}
		return rows[i].order < rows[j].order
	})
	return rows, staying, nil
}

// ---------- search ----------

// pocketSearch answers Pocket Search: the person's trips by name, destination
// and description, and the stays, legs and places inside them by their own
// names. A word may also be in the trip's name ("lisbon hotel"), but a trip's
// name alone never brings up everything in it. Trips the person is on or has
// ahead come before past ones; a trip found only by its description comes
// last.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	words := store.SearchTerms(q)
	if len(words) == 0 {
		return nil, nil
	}
	today := store.Today()
	trips, err := s.store.SearchTrips(ctx, uid, words, today, limit)
	if err != nil {
		return nil, err
	}
	stays, err := s.store.SearchLodging(ctx, uid, words, today, limit)
	if err != nil {
		return nil, err
	}
	legs, err := s.store.SearchTransport(ctx, uid, words, today, limit)
	if err != nil {
		return nil, err
	}
	places, err := s.store.SearchPlaces(ctx, uid, words, today, limit)
	if err != nil {
		return nil, err
	}
	w := s.wordingFor(ctx, uid)
	out, late := []core.SearchItem{}, []core.SearchItem{}
	for i := range trips {
		t := &trips[i]
		state := ""
		switch {
		case t.Archived:
			state = "Archived"
		case t.Status == store.StatusCancelled:
			state = "Cancelled"
		}
		it := core.SearchItem{ID: "trip:" + idStr(t.ID), Title: t.Name, Kind: "trip", Date: t.StartDate, URL: "/trips/" + idStr(t.ID),
			Snippet: dots(t.Destination, firstNonEmpty(w.span(t.StartDate, t.EndDate), "no dates yet"), state)}
		if t.ByName {
			out = append(out, it)
			continue
		}
		// say where the words were found when the line above does not show it
		it.Snippet = dots(it.Snippet, t.Description)
		late = append(late, it)
	}
	type inside struct {
		when int
		item core.SearchItem
	}
	found := []inside{}
	for i := range stays {
		l := &stays[i]
		ci, _ := wall(l.CheckIn)
		co, _ := wall(l.CheckOut)
		ref := ""
		if l.ReservationNumber != "" {
			ref = "#" + l.ReservationNumber
		}
		found = append(found, inside{l.When, core.SearchItem{ID: "stay:" + idStr(l.ID), Title: l.Name, Kind: "booking", Date: ci,
			URL: "/trips/" + idStr(l.TripID) + "/stay", Snippet: dots(l.TripName, w.span(ci, co), l.Address, ref)}})
	}
	for i := range legs {
		x := &legs[i]
		dep, depAt := wall(x.DepartAt)
		ref := ""
		if x.BookingRef != "" {
			ref = "ref " + x.BookingRef
		}
		found = append(found, inside{x.When, core.SearchItem{ID: "transport:" + idStr(x.ID), Title: transportLabel(&x.Transport), Kind: "booking", Date: dep,
			URL:     "/trips/" + idStr(x.TripID) + "/stay",
			Snippet: dots(x.TripName, strings.TrimSpace(w.day(dep, true)+" "+w.clock(depAt)), legRoute(&x.Transport), x.Carrier, x.Number, ref)}})
	}
	for i := range places {
		p := &places[i]
		// a place with a day is on the itinerary, one without is among the trip's places
		tab := "/places"
		if p.DayDate != "" {
			tab = "/itinerary"
		}
		found = append(found, inside{p.When, core.SearchItem{ID: "place:" + idStr(p.ID), Title: p.Name, Kind: "stop", Date: p.DayDate,
			URL:     "/trips/" + idStr(p.TripID) + tab,
			Snippet: dots(p.TripName, strings.TrimSpace(w.day(p.DayDate, true)+" "+w.clock(clockOf(p.StartTime))), p.Address)}})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].when < found[j].when })
	for _, f := range found {
		out = append(out, f.item)
	}
	if out = append(out, late...); len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- day ----------

// summaryParts is how many things the day's one line names before "and N more".
const summaryParts = 4

// pocketDay answers Pocket Day: the trip the person is on that day and what
// its itinerary has for the day (legs, check-in and check-out, the places
// with that day). Archived and cancelled trips say nothing, like on the first
// page. A trip's days are plain dates where it happens, so the time zone is
// not used.
func (s *Server) pocketDay(r *http.Request, date string, _ *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	trips, err := s.store.TripsOn(ctx, uid, date)
	if err != nil {
		return nil, err
	}
	if len(trips) == 0 {
		return nil, nil
	}
	w := s.wordingFor(ctx, uid)
	day := &core.Day{Date: date}
	var lines []string
	for i := range trips {
		t := &trips[i]
		rows, staying, err := s.plan(ctx, t, date, date, w)
		if err != nil {
			return nil, err
		}
		line := tripDayLine(t, date)
		head := core.DayItem{ID: "trip:" + idStr(t.ID), Title: line, Kind: "trip day", URL: "/trips/" + idStr(t.ID) + "/itinerary"}
		if staying != "" {
			head.Sub = "Staying at " + staying
		}
		if name := strings.TrimSpace(t.Name); !strings.Contains(line, name) {
			head.Sub = dots(name, head.Sub)
		}
		day.Items = append(day.Items, head)
		var parts []string
		for _, row := range rows {
			day.Items = append(day.Items, core.DayItem{ID: row.id, Time: row.time, End: row.end, Title: row.title, Sub: row.sub, Kind: row.kind,
				Done: row.done, URL: row.url, Note: row.note})
			parts = append(parts, row.short)
		}
		if n := len(parts); n > summaryParts {
			line += ": " + strings.Join(parts[:summaryParts-1], ", ") + fmt.Sprintf(" and %d more", n-summaryParts+1)
		} else if n > 0 {
			line += ": " + strings.Join(parts, ", ")
		}
		lines = append(lines, line)
	}
	day.Summary = strings.Join(lines, "; ")
	return day, nil
}

// ---------- widget ----------

// widgetRows is how many coming things the widget lists.
const widgetRows = 5

// pocketWidget answers Pocket Hub: the trip the person is on ("Day 2 of 5 in
// Lisbon") or the next one ("Lisbon in 12 days"), as on the first page, with
// what its itinerary has next. Nothing planned shows nothing. It reads one
// trip, however many the person has.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	today := store.Today()
	var t *store.Trip
	out := &core.WidgetData{}
	on, err := s.store.TripsOn(ctx, uid, today)
	if err != nil {
		return nil, err
	}
	if len(on) > 0 {
		t = &on[0]
		out.Line = tripDayLine(t, today)
	} else {
		t, err = s.store.NextTrip(ctx, uid, today)
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		out.Line = countdownLine(t, today)
	}
	out.URL = "/trips/" + idStr(t.ID)
	w := s.wordingFor(ctx, uid)
	rows, _, err := s.plan(ctx, t, today, "", w)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.done {
			continue // already been there
		}
		out.Items = append(out.Items, core.WidgetItem{ID: row.id, Title: row.title, Detail: dots(w.near(row.day, today), w.clock(row.time)), URL: row.url})
		if len(out.Items) == widgetRows {
			break
		}
	}
	return out, nil
}
