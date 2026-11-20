package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// A person's whole Travel in one file, for Pocket Move and for keeping a
// copy: every trip they made, each in the shape of the one-trip export.
// Trips others share with them stay with their owners.

// TravelExport is the whole-person export.
type TravelExport struct {
	Format     string       `json:"format"`
	Version    string       `json:"version"`
	ExportedAt string       `json:"exported_at"`
	Trips      []TripExport `json:"trips"`
}

func (s *Server) handleExportAll(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	ctx := r.Context()
	trips, err := s.store.ListTrips(ctx, store.TripFilter{UserID: u.ID, Owned: true})
	if err != nil {
		s.fail(w, err, "trips")
		return
	}
	out := TravelExport{Format: "pocket-travel", Version: Version, ExportedAt: store.Now(), Trips: []TripExport{}}
	for i := range trips {
		t := trips[i]
		t.Role = ""
		ex := TripExport{Format: "pocket-travel-trip", Version: Version, ExportedAt: out.ExportedAt, Trip: &t}
		ex.Places, _ = s.store.ListPlaces(ctx, t.ID)
		ex.Lodging, _ = s.store.ListLodging(ctx, t.ID)
		ex.Transport, _ = s.store.ListTransport(ctx, t.ID)
		ex.Expenses, _ = s.store.ListExpenses(ctx, t.ID)
		ex.Checklists, _ = s.store.ListChecklists(ctx, t.ID)
		ex.Journal, _ = s.store.ListJournal(ctx, t.ID)
		out.Trips = append(out.Trips, ex)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-travel.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

// handleImportAll takes the whole-person export (or one trip's export) and
// adds the trips the person does not have yet. A trip counts as already
// there when one of theirs has the same name and dates, or the same origin
// (external id). It never changes or deletes anything.
func (s *Server) handleImportAll(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	ctx := r.Context()
	var raw struct {
		Trips []TripExport    `json:"trips"`
		Trip  json.RawMessage `json:"trip"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the file is too large")
		return
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		writeErr(w, http.StatusBadRequest, "not a Pocket Travel export")
		return
	}
	trips := raw.Trips
	if len(raw.Trip) > 0 && string(raw.Trip) != "null" {
		var one TripExport
		if err := json.Unmarshal(body, &one); err != nil || one.Trip == nil {
			writeErr(w, http.StatusBadRequest, "not a Pocket Travel export")
			return
		}
		trips = append(trips, one)
	}
	if raw.Trips == nil && len(trips) == 0 {
		writeErr(w, http.StatusBadRequest, "not a Pocket Travel export")
		return
	}

	have, err := s.store.ListTrips(ctx, store.TripFilter{UserID: u.ID, Owned: true})
	if err != nil {
		s.fail(w, err, "trips")
		return
	}
	seen := map[string]bool{}
	mark := func(t *store.Trip) {
		seen[sameTripKey(t)] = true
		if t.ExternalID != "" {
			seen["ext\x00"+t.ExternalID] = true
		}
	}
	for i := range have {
		mark(&have[i])
	}
	imported, skipped := 0, 0
	for _, ex := range trips {
		if ex.Trip == nil || strings.TrimSpace(ex.Trip.Name) == "" {
			skipped++
			continue
		}
		if seen[sameTripKey(ex.Trip)] || (ex.Trip.ExternalID != "" && seen["ext\x00"+ex.Trip.ExternalID]) {
			skipped++
			continue
		}
		t, err := s.importTrip(ctx, u, ex, true)
		if err != nil {
			s.fail(w, err, "create trip")
			return
		}
		mark(t)
		imported++
	}
	writeJSON(w, http.StatusOK, map[string]any{"format": "pocket-travel", "imported": imported, "trips": imported, "skipped": skipped})
}

// sameTripKey is what makes two trips the same trip: name and dates.
func sameTripKey(t *store.Trip) string {
	return strings.ToLower(strings.TrimSpace(t.Name)) + "\x00" + t.StartDate + "\x00" + t.EndDate
}
