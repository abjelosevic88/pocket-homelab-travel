package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask (capabilities.go), as a
// signed-in person and through a Pocket Core link token
// pcx_<owner|member|admin2>_<search|day|widget> (the fake Core in
// newIsoHarness vouches for those).

// dayFrom is the day n days from today, as the app's own Today counts.
func dayFrom(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }

// answer calls a route and decodes its JSON object; any other status than 200 fails.
func (h *isoHarness) answer(tok, path string) map[string]any {
	h.t.Helper()
	code, b := h.do(tok, "GET", path, nil)
	if code != 200 {
		h.t.Fatalf("GET %s: %d %s", path, code, b)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		h.t.Fatalf("GET %s: %v in %s", path, err, b)
	}
	return m
}

// place adds a place straight to the store: the route would also look for
// its pictures on the internet.
func (h *isoHarness) place(trip int64, p store.Place) int64 {
	h.t.Helper()
	p.TripID = trip
	if err := h.st.CreatePlace(context.Background(), &p); err != nil {
		h.t.Fatal(err)
	}
	return p.ID
}

// items are the rows of a capability answer.
func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

// item finds the row with that title.
func item(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range items(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

// titles are the rows' titles, in order.
func titles(t *testing.T, m map[string]any) string {
	t.Helper()
	var out []string
	for _, it := range items(t, m) {
		out = append(out, it["title"].(string))
	}
	return strings.Join(out, " | ")
}

// lisbon is the owner's trip to Lisbon, from in days from today for five
// days, with a flight in, a hotel and two places.
type lisbon struct{ trip, flight, hotel, tower, market int64 }

func (h *isoHarness) lisbon(in int) lisbon {
	h.t.Helper()
	var l lisbon
	l.trip = h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Spring in Lisbon", "destination": "Lisbon", "description": "Pastel de nata tour",
		"start_date": dayFrom(in), "end_date": dayFrom(in + 4)})
	base := "/api/v1/trips/" + itoa(l.trip)
	l.flight = h.create(h.ownerTok, base+"/transport", map[string]any{"type": "flight", "from_name": "Vienna (VIE)", "to_name": "Lisbon (LIS)", "carrier": "TAP",
		"number": "TP 1273", "seat": "14A", "booking_ref": "K7XP2Q", "depart_at": dayFrom(in) + "T07:30", "arrive_at": dayFrom(in) + "T10:05"})
	l.hotel = h.create(h.ownerTok, base+"/lodging", map[string]any{"name": "Hotel Lisboa Plaza", "address": "Travessa do Salitre 7", "reservation_number": "RES-778",
		"check_in": dayFrom(in), "check_out": dayFrom(in + 4), "notes": "Door code 2580"})
	l.tower = h.place(l.trip, store.Place{Name: "Belém Tower", Address: "Av. Brasília", DayDate: dayFrom(in + 1), StartTime: "11:00", DurationMin: 90, Status: "planned",
		Notes: "Go early"})
	l.market = h.place(l.trip, store.Place{Name: "Time Out Market", Status: "idea"})
	return l
}

