package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/config"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// locHarness: Travel with a direct Dawarich (from env, as on the box), a fake
// Pocket Movement and a fake Pocket Core whose Travel → Movement link state
// the test sets.
type locHarness struct {
	t      *testing.T
	ts     *httptest.Server
	srv    *Server
	tok    string
	tripID int64

	mu       sync.Mutex
	link     string   // on | off_admin | …
	mvKeys   []string // keys Movement saw
	dwHits   int
	mvTokens int
}

func newLocHarness(t *testing.T, withCore bool) *locHarness {
	t.Helper()
	h := &locHarness{t: t, link: "on"}
	day := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC).Unix()
	pts := func(lat float64) string {
		return `[{"latitude":"` + strconv.FormatFloat(lat, 'f', 4, 64) + `","longitude":"19.0","timestamp":` + strconv.FormatInt(day, 10) +
			`},{"latitude":"` + strconv.FormatFloat(lat+0.01, 'f', 4, 64) + `","longitude":"19.0","timestamp":` + strconv.FormatInt(day+300, 10) + `}]`
	}
	dw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.dwHits++
		h.mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/points":
			_, _ = w.Write([]byte(pts(42.0)))
		case "/api/v1/timeline":
			_, _ = w.Write([]byte(`{"days":[{"date":"2026-05-02","summary":{"total_distance":3},"entries":[]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dw.Close)
	mv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.mvKeys = append(h.mvKeys, r.URL.Query().Get("api_key"))
		h.mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/points":
			_, _ = w.Write([]byte(pts(45.0)))
		case "/api/v1/calendar":
			_, _ = w.Write([]byte(`[{"date":"2026-05-02","distance":25000,"countries":["ME","HR"]},{"date":"2026-05-03","distance":1000,"countries":["HR"]}]`))
		case "/api/v1/timeline":
			if r.URL.Query().Get("distance_unit") != "km" {
				http.Error(w, "unit", 400)
				return
			}
			_, _ = w.Write([]byte(`{"days":[{"date":"2026-05-02","summary":{"total_distance":25},"entries":[{"type":"visit","name":"Old Town","started_at":"2026-05-02T11:00:00Z","ended_at":"2026-05-02T12:00:00Z","duration":60,"distance":0}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mv.Close)
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		h.mu.Lock()
		defer h.mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/apps/register":
			_, _ = w.Write([]byte(`{"app":{}}`))
		case "/api/v1/links/status":
			if r.URL.Query().Get("app") != "movement" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			out := []map[string]any{}
			for _, sc := range movementScopes {
				out = append(out, map[string]any{"app": "movement", "scope": sc, "state": h.link, "on": h.link == "on"})
			}
			_ = json.NewEncoder(w).Encode(out)
		case "/api/v1/links/token":
			var in struct{ App string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.App != "movement" || h.link != "on" {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "link off", "reason": h.link})
				return
			}
			h.mvTokens++
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_mv", "expires_in": 300, "app": "movement", "url": mv.URL})
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(coreSrv.Close)
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: dir + "/t.db", FilesDir: dir + "/files", AuthMode: "local", DefaultCurrency: "EUR", FXProvider: "none",
		SessionTTL: time.Hour, MaxUploadBytes: 10 << 20, UserAgent: "test", DawarichURL: dw.URL, DawarichAPIKey: "env-key"}
	if withCore {
		cfg.CoreURL, cfg.CoreRegistrationKey = coreSrv.URL, "pcr_test"
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h.srv = New(cfg, st, log, nil)
	h.ts = httptest.NewServer(h.srv.Router())
	t.Cleanup(h.ts.Close)
	ctx := context.Background()
	u := &store.User{Email: "owner@example.com", Name: "owner", Role: "admin"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if h.tok, _, err = st.CreateAPIToken(ctx, u.ID, "test"); err != nil {
		t.Fatal(err)
	}
	// The env Dawarich is copied into the first person's settings, as on the box.
	if err := h.srv.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	code, body := h.do("POST", "/api/v1/trips", map[string]any{"name": "Coast", "start_date": "2026-05-02", "end_date": "2026-05-03", "timezone": "UTC"})
	if code != 201 && code != 200 {
		t.Fatalf("trip %d %s", code, body)
	}
	var tr struct{ ID int64 }
	_ = json.Unmarshal([]byte(body), &tr)
	h.tripID = tr.ID
	return h
}

func (h *locHarness) do(method, path string, body any) (int, string) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, h.ts.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+h.tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (h *locHarness) route() RouteResponse {
	h.t.Helper()
	code, body := h.do("GET", "/api/v1/trips/"+strconv.FormatInt(h.tripID, 10)+"/location/route", nil)
	if code != 200 {
		h.t.Fatalf("route %d %s", code, body)
	}
	var rr RouteResponse
	_ = json.Unmarshal([]byte(body), &rr)
	return rr
}

func (h *locHarness) settings() map[string]any {
	h.t.Helper()
	code, body := h.do("GET", "/api/v1/settings", nil)
	if code != 200 {
		h.t.Fatalf("settings %d %s", code, body)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(body), &m)
	return m
}

func (h *locHarness) waitRegistered() {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !h.srv.coreReady() {
		if time.Now().After(deadline) {
			h.t.Fatal("Travel did not register with Core")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// With Core and the link on, Pocket Movement is the default even though a
// Dawarich is configured; picking Dawarich switches back, per person.
func TestLocationMovementDefaultWithCore(t *testing.T) {
	h := newLocHarness(t, true)
	h.waitRegistered()
	st := h.settings()
	if st["location_effective"] != "movement" || st["location_source"] != "" || st["core_ready"] != true || st["movement_available"] != true {
		t.Fatalf("settings %v", st)
	}
	rr := h.route()
	if rr.Source != "movement" || rr.RawCount != 2 {
		t.Fatalf("route %+v", rr)
	}
	if rr.Days["2026-05-02"] != 25 || len(rr.Countries) != 2 || rr.Countries[0].Code != "ME" || rr.Countries[1].Days != 2 {
		t.Fatalf("movement summaries not used: %+v", rr)
	}
	if len(rr.ByDay) != 1 || strings.Join(rr.ByDay[0].Countries, ",") != "ME,HR" {
		t.Fatalf("days %+v", rr.ByDay)
	}
	code, body := h.do("GET", "/api/v1/trips/"+strconv.FormatInt(h.tripID, 10)+"/location/timeline", nil)
	if code != 200 || !strings.Contains(body, "Old Town") {
		t.Fatalf("timeline %d %s", code, body)
	}
	h.mu.Lock()
	for _, k := range h.mvKeys {
		if k != "pcx_mv" {
			t.Fatalf("Movement saw key %q, want the link token", k)
		}
	}
	h.mu.Unlock()

	// Picking Dawarich: the route comes from Dawarich, never the cached Movement one.
	if code, body := h.do("PUT", "/api/v1/settings", map[string]any{"location_source": "dawarich"}); code != 200 {
		t.Fatalf("pick %d %s", code, body)
	}
	if rr := h.route(); rr.Source != "dawarich" || len(rr.Countries) != 0 {
		t.Fatalf("after picking Dawarich %+v", rr)
	}
	if code, _ := h.do("PUT", "/api/v1/settings", map[string]any{"location_source": "nope"}); code != 400 {
		t.Fatalf("bad source accepted: %d", code)
	}
}

// Core refusing the link (switched off by the admin): Dawarich meanwhile,
// and Settings says which link.
func TestLocationMovementLinkOff(t *testing.T) {
	h := newLocHarness(t, true)
	h.waitRegistered()
	h.mu.Lock()
	h.link = "off_admin"
	h.mu.Unlock()
	if rr := h.route(); rr.Source != "dawarich" || rr.RawCount != 2 {
		t.Fatalf("route %+v", rr)
	}
	st := h.settings()
	if st["location_effective"] != "dawarich" || !strings.Contains(st["location_detail"].(string), "Travel → Pocket Movement link is switched off") {
		t.Fatalf("settings %v", st)
	}
	// Back on: Movement again (the refusal is not remembered past a fresh look).
	h.mu.Lock()
	h.link = "on"
	h.mu.Unlock()
	if st := h.settings(); st["location_effective"] != "movement" {
		t.Fatalf("after on %v", st)
	}
}

// Without Core: Dawarich exactly as before, Movement only shown as needing Core.
func TestLocationWithoutCore(t *testing.T) {
	h := newLocHarness(t, false)
	st := h.settings()
	if st["location_effective"] != "dawarich" || st["core_ready"] != false || st["movement_available"] != false {
		t.Fatalf("settings %v", st)
	}
	// A stored Movement choice does not break anything either.
	if code, body := h.do("PUT", "/api/v1/settings", map[string]any{"location_source": "movement"}); code != 200 {
		t.Fatalf("pick %d %s", code, body)
	}
	if rr := h.route(); rr.Source != "dawarich" || rr.RawCount != 2 {
		t.Fatalf("route %+v", rr)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.mvKeys) != 0 || h.mvTokens != 0 {
		t.Fatal("Movement reached without Core")
	}
}
