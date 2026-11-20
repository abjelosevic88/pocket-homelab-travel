package server

import (
	"encoding/json"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// Pocket Move links (pcx_<who>_export / pcx_<who>_import) reach exactly the
// Portable routes and nothing else.
func TestMoveLinkScopes(t *testing.T) {
	h := newIsoHarness(t)
	trip := itoa(h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Member Rome", "start_date": "2026-06-01", "end_date": "2026-06-04"}))
	exp, imp := "pcx_member_export", "pcx_member_import"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "POST", "/api/v1/import/trip", 403},
		{exp, "GET", "/api/v1/trips", 403},
		{exp, "GET", "/api/v1/trips/" + trip + "/export/trip.json", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{exp, "GET", "/api/v1/tokens", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "POST", "/api/v1/trips", 403},
		{imp, "POST", "/api/v1/import/adventurelog", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{"pcx_member_trips", "GET", core.PortablePath, 403},
		{"pcx_member_trips", "GET", "/api/v1/export.json", 403},
	} {
		if code, body := h.do(c.tok, c.method, c.path, map[string]any{}); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := h.do(exp, "GET", core.PortablePath, nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 1 || p.Exports[0].Kind != core.KindPocket || len(p.Imports) != 1 || p.Imports[0].Form != "" {
		t.Fatalf("portable: %s", body)
	}
}

// The owner's trips move to the member: the export holds only the owner's
// own trips, the import adds them once, and nobody else's trips change.
func TestMoveTravelAddsOnce(t *testing.T) {
	h := newIsoHarness(t)
	lisbon := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Owner Lisbon", "start_date": "2026-05-01", "end_date": "2026-05-05", "currency": "EUR"})
	h.create(h.ownerTok, "/api/v1/trips/"+itoa(lisbon)+"/places", map[string]any{"name": "Belém Tower"})
	h.create(h.ownerTok, "/api/v1/trips/"+itoa(lisbon)+"/expenses", map[string]any{"name": "Pastéis", "amount": 650, "currency": "EUR", "category": "food", "kind": "actual"})
	h.create(h.ownerTok, "/api/v1/trips/"+itoa(lisbon)+"/checklists", map[string]any{"name": "Pack", "items": []string{"Sunscreen"}})
	rome := h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Member Rome", "start_date": "2026-06-01", "end_date": "2026-06-04"})
	// Rome is shared with the owner: it stays the member's.
	if code, b := h.do(h.memberTok, "PUT", "/api/v1/trips/"+itoa(rome)+"/members", map[string]any{"members": []map[string]any{{"user_id": h.owner.ID, "role": "editor"}}}); code != 200 {
		t.Fatalf("share: %d %s", code, b)
	}

	code, export := h.do("pcx_owner_export", "GET", "/api/v1/export.json", nil)
	if code != 200 || !strings.Contains(string(export), "Belém Tower") || !strings.Contains(string(export), "Sunscreen") || strings.Contains(string(export), "Member Rome") {
		t.Fatalf("owner's export: %d %s", code, export)
	}
	if _, mine := h.do("pcx_member_export", "GET", "/api/v1/export.json", nil); strings.Contains(string(mine), "Lisbon") || !strings.Contains(string(mine), "Member Rome") {
		t.Fatalf("member's export: %s", mine)
	}

	for i, want := range []float64{1, 0} {
		code, b := h.do("pcx_member_import", "POST", "/api/v1/import", json.RawMessage(export))
		var out map[string]any
		_ = json.Unmarshal(b, &out)
		if code != 200 || out["imported"] != want || out["skipped"] != 1-want {
			t.Fatalf("import %d: %d %s", i+1, code, b)
		}
	}
	// One trip's own file goes the same way, and is already there too.
	_, one := h.do(h.ownerTok, "GET", "/api/v1/trips/"+itoa(lisbon)+"/export/trip.json", nil)
	if code, b := h.do("pcx_member_import", "POST", "/api/v1/import", json.RawMessage(one)); code != 200 || !strings.Contains(string(b), `"imported":0`) {
		t.Fatalf("one trip again: %d %s", code, b)
	}
	if code, _ := h.do("pcx_member_import", "POST", "/api/v1/import", map[string]any{"hello": 1}); code != 400 {
		t.Fatalf("not an export: %d", code)
	}

	count := func(u *store.User) (all []store.Trip) {
		all, _ = h.st.ListTrips(t.Context(), store.TripFilter{UserID: u.ID, Owned: true})
		return
	}
	got := count(h.member)
	if len(got) != 2 {
		t.Fatalf("member owns %d trips, want 2", len(got))
	}
	for _, tr := range got {
		if tr.Name == "Owner Lisbon" {
			if p, _ := h.st.ListPlaces(t.Context(), tr.ID); len(p) != 1 {
				t.Fatalf("places: %v", p)
			}
			src, _ := h.st.ListChecklists(t.Context(), lisbon)
			if c, _ := h.st.ListChecklists(t.Context(), tr.ID); len(c) != len(src) || len(c[len(c)-1].Items) != 1 {
				t.Fatalf("checklists: %v", c)
			}
		}
	}
	if n := len(count(h.owner)); n != 1 {
		t.Fatalf("owner owns %d trips, want 1", n)
	}
	if n := len(count(h.admin2)); n != 0 {
		t.Fatalf("admin2 owns %d trips", n)
	}
}