func TestPocketSearch(t *testing.T) {
	h := newIsoHarness(t)
	l := h.lisbon(12)
	when := (wording{}).span(dayFrom(12), dayFrom(16))
	past := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Old Lisbon weekend", "start_date": "2024-03-01", "end_date": "2024-03-03"})
	gone := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Lisbon that never was", "start_date": dayFrom(30), "end_date": dayFrom(33), "status": "cancelled"})
	rome := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Rome", "destination": "Rome", "description": "We flew home via Lisbon",
		"start_date": "2023-05-01", "end_date": "2023-05-04"})
	h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Paris", "destination": "Paris"})
	h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Member in Lisbon", "start_date": dayFrom(3), "end_date": dayFrom(5)})

	// trips by name and destination: the one ahead, the past one, the cancelled one; then
	// what is inside trips; a trip found only by its description comes last
	res := h.answer(h.ownerTok, "/api/v1/pocket/search?q=lisbon&limit=20")
	if got := titles(t, res); got != "Spring in Lisbon | Old Lisbon weekend | Lisbon that never was | Vienna (VIE) → Lisbon (LIS) | Rome" {
		t.Fatalf("lisbon: %s", got)
	}
	all := items(t, res)
	if it := all[0]; it["id"] != "trip:"+itoa(l.trip) || it["kind"] != "trip" || it["url"] != "/trips/"+itoa(l.trip) || it["date"] != dayFrom(12) || it["snippet"] != "Lisbon · "+when {
		t.Fatalf("the trip ahead: %v", it)
	}
	if it := all[1]; it["id"] != "trip:"+itoa(past) || it["snippet"] != "1 Mar – 3 Mar 2024" {
		t.Fatalf("the past trip: %v", it)
	}
	if it := all[2]; it["id"] != "trip:"+itoa(gone) || !strings.HasSuffix(it["snippet"].(string), " · Cancelled") {
		t.Fatalf("the cancelled trip: %v", it)
	}
	if it := all[3]; it["id"] != "transport:"+itoa(l.flight) || it["kind"] != "booking" || it["url"] != "/trips/"+itoa(l.trip)+"/stay" || it["date"] != dayFrom(12) ||
		it["snippet"] != "Spring in Lisbon · "+(wording{}).day(dayFrom(12), true)+" 07:30 · TAP · TP 1273 · ref K7XP2Q" {
		t.Fatalf("the flight: %v", it)
	}
	if it := all[4]; it["id"] != "trip:"+itoa(rome) || it["snippet"] != "Rome · 1 May – 4 May 2023 · We flew home via Lisbon" {
		t.Fatalf("found by its description: %v", it)
	}
	// a word of the trip and a word of the thing: the hotel, not everything in Lisbon
	res = h.answer(h.ownerTok, "/api/v1/pocket/search?q=lisbon+hotel")
	if it := items(t, res); len(it) != 1 || it[0]["id"] != "stay:"+itoa(l.hotel) || it[0]["title"] != "Hotel Lisboa Plaza" || it[0]["kind"] != "booking" ||
		it[0]["url"] != "/trips/"+itoa(l.trip)+"/stay" || it[0]["date"] != dayFrom(12) || it[0]["snippet"] != "Spring in Lisbon · "+when+" · Travessa do Salitre 7 · #RES-778" {
		t.Fatalf("lisbon hotel: %v", it)
	}
	// by reservation number, flight number, and a name with letters outside a–z in any case
	if it := items(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q=res-778")); len(it) != 1 || it[0]["id"] != "stay:"+itoa(l.hotel) {
		t.Fatalf("by reservation number: %v", it)
	}
	if it := items(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q=tp+1273")); len(it) != 1 || it[0]["id"] != "transport:"+itoa(l.flight) {
		t.Fatalf("by flight number: %v", it)
	}
	for _, q := range []string{"belém", "BELÉM", "Belém tower"} {
		it := items(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q="+url.QueryEscape(q)))
		if len(it) != 1 || it[0]["id"] != "place:"+itoa(l.tower) || it[0]["kind"] != "stop" || it[0]["url"] != "/trips/"+itoa(l.trip)+"/itinerary" || it[0]["date"] != dayFrom(13) ||
			it[0]["snippet"] != "Spring in Lisbon · "+(wording{}).day(dayFrom(13), true)+" 11:00 · Av. Brasília" {
			t.Fatalf("%s: %v", q, it)
		}
	}
	// a place without a day opens among the trip's places
	if it := items(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q=time+out")); len(it) != 1 || it[0]["id"] != "place:"+itoa(l.market) ||
		it[0]["url"] != "/trips/"+itoa(l.trip)+"/places" || it[0]["snippet"] != "Spring in Lisbon" || it[0]["date"] != nil {
		t.Fatalf("an idea: %v", it)
	}
	// in a trip still ahead: what has a day first, soonest first
	h.place(l.trip, store.Place{Name: "Feira market", DayDate: dayFrom(15), Status: "planned"})
	h.place(l.trip, store.Place{Name: "Campo de Ourique market", DayDate: dayFrom(14), Status: "planned"})
	if got := titles(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q=market")); got != "Campo de Ourique market | Feira market | Time Out Market" {
		t.Fatalf("market: %s", got)
	}
	if it := items(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q=lisbon&limit=2")); len(it) != 2 || it[1]["title"] != "Old Lisbon weekend" {
		t.Fatalf("limit 2: %v", it)
	}
	// nothing to look for, and signs that mean "anything" to the database
	for _, q := range []string{"", "??", "*", "%", "_", "% _", "lisbon%nata", "xyzzy"} {
		if it := items(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q="+url.QueryEscape(q))); len(it) != 0 {
			t.Fatalf("%q finds %v", q, it)
		}
	}
	// the date the way the person reads it
	h.do(h.ownerTok, "PUT", "/api/v1/settings", map[string]any{"date_format": "dmy-dot", "time_format": "12h"})
	if it := items(t, h.answer(h.ownerTok, "/api/v1/pocket/search?q=old+lisbon")); len(it) != 1 || it[0]["snippet"] != "01.03. – 03.03.2024" {
		t.Fatalf("the person's date format: %v", it)
	}

	// another person's trips never appear, and theirs do not appear here
	if got := titles(t, h.answer(h.memberTok, "/api/v1/pocket/search?q=lisbon")); got != "Member in Lisbon" {
		t.Fatalf("the member finds: %s", got)
	}
	for _, q := range []string{"lisbon", "hotel", "tp 1273", "belém", "rome", "paris"} {
		if it := items(t, h.answer(h.admin2Tok, "/api/v1/pocket/search?q="+url.QueryEscape(q))); len(it) != 0 {
			t.Fatalf("the second admin finds the owner's %q: %v", q, it)
		}
	}
	// a shared trip is found by who it is shared with, with what is in it
	if code, b := h.do(h.ownerTok, "PUT", "/api/v1/trips/"+itoa(l.trip)+"/members", map[string]any{"members": []any{map[string]any{"user_id": h.member.ID, "role": "viewer"}}}); code != 200 {
		t.Fatalf("share: %d %s", code, b)
	}
	if got := titles(t, h.answer(h.memberTok, "/api/v1/pocket/search?q=lisbon")); got != "Member in Lisbon | Spring in Lisbon | Vienna (VIE) → Lisbon (LIS)" {
		t.Fatalf("the member, after sharing: %s", got)
	}
	if code, _ := h.do("", "GET", "/api/v1/pocket/search?q=lisbon", nil); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
	// the list other apps read is as it was: a bare array, by name and destination
	code, b := h.do(h.ownerTok, "GET", "/api/v1/trips?q=lisbon", nil)
	var old []map[string]any
	if err := json.Unmarshal(b, &old); code != 200 || err != nil || len(old) != 3 || old[0]["counts"] == nil {
		t.Fatalf("/trips changed: %d %v %s", code, err, b)
	}
}

func TestPocketDay(t *testing.T) {
	h := newIsoHarness(t)
	l := h.lisbon(-1) // today is day 2 of 5
	base := "/api/v1/trips/" + itoa(l.trip)
	today, tomorrow := dayFrom(0), dayFrom(1)
	h.do(h.ownerTok, "PUT", base+"/places/"+itoa(l.tower), map[string]any{"day_date": today, "status": "visited"})
	train := h.create(h.ownerTok, base+"/transport", map[string]any{"type": "train", "name": "Train to Sintra", "from_name": "Rossio", "to_name": "Sintra",
		"depart_at": today + "T09:10", "arrive_at": today + "T09:50"})
	inn := h.create(h.ownerTok, base+"/lodging", map[string]any{"name": "Airport Inn", "check_in": dayFrom(-1), "check_out": today})
	h.do(h.ownerTok, "PUT", base+"/lodging/"+itoa(l.hotel), map[string]any{"check_in": today})
	dinner := h.place(l.trip, store.Place{Name: "Dinner at Ramiro", DayDate: today, StartTime: "20:00", Status: "booked"})
	h.place(l.trip, store.Place{Name: "Tile museum", DayDate: today, StartTime: "16:00", Status: "skipped"})
	h.place(l.trip, store.Place{Name: "LX Factory", DayDate: tomorrow, Status: "planned"})

	day := h.answer(h.ownerTok, "/api/v1/pocket/day/"+today)
	if day["date"] != today || day["summary"] != "Day 2 of 5 in Lisbon: Train to Sintra 09:10, check-out from Airport Inn, Belém Tower 11:00 and 2 more" {
		t.Fatalf("day: %v", day)
	}
	// in the order of the itinerary; the skipped place and the idea are not there
	if got := titles(t, day); got != "Day 2 of 5 in Lisbon | Train to Sintra | Check out · Airport Inn | Belém Tower | Check in · Hotel Lisboa Plaza | Dinner at Ramiro" {
		t.Fatalf("the day's rows: %s", got)
	}
	if it := item(t, day, "Day 2 of 5 in Lisbon"); it["id"] != "trip:"+itoa(l.trip) || it["kind"] != "trip day" || it["url"] != "/trips/"+itoa(l.trip)+"/itinerary" ||
		it["sub"] != "Spring in Lisbon · Staying at Hotel Lisboa Plaza" || it["time"] != nil {
		t.Fatalf("the trip's day: %v", it)
	}
	if it := item(t, day, "Train to Sintra"); it["id"] != "transport:"+itoa(train) || it["kind"] != "train" || it["time"] != "09:10" || it["end"] != "09:50" ||
		it["sub"] != "Rossio → Sintra" || it["url"] != "/trips/"+itoa(l.trip)+"/stay" {
		t.Fatalf("the train: %v", it)
	}
	if it := item(t, day, "Check out · Airport Inn"); it["id"] != "stay:"+itoa(inn)+":out" || it["kind"] != "stay" || it["time"] != nil {
		t.Fatalf("check-out: %v", it)
	}
	// a booking's notes (the door code) stay in the app
	if it := item(t, day, "Check in · Hotel Lisboa Plaza"); it["id"] != "stay:"+itoa(l.hotel)+":in" || it["kind"] != "stay" || it["sub"] != "Travessa do Salitre 7 · 3 nights" ||
		it["note"] != nil || it["url"] != "/trips/"+itoa(l.trip)+"/stay" {
		t.Fatalf("check-in: %v", it)
	}
	if it := item(t, day, "Belém Tower"); it["id"] != "place:"+itoa(l.tower) || it["kind"] != "plan" || it["time"] != "11:00" || it["end"] != "12:30" || it["done"] != true ||
		it["sub"] != "Av. Brasília" || it["note"] != "Go early" || it["url"] != "/trips/"+itoa(l.trip)+"/itinerary" {
		t.Fatalf("a place that was visited: %v", it)
	}
	if it := item(t, day, "Dinner at Ramiro"); it["id"] != "place:"+itoa(dinner) || it["time"] != "20:00" || it["end"] != nil || it["done"] != nil {
		t.Fatalf("dinner: %v", it)
	}
	// the day before: the flight in and the first check-in
	if d := h.answer(h.ownerTok, "/api/v1/pocket/day/"+dayFrom(-1)); d["summary"] != "Day 1 of 5 in Lisbon: Vienna (VIE) → Lisbon (LIS) 07:30, check-in at Airport Inn" ||
		item(t, d, "Vienna (VIE) → Lisbon (LIS)")["kind"] != "flight" || item(t, d, "Vienna (VIE) → Lisbon (LIS)")["sub"] != "TAP · TP 1273 · seat 14A" ||
		item(t, d, "Day 1 of 5 in Lisbon")["sub"] != "Spring in Lisbon · Staying at Airport Inn" {
		t.Fatalf("the first day: %v", d)
	}
	if d := h.answer(h.ownerTok, "/api/v1/pocket/day/"+tomorrow+"?tz=Europe/Sarajevo"); d["summary"] != "Day 3 of 5 in Lisbon: LX Factory" || len(items(t, d)) != 2 {
		t.Fatalf("tomorrow: %v", d)
	}
	// a day with nothing planned still says where the person is; the last day has the check-out
	if d := h.answer(h.ownerTok, "/api/v1/pocket/day/"+dayFrom(2)); d["summary"] != "Day 4 of 5 in Lisbon" || len(items(t, d)) != 1 {
		t.Fatalf("a free day: %v", d)
	}
	if d := h.answer(h.ownerTok, "/api/v1/pocket/day/"+dayFrom(3)); d["summary"] != "Day 5 of 5 in Lisbon: check-out from Hotel Lisboa Plaza" ||
		item(t, d, "Day 5 of 5 in Lisbon")["sub"] != "Spring in Lisbon" {
		t.Fatalf("the last day: %v", d)
	}
	// the clock the way the person reads it, in the line only
	h.do(h.ownerTok, "PUT", "/api/v1/settings", map[string]any{"time_format": "12h"})
	if d := h.answer(h.ownerTok, "/api/v1/pocket/day/"+today); !strings.HasPrefix(d["summary"].(string), "Day 2 of 5 in Lisbon: Train to Sintra 9:10 AM, ") ||
		item(t, d, "Train to Sintra")["time"] != "09:10" {
		t.Fatalf("12-hour clock: %v", d)
	}
	// days off the trip, and other people's days
	for _, d := range []string{dayFrom(-2), dayFrom(4), "2019-03-04"} {
		if got := h.answer(h.ownerTok, "/api/v1/pocket/day/"+d); got["date"] != d || got["summary"] != nil || len(items(t, got)) != 0 {
			t.Fatalf("a day off the trip: %v", got)
		}
	}
	for _, tok := range []string{h.memberTok, h.admin2Tok} {
		if got := h.answer(tok, "/api/v1/pocket/day/"+today); got["summary"] != nil || len(items(t, got)) != 0 {
			t.Fatalf("somebody else sees the owner's day: %v", got)
		}
	}
	// shared with the member: the same day; taken back: nothing again
	h.do(h.ownerTok, "PUT", base+"/members", map[string]any{"members": []any{map[string]any{"user_id": h.member.ID, "role": "viewer"}}})
	if got := h.answer(h.memberTok, "/api/v1/pocket/day/"+today); len(items(t, got)) != 6 || !strings.HasPrefix(got["summary"].(string), "Day 2 of 5 in Lisbon: Train to Sintra 09:10") {
		t.Fatalf("the member on the shared trip: %v", got)
	}
	h.do(h.ownerTok, "PUT", base+"/members", map[string]any{"members": []any{}})
	if got := h.answer(h.memberTok, "/api/v1/pocket/day/"+today); len(items(t, got)) != 0 {
		t.Fatalf("the member after the trip was taken back: %v", got)
	}
	if code, _ := h.do(h.ownerTok, "GET", "/api/v1/pocket/day/today", nil); code != 400 {
		t.Fatalf("a bad date: %d", code)
	}
	if code, _ := h.do("", "GET", "/api/v1/pocket/day/"+today, nil); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
}

func TestPocketDayEdges(t *testing.T) {
	h := newIsoHarness(t)
	today := dayFrom(0)
	// a trip without a destination, a leg that lands today, and a room for the day
	road := h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Road trip", "start_date": dayFrom(-1), "end_date": dayFrom(1)})
	base := "/api/v1/trips/" + itoa(road)
	ferry := h.create(h.memberTok, base+"/transport", map[string]any{"type": "ferry", "name": "Night ferry", "from_name": "Split", "to_name": "Ancona",
		"depart_at": dayFrom(-1) + "T20:00", "arrive_at": today + "T07:00"})
	walk := h.create(h.memberTok, base+"/transport", map[string]any{"type": "other", "from_name": "Port", "to_name": "Old town", "depart_at": today})
	room := h.create(h.memberTok, base+"/lodging", map[string]any{"name": "Day room", "check_in": today, "check_out": today})
	d := h.answer(h.memberTok, "/api/v1/pocket/day/"+today)
	if d["summary"] != "Road trip, day 2 of 3: Port → Old town, Night ferry arrives 07:00, check-in at Day room" {
		t.Fatalf("summary: %v", d["summary"])
	}
	if it := item(t, d, "Road trip, day 2 of 3"); it["sub"] != nil || it["kind"] != "trip day" {
		t.Fatalf("the trip's day: %v", it)
	}
	if it := item(t, d, "Arrival · Night ferry"); it["id"] != "transport:"+itoa(ferry)+":arrival" || it["kind"] != "ferry" || it["time"] != "07:00" || it["sub"] != "Split → Ancona" {
		t.Fatalf("the ferry lands: %v", it)
	}
	if it := item(t, d, "Port → Old town"); it["id"] != "transport:"+itoa(walk) || it["kind"] != "travel" || it["time"] != nil || it["sub"] != nil {
		t.Fatalf("a leg without a time: %v", it)
	}
	if it := item(t, d, "Check in and out · Day room"); it["id"] != "stay:"+itoa(room) || it["kind"] != "stay" {
		t.Fatalf("a room for the day: %v", it)
	}
	// the day it left: when it lands is in the line below
	if it := item(t, h.answer(h.memberTok, "/api/v1/pocket/day/"+dayFrom(-1)), "Night ferry"); it["id"] != "transport:"+itoa(ferry) || it["time"] != "20:00" || it["end"] != nil ||
		it["sub"] != "Split → Ancona · arrives "+(wording{}).day(today, false)+" 07:00" {
		t.Fatalf("the ferry leaves: %v", it)
	}
	// a second trip on the same day is told too
	h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Conference", "destination": "Ancona", "start_date": today, "end_date": today})
	if d := h.answer(h.memberTok, "/api/v1/pocket/day/"+today); !strings.HasSuffix(d["summary"].(string), "check-in at Day room; Day 1 of 1 in Ancona") || len(items(t, d)) != 5 {
		t.Fatalf("two trips: %v", d)
	}
	// a cancelled trip and an archived one say nothing, like on the first page
	h.do(h.memberTok, "PUT", base, map[string]any{"status": "cancelled"})
	if d := h.answer(h.memberTok, "/api/v1/pocket/day/"+today); d["summary"] != "Day 1 of 1 in Ancona" {
		t.Fatalf("a cancelled trip: %v", d)
	}
	h.do(h.memberTok, "PUT", base, map[string]any{"status": "booked", "archived": true})
	if d := h.answer(h.memberTok, "/api/v1/pocket/day/"+today); d["summary"] != "Day 1 of 1 in Ancona" {
		t.Fatalf("an archived trip: %v", d)
	}
}

func TestPocketWidget(t *testing.T) {
	h := newIsoHarness(t)
	empty := func(tok, why string) {
		t.Helper()
		if w := h.answer(tok, "/api/v1/pocket/widget"); w["line"] != nil || w["url"] != nil || len(items(t, w)) != 0 {
			t.Fatalf("%s: %v", why, w)
		}
	}
	empty(h.ownerTok, "no trips at all")
	// only a past trip, a cancelled one and one without dates: nothing is planned
	h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Rome", "start_date": "2023-05-01", "end_date": "2023-05-04"})
	h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Oslo", "start_date": dayFrom(5), "end_date": dayFrom(8), "status": "cancelled"})
	h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Someday"})
	empty(h.ownerTok, "nothing ahead")

	// the next trip, with what comes first
	h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Tokyo", "destination": "Tokyo", "start_date": dayFrom(40), "end_date": dayFrom(50)})
	l := h.lisbon(12)
	near := func(n int) string {
		return time.Now().AddDate(0, 0, n).Format("Mon") + " " + (wording{}).day(dayFrom(n), dayFrom(n)[:4] != dayFrom(0)[:4])
	}
	w := h.answer(h.ownerTok, "/api/v1/pocket/widget")
	if w["line"] != "Lisbon in 12 days" || w["url"] != "/trips/"+itoa(l.trip) || w["metrics"] != nil {
		t.Fatalf("the next trip: %v", w)
	}
	if got := titles(t, w); got != "Vienna (VIE) → Lisbon (LIS) | Check in · Hotel Lisboa Plaza | Belém Tower | Check out · Hotel Lisboa Plaza" {
		t.Fatalf("what comes first: %s", got)
	}
	if it := item(t, w, "Vienna (VIE) → Lisbon (LIS)"); it["id"] != "transport:"+itoa(l.flight) || it["detail"] != near(12)+" · 07:30" || it["url"] != "/trips/"+itoa(l.trip)+"/stay" {
		t.Fatalf("the flight: %v", it)
	}
	if it := item(t, w, "Check in · Hotel Lisboa Plaza"); it["id"] != "stay:"+itoa(l.hotel)+":in" || it["detail"] != near(12) {
		t.Fatalf("check-in: %v", it)
	}
	if it := item(t, w, "Belém Tower"); it["id"] != "place:"+itoa(l.tower) || it["detail"] != near(13)+" · 11:00" || it["url"] != "/trips/"+itoa(l.trip)+"/itinerary" {
		t.Fatalf("a place: %v", it)
	}
	if it := item(t, w, "Check out · Hotel Lisboa Plaza"); it["id"] != "stay:"+itoa(l.hotel)+":out" {
		t.Fatalf("check-out: %v", it)
	}
	// a trip that starts tomorrow
	paris := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "A weekend away", "start_date": dayFrom(1), "end_date": dayFrom(2)})
	if w := h.answer(h.ownerTok, "/api/v1/pocket/widget"); w["line"] != "A weekend away tomorrow" || w["url"] != "/trips/"+itoa(paris) || len(items(t, w)) != 0 {
		t.Fatalf("tomorrow: %v", w)
	}

	// on a trip: the day, and the next things from today on, at most five; where the person already was is left out
	cur := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Porto", "destination": "Porto", "start_date": dayFrom(-1), "end_date": dayFrom(3)})
	base := "/api/v1/trips/" + itoa(cur)
	h.create(h.ownerTok, base+"/transport", map[string]any{"type": "flight", "name": "Flight out", "depart_at": dayFrom(-1) + "T06:00"})
	h.create(h.ownerTok, base+"/lodging", map[string]any{"name": "Ribeira flat", "check_in": dayFrom(-1), "check_out": dayFrom(3)})
	h.place(cur, store.Place{Name: "Livraria Lello", DayDate: dayFrom(0), StartTime: "09:00", Status: "visited"})
	h.place(cur, store.Place{Name: "Port cellars", DayDate: dayFrom(0), StartTime: "15:00", Status: "booked"})
	for _, name := range []string{"Bolhão market", "Serralves", "Foz", "Clérigos"} {
		h.place(cur, store.Place{Name: name, DayDate: dayFrom(1), Status: "planned"})
	}
	h.do(h.ownerTok, "PUT", "/api/v1/settings", map[string]any{"time_format": "12h"})
	w = h.answer(h.ownerTok, "/api/v1/pocket/widget")
	if w["line"] != "Day 2 of 5 in Porto" || w["url"] != "/trips/"+itoa(cur) {
		t.Fatalf("on a trip: %v", w)
	}
	if got := titles(t, w); got != "Port cellars | Bolhão market | Serralves | Foz | Clérigos" {
		t.Fatalf("what comes next: %s", got)
	}
	if it := item(t, w, "Port cellars"); it["detail"] != "Today · 3:00 PM" {
		t.Fatalf("today: %v", it)
	}
	if it := item(t, w, "Serralves"); it["detail"] != "Tomorrow" {
		t.Fatalf("tomorrow: %v", it)
	}

	// each person their own; a shared trip counts for who it is shared with
	empty(h.admin2Tok, "the second admin")
	empty(h.memberTok, "the member")
	h.do(h.ownerTok, "PUT", base+"/members", map[string]any{"members": []any{map[string]any{"user_id": h.member.ID, "role": "editor"}}})
	if w := h.answer(h.memberTok, "/api/v1/pocket/widget"); w["line"] != "Day 2 of 5 in Porto" || item(t, w, "Port cellars")["detail"] != "Today · 15:00" {
		t.Fatalf("the member on the shared trip: %v", w)
	}
	if code, _ := h.do("", "GET", "/api/v1/pocket/widget", nil); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
	// the Homepage numbers are as they were
	if d := h.answer(h.ownerTok, "/api/v1/dashboard"); d["current_trip"] != "Porto" || d["next_trip"] != "A weekend away" {
		t.Fatalf("/dashboard changed: %v", d)
	}
}

