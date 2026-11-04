package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func TestSearch(t *testing.T) {
	h := newIsoHarness(t)
	pt := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Portugal 2025", "destination": "Lisboa", "start_date": "2025-07-01", "end_date": "2025-07-14", "currency": "EUR"})
	h.create(h.ownerTok, fmt.Sprintf("/api/v1/trips/%d/places", pt), map[string]any{"name": "Açores", "category": "nature"})
	h.create(h.ownerTok, fmt.Sprintf("/api/v1/trips/%d/lodging", pt), map[string]any{"name": "Casa São Miguel", "check_in": "2025-07-05", "check_out": "2025-07-09"})
	ct := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Čačak winter", "destination": "Serbia", "start_date": "2026-01-10", "end_date": "2026-01-12", "currency": "EUR"})
	// someone else's trip never shows up
	h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Azores secret", "start_date": "2025-07-01", "end_date": "2025-07-02", "currency": "EUR"})

	type res struct {
		Trips []struct {
			ID     int64 `json:"id"`
			Direct bool  `json:"direct"`
			Hits   []struct {
				Kind     string `json:"kind"`
				Name     string `json:"name"`
				TripName string `json:"trip_name"`
			} `json:"hits"`
		} `json:"trips"`
		Places []struct {
			Name     string `json:"name"`
			TripName string `json:"trip_name"`
		} `json:"places"`
		Stays []struct {
			Name string `json:"name"`
		} `json:"stays"`
	}
	get := func(q string) res {
		t.Helper()
		code, b := h.do(h.ownerTok, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(q), nil)
		if code != http.StatusOK {
			t.Fatalf("search %q: %d %s", q, code, b)
		}
		var r res
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	// accents are ignored, and the place says which trip it is in
	r := get("acores")
	if len(r.Places) != 1 || r.Places[0].Name != "Açores" || r.Places[0].TripName != "Portugal 2025" {
		t.Fatalf("acores places = %+v", r.Places)
	}
	if len(r.Trips) != 1 || r.Trips[0].ID != pt || r.Trips[0].Direct || len(r.Trips[0].Hits) != 1 {
		t.Fatalf("acores trips = %+v", r.Trips)
	}
	if r := get("cacak"); len(r.Trips) != 1 || r.Trips[0].ID != ct || !r.Trips[0].Direct {
		t.Fatalf("cacak = %+v", r.Trips)
	}
	if r := get("sao miguel"); len(r.Stays) != 1 {
		t.Fatalf("stay search = %+v", r.Stays)
	}
	// "portugal" finds the trip, not each of its places
	if r := get("portugal"); len(r.Trips) != 1 || !r.Trips[0].Direct || len(r.Places) != 0 {
		t.Fatalf("portugal = %+v", r)
	}
	// words split over trip and place
	if r := get("portugal açores"); len(r.Places) != 1 {
		t.Fatalf("portugal açores = %+v", r.Places)
	}
	// dates
	if r := get("summer 2025"); len(r.Trips) != 1 || r.Trips[0].ID != pt {
		t.Fatalf("summer 2025 = %+v", r.Trips)
	}
	if r := get("winter 2025"); len(r.Trips) != 1 || r.Trips[0].ID != ct {
		t.Fatalf("winter 2025 = %+v", r.Trips)
	}
	if r := get("2026"); len(r.Trips) != 1 || r.Trips[0].ID != ct {
		t.Fatalf("2026 = %+v", r.Trips)
	}
	if r := get("july"); len(r.Trips) != 1 || r.Trips[0].ID != pt {
		t.Fatalf("july = %+v", r.Trips)
	}
	if r := get("acores 2024"); len(r.Trips) != 0 {
		t.Fatalf("acores 2024 = %+v", r.Trips)
	}
	// a month word that is really a name falls back to text
	if r := get("winter"); len(r.Trips) != 1 || r.Trips[0].ID != ct {
		t.Fatalf("winter = %+v", r.Trips)
	}
}

func TestStayTimesAndCoordinates(t *testing.T) {
	h := newIsoHarness(t)
	tr := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Azores", "start_date": "2025-07-01", "end_date": "2025-07-14", "currency": "EUR"})
	base := fmt.Sprintf("/api/v1/trips/%d/lodging", tr)
	// a rental with only GPS coordinates and times
	code, b := h.do(h.ownerTok, http.MethodPost, base, map[string]any{"name": "Airbnb Sete Cidades", "lat": 37.8569, "lng": -25.7903, "check_in": "2025-07-05T16:30", "check_out": "2025-07-09T10:00"})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create: %d %s", code, b)
	}
	var l struct {
		CheckIn string   `json:"check_in"`
		Lat     *float64 `json:"lat"`
	}
	_ = json.Unmarshal(b, &l)
	if l.CheckIn != "2025-07-05T16:30" || l.Lat == nil {
		t.Fatalf("stay = %s", b)
	}
	for _, bad := range []map[string]any{
		{"name": "x", "check_in": "2025-07-05T25:00"},
		{"name": "x", "check_in": "05/07/2025"},
		{"name": "x", "check_in": "2025-07-05T15:00", "check_out": "2025-07-05T11:00"},
	} {
		if code, b := h.do(h.ownerTok, http.MethodPost, base, bad); code != http.StatusBadRequest {
			t.Fatalf("%v: %d %s", bad, code, b)
		}
	}
	// defaults are per person
	if code, b := h.do(h.ownerTok, http.MethodPut, "/api/v1/settings", map[string]any{"checkin_time": "14:00", "checkout_time": "10:30"}); code != http.StatusOK {
		t.Fatalf("settings: %d %s", code, b)
	}
	if code, _ := h.do(h.ownerTok, http.MethodPut, "/api/v1/settings", map[string]any{"checkin_time": "2pm"}); code != http.StatusBadRequest {
		t.Fatalf("bad time accepted: %d", code)
	}
	_, b = h.do(h.ownerTok, http.MethodGet, "/api/v1/settings", nil)
	var st struct {
		CheckinTime  string `json:"checkin_time"`
		CheckoutTime string `json:"checkout_time"`
	}
	_ = json.Unmarshal(b, &st)
	if st.CheckinTime != "14:00" || st.CheckoutTime != "10:30" {
		t.Fatalf("settings = %s", b)
	}
	_, b = h.do(h.memberTok, http.MethodGet, "/api/v1/settings", nil)
	_ = json.Unmarshal(b, &st)
	if st.CheckinTime != "15:00" {
		t.Fatalf("member default = %s", st.CheckinTime)
	}
}
