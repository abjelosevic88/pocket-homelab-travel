package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/config"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// isoHarness runs the real router with three people: the owner (first
// admin), a member and a second admin. Each authenticates with their own
// personal API token; the member also has a Pocket Core device token.
type isoHarness struct {
	t                       *testing.T
	ts                      *httptest.Server
	st                      *store.Store
	srv                     *Server
	owner, member, admin2   *store.User
	ownerTok, memberTok     string
	admin2Tok, memberDevice string
}

func newIsoHarness(t *testing.T) *isoHarness {
	t.Helper()
	// A fake Pocket Core that vouches for the member's paired phone.
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/introspect":
			var in struct {
				Token string `json:"token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Token == "pcx_member_trips" { // a linked app acting for the member
				_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "scope": "trips:read", "audience": "travel",
					"user": map[string]any{"id": 7, "email": "member@example.com", "name": "M", "role": "member"}, "app": map[string]any{"id": 3, "slug": "garage", "name": "Pocket Garage"}})
				return
			}
			if in.Token == "pcd_member_phone" {
				_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "device", "user": map[string]any{"id": 7, "email": "member@example.com", "name": "M", "role": "member"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	t.Cleanup(core.Close)
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: dir + "/t.db", FilesDir: dir + "/files", AuthMode: "local", DefaultCurrency: "EUR", FXProvider: "none",
		SessionTTL: time.Hour, MaxUploadBytes: 10 << 20, CoreURL: core.URL, UserAgent: "test"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := New(cfg, st, log, nil)
	if err := srv.core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	h := &isoHarness{t: t, ts: ts, st: st, srv: srv, memberDevice: "pcd_member_phone"}
	ctx := context.Background()
	mk := func(email, role string) (*store.User, string) {
		u := &store.User{Email: email, Name: strings.Split(email, "@")[0], Role: role}
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		tok, _, err := st.CreateAPIToken(ctx, u.ID, "test")
		if err != nil {
			t.Fatal(err)
		}
		return u, tok
	}
	h.owner, h.ownerTok = mk("owner@example.com", "admin")
	h.member, h.memberTok = mk("member@example.com", "member")
	h.admin2, h.admin2Tok = mk("admin2@example.com", "admin")
	return h
}

func (h *isoHarness) do(tok, method, path string, body any) (int, []byte) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.ts.URL+path, rd)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (h *isoHarness) create(tok, path string, body any) int64 {
	h.t.Helper()
	code, b := h.do(tok, http.MethodPost, path, body)
	if code != http.StatusCreated && code != http.StatusOK {
		h.t.Fatalf("POST %s: %d %s", path, code, b)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(b, &out)
	return out.ID
}

func (h *isoHarness) upload(tok, path string) int64 {
	h.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "ticket.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4 secret ticket"))
	_ = mw.WriteField("kind", "ticket")
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, h.ts.URL+path, &buf)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		h.t.Fatalf("upload: %d %s", res.StatusCode, b)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(b, &out)
	return out.ID
}

func listLen(t *testing.T, b []byte) int {
	t.Helper()
	var v []json.RawMessage
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not a list: %s", b)
	}
	return len(v)
}

// ownerTrip creates a trip full of the owner's content.
type ownerContent struct {
	trip, place, lodging, expense, list, item, journal, doc int64
}

func (h *isoHarness) seedOwner() ownerContent {
	var c ownerContent
	c.trip = h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Secret Lisbon", "destination": "Lisbon", "start_date": "2026-05-01", "end_date": "2026-05-05", "firefly_tag": "lisbon"})
	base := fmt.Sprintf("/api/v1/trips/%d", c.trip)
	c.place = h.create(h.ownerTok, base+"/places", map[string]any{"name": "Hidden bar", "category": "bar"})
	c.lodging = h.create(h.ownerTok, base+"/lodging", map[string]any{"name": "Hotel Owner", "check_in": "2026-05-01", "check_out": "2026-05-05"})
	c.expense = h.create(h.ownerTok, base+"/expenses", map[string]any{"name": "Dinner", "amount": 5000, "currency": "EUR", "kind": "actual", "date": "2026-05-02"})
	c.list = h.create(h.ownerTok, base+"/checklists", map[string]any{"name": "Owner list"})
	c.item = h.create(h.ownerTok, fmt.Sprintf("%s/checklists/%d/items", base, c.list), map[string]any{"text": "passport"})
	c.journal = h.create(h.ownerTok, base+"/journal", map[string]any{"title": "Day one", "body": "private thoughts", "date": "2026-05-01"})
	c.doc = h.upload(h.ownerTok, base+"/documents")
	return c
}

func TestPerUserIsolation(t *testing.T) {
	h := newIsoHarness(t)
	c := h.seedOwner()
	base := fmt.Sprintf("/api/v1/trips/%d", c.trip)

	// The owner sees everything of theirs.
	if code, b := h.do(h.ownerTok, "GET", "/api/v1/trips", nil); code != 200 || listLen(t, b) != 1 {
		t.Fatalf("owner list: %d %s", code, b)
	}

	// Everyone else — a member by API token, the same member by their Core
	// device token, and a second admin — sees nothing of it and gets 404, never 403.
	for name, tok := range map[string]string{"member": h.memberTok, "member device": h.memberDevice, "second admin": h.admin2Tok} {
		if code, b := h.do(tok, "GET", "/api/v1/trips", nil); code != 200 || listLen(t, b) != 0 {
			t.Errorf("%s list: %d %s", name, code, b)
		}
		if code, b := h.do(tok, "GET", "/api/v1/trips?all=1&q=Secret", nil); code != 200 || listLen(t, b) != 0 {
			t.Errorf("%s search/all: %d %s", name, code, b)
		}
		reads := []string{
			base, base + "/overview", base + "/activity", base + "/places", fmt.Sprintf("%s/places/%d", base, c.place),
			fmt.Sprintf("%s/places/%d/photos", base, c.place), base + "/lodging", base + "/transport", base + "/expenses", base + "/budget",
			base + "/checklists", base + "/journal", base + "/documents", fmt.Sprintf("%s/documents/%d/file", base, c.doc),
			base + "/cover", base + "/weather", base + "/dawarich/route", base + "/dawarich/timeline", base + "/photos",
			base + "/export/calendar.ics", base + "/export/trip.gpx?route=1", base + "/export/trip.json",
		}
		for _, p := range reads {
			if code, b := h.do(tok, "GET", p, nil); code != http.StatusNotFound {
				t.Errorf("%s GET %s: %d %s", name, p, code, b)
			}
		}
		writes := []struct{ method, path string }{
			{"PUT", base}, {"DELETE", base}, {"PUT", base + "/members"}, {"POST", base + "/duplicate"},
			{"POST", base + "/places"}, {"PUT", fmt.Sprintf("%s/places/%d", base, c.place)}, {"DELETE", fmt.Sprintf("%s/places/%d", base, c.place)},
			{"PUT", fmt.Sprintf("%s/lodging/%d", base, c.lodging)}, {"DELETE", fmt.Sprintf("%s/lodging/%d", base, c.lodging)},
			{"PUT", fmt.Sprintf("%s/expenses/%d", base, c.expense)}, {"DELETE", fmt.Sprintf("%s/expenses/%d", base, c.expense)},
			{"PUT", fmt.Sprintf("%s/checklists/items/%d", base, c.item)}, {"DELETE", fmt.Sprintf("%s/checklists/%d", base, c.list)},
			{"PUT", fmt.Sprintf("%s/journal/%d", base, c.journal)}, {"DELETE", fmt.Sprintf("%s/journal/%d", base, c.journal)},
			{"DELETE", fmt.Sprintf("%s/documents/%d", base, c.doc)}, {"POST", base + "/firefly/sync"}, {"POST", base + "/refresh"},
			{"POST", base + "/ai/recap"},
		}
		for _, wr := range writes {
			if code, b := h.do(tok, wr.method, wr.path, map[string]any{"name": "pwned", "text": "pwned", "title": "pwned", "members": []any{}}); code != http.StatusNotFound {
				t.Errorf("%s %s %s: %d %s", name, wr.method, wr.path, code, b)
			}
		}
		// Home and the Homepage widget count none of the owner's trips.
		if _, b := h.do(tok, "GET", "/api/v1/dashboard", nil); !strings.Contains(string(b), `"trips":0`) {
			t.Errorf("%s dashboard: %s", name, b)
		}
		if _, b := h.do(tok, "GET", "/api/v1/home", nil); strings.Contains(string(b), "Secret Lisbon") || strings.Contains(string(b), "private thoughts") {
			t.Errorf("%s home leaks: %s", name, b)
		}
	}
	// The calendar subscription feed with someone else's token.
	if code, _ := h.do("", "GET", fmt.Sprintf("/cal/%d.ics?token=%s", c.trip, h.memberTok), nil); code != http.StatusNotFound {
		t.Errorf("member calendar feed: %d", code)
	}
	if code, b := h.do("", "GET", fmt.Sprintf("/cal/%d.ics?token=%s", c.trip, h.ownerTok), nil); code != 200 || !strings.Contains(string(b), "BEGIN:VCALENDAR") {
		t.Errorf("owner calendar feed: %d", code)
	}

	// Child ids cannot be reached through a trip of your own either.
	mine := h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Member trip"})
	mbase := fmt.Sprintf("/api/v1/trips/%d", mine)
	for _, p := range []string{
		fmt.Sprintf("%s/places/%d", mbase, c.place), fmt.Sprintf("%s/places/%d/photos", mbase, c.place), fmt.Sprintf("%s/documents/%d/file", mbase, c.doc),
		fmt.Sprintf("%s/lodging/%d/image", mbase, c.lodging),
	} {
		if code, _ := h.do(h.memberTok, "GET", p, nil); code != http.StatusNotFound {
			t.Errorf("cross-trip GET %s: %d", p, code)
		}
	}
	for _, wr := range []struct{ method, path string }{
		{"PUT", fmt.Sprintf("%s/places/%d", mbase, c.place)}, {"DELETE", fmt.Sprintf("%s/places/%d", mbase, c.place)},
		{"PUT", fmt.Sprintf("%s/expenses/%d", mbase, c.expense)}, {"DELETE", fmt.Sprintf("%s/expenses/%d", mbase, c.expense)},
		{"PUT", fmt.Sprintf("%s/journal/%d", mbase, c.journal)}, {"DELETE", fmt.Sprintf("%s/journal/%d", mbase, c.journal)},
		{"PUT", fmt.Sprintf("%s/checklists/items/%d", mbase, c.item)}, {"DELETE", fmt.Sprintf("%s/checklists/items/%d", mbase, c.item)},
		{"DELETE", fmt.Sprintf("%s/documents/%d", mbase, c.doc)}, {"DELETE", fmt.Sprintf("%s/lodging/%d", mbase, c.lodging)},
	} {
		if code, b := h.do(h.memberTok, wr.method, wr.path, map[string]any{"name": "pwned", "text": "pwned", "title": "pwned"}); code != http.StatusNotFound {
			t.Errorf("cross-trip %s %s: %d %s", wr.method, wr.path, code, b)
		}
	}

	// Nothing of the owner's changed.
	ctx := context.Background()
	if p, err := h.st.GetPlace(ctx, c.trip, c.place); err != nil || p.Name != "Hidden bar" {
		t.Errorf("owner place changed: %v %v", p, err)
	}
	if j, err := h.st.GetJournal(ctx, c.trip, c.journal); err != nil || j.Body != "private thoughts" {
		t.Errorf("owner journal changed: %v %v", j, err)
	}
	if tr, err := h.st.GetTrip(ctx, c.trip, h.owner.ID); err != nil || tr.Name != "Secret Lisbon" || tr.OwnerID != h.owner.ID {
		t.Errorf("owner trip changed: %v %v", tr, err)
	}
	// The member's own trip is theirs, whatever the body says.
	if tr, _ := h.st.GetTrip(ctx, mine, h.member.ID); tr.OwnerID != h.member.ID {
		t.Errorf("member trip owner = %d", tr.OwnerID)
	}
	forged := h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Forged", "owner_id": h.owner.ID})
	if tr, _ := h.st.GetTrip(ctx, forged, h.member.ID); tr.OwnerID != h.member.ID {
		t.Errorf("owner_id from the body was honoured: %d", tr.OwnerID)
	}

	// Box administration is admin-only; a member cannot download the database.
	if code, _ := h.do(h.memberTok, "GET", "/api/v1/backup", nil); code != http.StatusForbidden {
		t.Errorf("member backup: %d", code)
	}
	if code, _ := h.do(h.memberTok, "GET", "/api/v1/settings/system", nil); code != http.StatusForbidden {
		t.Errorf("member system info: %d", code)
	}
}

func TestPerUserSettingsAndIntegrations(t *testing.T) {
	h := newIsoHarness(t)
	ctx := context.Background()
	// The owner's integrations, as the migration leaves them.
	own, _ := h.st.GetSettings(ctx, h.owner.ID)
	own.FireflyURL, own.FireflyToken = "http://firefly.invalid", "owner-firefly-token-123456"
	own.DawarichURL, own.DawarichAPIKey = "http://dawarich.invalid", "owner-dawarich-key-123456"
	own.ImmichURL, own.ImmichAPIKey = "http://immich.invalid", "owner-immich-key-123456"
	own.OpenAIKey = "sk-owner-openai-key-123456"
	own.HomeCurrency = "BAM"
	if err := h.st.SaveUserSettings(ctx, h.owner.ID, own); err != nil {
		t.Fatal(err)
	}

	// The member sees none of the owner's credentials or preferences.
	for _, tok := range []string{h.memberTok, h.memberDevice, h.admin2Tok} {
		_, b := h.do(tok, "GET", "/api/v1/settings", nil)
		var st map[string]any
		_ = json.Unmarshal(b, &st)
		for _, k := range []string{"firefly_configured", "dawarich_configured", "immich_configured", "openai_configured"} {
			if st[k] != false {
				t.Errorf("%s = %v for someone else", k, st[k])
			}
		}
		if strings.Contains(string(b), "invalid") || strings.Contains(string(b), "BAM") {
			t.Errorf("settings leak: %s", b)
		}
		// Testing a connection with a masked key does not borrow the owner's.
		_, b = h.do(tok, "POST", "/api/v1/integrations/test", map[string]any{"which": "openai", "url": "http://127.0.0.1:1", "key": "sk-o••••3456"})
		if !strings.Contains(string(b), `"ok":false`) {
			t.Errorf("integration test: %s", b)
		}
	}
	// The owner still sees theirs.
	_, b := h.do(h.ownerTok, "GET", "/api/v1/settings", nil)
	if !strings.Contains(string(b), `"firefly_configured":true`) || !strings.Contains(string(b), `"home_currency":"BAM"`) || strings.Contains(string(b), "owner-firefly-token-123456") {
		t.Errorf("owner settings: %s", b)
	}

	// A member saves their own preferences without touching the owner's,
	// and cannot change the box-wide title.
	if code, b := h.do(h.memberTok, "PUT", "/api/v1/settings", map[string]any{"home_currency": "USD", "distance_unit": "mi", "dawarich_url": "http://mine.invalid", "dawarich_api_key": "member-key-abcdef"}); code != 200 {
		t.Fatalf("member save: %d %s", code, b)
	}
	if code, _ := h.do(h.memberTok, "PUT", "/api/v1/settings", map[string]any{"title": "Hacked"}); code != http.StatusForbidden {
		t.Errorf("member title change: %d", code)
	}
	o, _ := h.st.GetSettings(ctx, h.owner.ID)
	m, _ := h.st.GetSettings(ctx, h.member.ID)
	if o.HomeCurrency != "BAM" || o.DistanceUnit != "km" || o.DawarichAPIKey != "owner-dawarich-key-123456" || o.Title != store.DefaultTitle {
		t.Errorf("owner settings changed: %+v", o)
	}
	if m.HomeCurrency != "USD" || m.DistanceUnit != "mi" || m.DawarichAPIKey != "member-key-abcdef" || m.FireflyToken != "" || m.OpenAIKey != "" {
		t.Errorf("member settings: %+v", m)
	}
	// New trips default to the creator's own home currency.
	id := h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Mine"})
	if tr, _ := h.st.GetTrip(ctx, id, h.member.ID); tr.Currency != "USD" {
		t.Errorf("member trip currency %s", tr.Currency)
	}
	// AI uses the caller's own key: the member has none.
	if code, b := h.do(h.memberTok, "POST", fmt.Sprintf("/api/v1/trips/%d/ai/recap", id), map[string]any{}); code != http.StatusBadRequest || !strings.Contains(string(b), "OpenAI") {
		t.Errorf("member AI: %d %s", code, b)
	}

	// Sharing: the owner adds the member as a viewer of one trip.
	trip := h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Shared", "start_date": "2026-05-01", "end_date": "2026-05-03", "firefly_tag": "shared"})
	base := fmt.Sprintf("/api/v1/trips/%d", trip)
	// The owner's recorded route sits in the owner's cache.
	_ = h.st.CachePut(ctx, userCacheKey(trip, h.owner.ID, "dawarich-route"), `{"points":[[38.7,-9.1,1]],"raw_count":1,"distance_km":42,"days_km":{},"days":[]}`)
	_ = h.st.CachePut(ctx, userCacheKey(trip, h.owner.ID, "immich-photos"), `[{"id":"owner-photo"}]`)
	if code, b := h.do(h.ownerTok, "PUT", base+"/members", map[string]any{"members": []any{map[string]any{"user_id": h.member.ID, "role": "viewer"}}}); code != 200 {
		t.Fatalf("share: %d %s", code, b)
	}
	if code, b := h.do(h.memberTok, "GET", base, nil); code != 200 || !strings.Contains(string(b), `"role":"viewer"`) {
		t.Fatalf("shared trip: %d %s", code, b)
	}
	if code, _ := h.do(h.memberTok, "PUT", base, map[string]any{"name": "x"}); code != http.StatusForbidden {
		t.Errorf("viewer edit: %d", code)
	}
	// The member's route/photos come from their own accounts, never the owner's cache.
	if _, b := h.do(h.memberTok, "GET", base+"/photos", nil); strings.Contains(string(b), "owner-photo") {
		t.Errorf("owner photos leaked: %s", b)
	}
	if _, b := h.do(h.memberTok, "GET", base+"/export/trip.gpx?route=1", nil); strings.Contains(string(b), "<trkpt") {
		t.Errorf("owner route leaked in GPX: %s", b)
	}
	_, b = h.do(h.memberTok, "GET", base+"/overview", nil)
	var ov struct {
		Integrations map[string]map[string]any `json:"integrations"`
	}
	_ = json.Unmarshal(b, &ov)
	if ov.Integrations["immich"]["configured"] != false || ov.Integrations["ai"]["configured"] != false {
		t.Errorf("member overview borrows the owner's accounts: %v", ov.Integrations)
	}
	if _, b := h.do(h.memberTok, "GET", "/api/v1/home", nil); strings.Contains(string(b), `"recorded_km":42`) || strings.Contains(string(b), `"photos":1`) {
		t.Errorf("member home counts the owner's route/photos: %s", b)
	}
	if _, b := h.do(h.ownerTok, "GET", "/api/v1/home", nil); !strings.Contains(string(b), `"recorded_km":42`) {
		t.Errorf("owner home lost their route: %s", b)
	}
	// An editor may plan but not repoint the Firefly tag at the owner's other spending.
	_, _ = h.do(h.ownerTok, "PUT", base+"/members", map[string]any{"members": []any{map[string]any{"user_id": h.member.ID, "role": "editor"}}})
	if code, _ := h.do(h.memberTok, "PUT", base, map[string]any{"firefly_tag": "salary"}); code != http.StatusForbidden {
		t.Errorf("editor changed the firefly tag: %d", code)
	}
	if code, b := h.do(h.memberTok, "PUT", base, map[string]any{"name": "Shared!", "firefly_tag": "shared"}); code != 200 {
		t.Errorf("editor edit: %d %s", code, b)
	}
	// Only the owner shares or deletes.
	if code, _ := h.do(h.memberTok, "DELETE", base, nil); code != http.StatusForbidden {
		t.Errorf("editor delete: %d", code)
	}
	// The member's own Firefly tags come from their (absent) account.
	if _, b := h.do(h.memberTok, "GET", "/api/v1/firefly/tags", nil); listLen(t, b) != 0 {
		t.Errorf("tags: %s", b)
	}
}

// TestMigrationBackfill runs 0003 against a database shaped like the live one
// (one admin, box-wide integration settings) and checks the owner keeps them.
func TestMigrationBackfill(t *testing.T) {
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(dir+"/t.db", log)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u := &store.User{Email: "owner@example.com", Role: "admin"}
	_ = st.CreateUser(ctx, u)
	// Roll back to the pre-0003 shape: settings box-wide, no user_settings rows.
	if _, err := st.DB.Exec(`DELETE FROM schema_migrations WHERE name = '0003_per_user.sql'; DROP TABLE user_settings;
		INSERT INTO settings (key, value) VALUES ('title','My Travel'),('firefly_token','tok'),('dawarich_api_key','dk'),('home_currency','BAM'),('fx_auto','0'),('setup_complete','1');
		INSERT INTO cache (key, value, fetched_at) VALUES ('trip:1:dawarich-route','{}','x'),('trip:1:weather:1','{}','x')`); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = store.Open(dir+"/t.db", log) // applies 0003 again
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	o, _ := st.GetSettings(ctx, u.ID)
	if o.FireflyToken != "tok" || o.DawarichAPIKey != "dk" || o.HomeCurrency != "BAM" || o.Title != "My Travel" || o.FXAuto || !o.SetupComplete {
		t.Errorf("owner after migration: %+v", o)
	}
	g, _ := st.GetSettings(ctx, 0)
	if g.FireflyToken != "" || g.DawarichAPIKey != "" || g.Title != "My Travel" {
		t.Errorf("box-wide after migration: %+v", g)
	}
	var n int
	_ = st.DB.QueryRow(`SELECT COUNT(1) FROM settings WHERE key NOT IN ('title','setup_complete','fx_auto')`).Scan(&n)
	if n != 0 {
		t.Errorf("%d personal keys left box-wide", n)
	}
	if _, _, ok := st.CacheGet(ctx, "trip:1:dawarich-route"); ok {
		t.Error("trip-wide route cache survived")
	}
	if _, _, ok := st.CacheGet(ctx, "trip:1:weather:1"); !ok {
		t.Error("weather cache dropped")
	}
}