// A link token opens its one route (and /auth/me) and nothing else; the
// scope other apps read trips with does not open the new routes.
func TestPocketLinkTokens(t *testing.T) {
	h := newIsoHarness(t)
	l := h.lisbon(-1)
	h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Member Rome", "destination": "Rome", "start_date": dayFrom(-1), "end_date": dayFrom(2)})
	today := dayFrom(0)
	trip := "/api/v1/trips/" + itoa(l.trip)
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{"pcx_owner_search", "GET", "/api/v1/pocket/search?q=lisbon", 200},
		{"pcx_owner_day", "GET", "/api/v1/pocket/day/" + today, 200},
		{"pcx_owner_widget", "GET", "/api/v1/pocket/widget", 200},
		{"pcx_owner_widget", "GET", "/api/v1/auth/me", 200},
		// another capability's route
		{"pcx_owner_search", "GET", "/api/v1/pocket/day/" + today, 403},
		{"pcx_owner_search", "GET", "/api/v1/pocket/widget", 403},
		{"pcx_owner_day", "GET", "/api/v1/pocket/search?q=lisbon", 403},
		{"pcx_owner_day", "GET", "/api/v1/pocket/widget", 403},
		{"pcx_owner_widget", "GET", "/api/v1/pocket/search?q=lisbon", 403},
		{"pcx_owner_widget", "GET", "/api/v1/pocket/day/" + today, 403},
		// nothing is kept here from a quick line: there is no such route
		{"pcx_owner_widget", "POST", "/api/v1/pocket/capture", 404},
		{"pcx_owner_search", "POST", "/api/v1/pocket/search?q=lisbon", 405},
		// the rest of the app
		{"pcx_owner_search", "GET", "/api/v1/trips", 403},
		{"pcx_owner_search", "GET", "/api/v1/trips?q=lisbon", 403},
		{"pcx_owner_search", "GET", trip, 403},
		{"pcx_owner_day", "GET", trip + "/overview", 403},
		{"pcx_owner_day", "GET", trip + "/places", 403},
		{"pcx_owner_day", "GET", trip + "/journal", 403},
		{"pcx_owner_widget", "GET", "/api/v1/home", 403},
		{"pcx_owner_widget", "GET", "/api/v1/dashboard", 403},
		{"pcx_owner_widget", "GET", "/api/v1/settings", 403},
		{"pcx_owner_widget", "GET", "/api/v1/export.json", 403},
		{"pcx_owner_widget", "POST", "/api/v1/trips", 403},
		{"pcx_owner_widget", "PUT", trip, 403},
		{"pcx_owner_widget", "DELETE", trip, 403},
		// reading trips for another app does not open the new routes, and still reads trips
		{"pcx_member_trips", "GET", "/api/v1/pocket/search?q=rome", 403},
		{"pcx_member_trips", "GET", "/api/v1/pocket/day/" + today, 403},
		{"pcx_member_trips", "GET", "/api/v1/pocket/widget", 403},
		{"pcx_member_trips", "GET", "/api/v1/trips", 200},
		// Pocket Move's links neither
		{"pcx_owner_export", "GET", "/api/v1/pocket/widget", 403},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", 401},
	} {
		if code, b := h.do(c.tok, c.method, c.path, map[string]any{}); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, b)
		}
	}
	// the token acts for its person only
	if got := titles(t, h.answer("pcx_owner_search", "/api/v1/pocket/search?q=lisbon")); got != "Spring in Lisbon | Vienna (VIE) → Lisbon (LIS)" {
		t.Fatalf("the owner through the link: %s", got)
	}
	if got := items(t, h.answer("pcx_member_search", "/api/v1/pocket/search?q=lisbon")); len(got) != 0 {
		t.Fatalf("the member through the link finds the owner's trip: %v", got)
	}
	if got := titles(t, h.answer("pcx_member_search", "/api/v1/pocket/search?q=rome")); got != "Member Rome" {
		t.Fatalf("the member through the link: %s", got)
	}
	if d := h.answer("pcx_owner_day", "/api/v1/pocket/day/"+today); d["summary"] != "Day 2 of 5 in Lisbon: Belém Tower 11:00" {
		t.Fatalf("the owner's day through the link: %v", d)
	}
	if d := h.answer("pcx_member_day", "/api/v1/pocket/day/"+today); d["summary"] != "Day 2 of 4 in Rome" || len(items(t, d)) != 1 {
		t.Fatalf("the member's day through the link: %v", d)
	}
	if w := h.answer("pcx_owner_widget", "/api/v1/pocket/widget"); w["line"] != "Day 2 of 5 in Lisbon" {
		t.Fatalf("the owner's widget through the link: %v", w)
	}
	if w := h.answer("pcx_member_widget", "/api/v1/pocket/widget"); w["line"] != "Day 2 of 4 in Rome" || len(items(t, w)) != 0 {
		t.Fatalf("the member's widget through the link: %v", w)
	}
	if w := h.answer("pcx_admin2_widget", "/api/v1/pocket/widget"); w["line"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("someone without trips through the link: %v", w)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{"trips:read", req("GET", "/api/v1/trips"), true},
		{"trips:read", req("GET", "/api/v1/trips/3"), true},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02/more"), false},
		{core.ScopeWidget, req("GET", "/api/v1/home"), false},
		{core.ScopeSearch, req("GET", "/api/v1/trips"), false},
		{core.ScopeDay, req("GET", "/api/v1/trips/3"), false},
		// Travel keeps no quick lines
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), false},
		{"trips:read", req("GET", "/api/v1/pocket/search?q=x"), false},
		{"trips:read", req("GET", "/api/v1/pocket/day/2026-01-02"), false},
		{"trips:read", req("GET", "/api/v1/pocket/widget"), false},
		{"move:export", req("GET", "/api/v1/pocket/widget"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

func TestManifestCapabilities(t *testing.T) {
	h := newIsoHarness(t)
	h.srv.cfg.CoreRegistrationKey = "key"
	if err := h.srv.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	mp := h.manifest.Load()
	if mp == nil {
		t.Fatal("nothing was registered")
	}
	m := *mp
	if m["slug"] != "travel" || m["category"] != "Places" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	if search == nil || day == nil || day["label"] != "Travel" || day["one"] != "plan" || day["many"] != "plans" || widget == nil || widget["title"] != "Next trip" {
		t.Fatalf("capabilities: %v %v %v", search, day, widget)
	}
	if bangs, _ := json.Marshal(search["bangs"]); string(bangs) != `["trips","trip","travel"]` {
		t.Fatalf("bangs: %s", bangs)
	}
	if kinds, _ := json.Marshal(search["kinds"]); string(kinds) != `["trip","booking","stop"]` {
		t.Fatalf("kinds: %s", kinds)
	}
	if m["capture"] != nil {
		t.Fatalf("Travel keeps no quick lines: %v", m["capture"])
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"trips:read", "move:export", "move:import", core.ScopeSearch, core.ScopeDay, core.ScopeWidget} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
	if offered[core.ScopeCapture] {
		t.Error("pocket:capture is offered")
	}
}
