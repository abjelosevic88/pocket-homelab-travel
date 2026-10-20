package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/config"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// fakeTx is one withdrawal the fake Pocket Money serves.
type fakeTx struct {
	id, date, desc, amount, cur, ext string
}

// moneyHarness: Travel with a fake Pocket Core (app links) and a fake Pocket
// Money serving Firefly-shaped JSON under /firefly.
type moneyHarness struct {
	t        *testing.T
	ts       *httptest.Server
	st       *store.Store
	srv      *Server
	owner    *store.User
	ownerTok string
	money    *httptest.Server

	mu         sync.Mutex
	linkOn     bool
	origin     string // original_source on every transaction
	txs        []fakeTx
	moneyAuth  []string // Authorization headers Money saw
	moneyPaths []string
	actAs      []map[string]any // token requests Core saw
}

func newMoneyHarness(t *testing.T, env func(*config.Config)) *moneyHarness {
	t.Helper()
	h := &moneyHarness{t: t, linkOn: true, origin: "pocket-money-test"}
	h.money = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.moneyAuth = append(h.moneyAuth, r.Header.Get("Authorization"))
		h.moneyPaths = append(h.moneyPaths, r.URL.Path)
		txs, origin := append([]fakeTx(nil), h.txs...), h.origin
		h.mu.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(401)
			return
		}
		page := map[string]any{"pagination": map[string]any{"total_pages": 1}}
		switch r.URL.Path {
		case "/firefly/api/v1/about":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"version": "6.7.9", "php_version": "pocket-money 0.5.0"}})
		case "/firefly/api/v1/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
				map[string]any{"id": "7", "attributes": map[string]any{"tag": "Lisbon 2026"}},
				map[string]any{"id": "8", "attributes": map[string]any{"tag": "other"}},
			}, "meta": page})
		case "/firefly/api/v1/tags/7/transactions":
			var data []any
			for _, x := range txs {
				var ext any
				if x.ext != "" {
					ext = x.ext
				}
				data = append(data, map[string]any{"id": x.id, "attributes": map[string]any{"transactions": []any{map[string]any{
					"transaction_journal_id": x.id, "type": "withdrawal", "date": x.date + "T12:00:00+02:00", "description": x.desc,
					"amount": x.amount, "currency_code": x.cur, "currency_decimal_places": 2, "foreign_amount": nil, "foreign_currency_code": nil,
					"category_name": nil, "destination_name": "Shop", "notes": nil, "tags": []string{"Lisbon 2026"},
					"external_id": ext, "original_source": origin,
				}}}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": page})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(h.money.Close)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		on := h.linkOn
		h.mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/links/token":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			h.mu.Lock()
			h.actAs = append(h.actAs, in)
			h.mu.Unlock()
			if !on {
				w.WriteHeader(403)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "link off", "reason": "off_person"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_money_read", "expires_in": 300, "scope": "transactions:read", "app": "money", "url": h.money.URL})
		case "/api/v1/links/status":
			st := "on"
			if !on {
				st = "off_person"
			}
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"app": "money", "scope": "transactions:read", "state": st, "on": on}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	t.Cleanup(core.Close)
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: dir + "/t.db", FilesDir: dir + "/files", AuthMode: "local", DefaultCurrency: "EUR", FXProvider: "none",
		SessionTTL: time.Hour, MaxUploadBytes: 10 << 20, CoreURL: core.URL, UserAgent: "test"}
	if env != nil {
		env(cfg)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h.st = st
	h.srv = New(cfg, st, log, nil)
	h.ts = httptest.NewServer(h.srv.Router())
	t.Cleanup(h.ts.Close)
	ctx := context.Background()
	h.owner = &store.User{Email: "owner@example.com", Name: "owner", Role: "admin"}
	if err := st.CreateUser(ctx, h.owner); err != nil {
		t.Fatal(err)
	}
	if h.ownerTok, _, err = st.CreateAPIToken(ctx, h.owner.ID, "test"); err != nil {
		t.Fatal(err)
	}
	h.txs = []fakeTx{
		{id: "9001", date: "2026-05-02", desc: "Pastéis de Belém", amount: "12.40", cur: "EUR"},
		{id: "9002", date: "2026-05-03", desc: "Tram 28", amount: "6.00", cur: "EUR"},
	}
	return h
}

func (h *moneyHarness) do(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.ts.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+h.ownerTok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	b, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out
}

func (h *moneyHarness) trip() int64 {
	h.t.Helper()
	code, out := h.do("POST", "/api/v1/trips", map[string]any{"name": "Lisbon", "start_date": "2026-05-01", "end_date": "2026-05-05", "currency": "EUR", "firefly_tag": "lisbon 2026"})
	if code != 201 {
		h.t.Fatalf("create trip %d %v", code, out)
	}
	time.Sleep(50 * time.Millisecond) // a create with a tag may not sync, but let any goroutine settle
	return int64(out["id"].(float64))
}

func (h *moneyHarness) synced(tripID int64) []store.Expense {
	h.t.Helper()
	all, err := h.st.ListExpenses(context.Background(), tripID)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []store.Expense
	for _, e := range all {
		if store.IsSynced(e.Source) {
			out = append(out, e)
		}
	}
	return out
}

func (h *moneyHarness) reset() {
	h.mu.Lock()
	h.moneyAuth, h.moneyPaths, h.actAs = nil, nil, nil
	h.mu.Unlock()
}

// No token saved and Travel connected to Core: Pocket Core mode, a fresh
// pcx_ token for every sync, sent to <money>/firefly.
func TestMoneyCoreMode(t *testing.T) {
	h := newMoneyHarness(t, nil)
	_, st := h.do("GET", "/api/v1/settings", nil)
	if st["money_mode"] != "core" || st["money_via"] != "core" || st["firefly_configured"] != true {
		t.Fatalf("settings %v", st)
	}
	id := h.trip()
	h.reset()
	for i := 0; i < 2; i++ {
		code, out := h.do("POST", "/api/v1/trips/"+itoa(id)+"/money/sync", nil)
		if code != 200 || out["synced"].(float64) != 2 {
			t.Fatalf("sync %d %v", code, out)
		}
	}
	rows := h.synced(id)
	if len(rows) != 2 || rows[0].Source != "money" {
		t.Fatalf("rows %+v", rows)
	}
	h.mu.Lock()
	if len(h.actAs) != 2 {
		t.Fatalf("want a token per sync, Core saw %d", len(h.actAs))
	}
	if h.actAs[0]["user"] != "owner@example.com" || h.actAs[0]["app"] != "money" {
		t.Fatalf("act-as %v", h.actAs[0])
	}
	if sc, _ := h.actAs[0]["scopes"].([]any); len(sc) != 1 || sc[0] != "transactions:read" {
		t.Fatalf("scopes %v", h.actAs[0]["scopes"])
	}
	for i, a := range h.moneyAuth {
		if a != "Bearer pcx_money_read" || !strings.HasPrefix(h.moneyPaths[i], "/firefly/api/v1/") {
			t.Fatalf("money call %s %s", h.moneyPaths[i], a)
		}
	}
	h.mu.Unlock()
	// the connection test goes the same way
	_, res := h.do("POST", "/api/v1/integrations/test", map[string]any{"which": "money"})
	if res["ok"] != true || !strings.Contains(res["message"].(string), "Pocket Money 0.5.0") || !strings.Contains(res["message"].(string), "Pocket Core") {
		t.Fatalf("test %v", res)
	}
}

// A saved token means direct (the owner's old Firefly setup keeps working).
func TestMoneyDirectMode(t *testing.T) {
	h := newMoneyHarness(t, nil)
	h.origin = "ff3-v6.1.0" // a real Firefly III
	code, st := h.do("PUT", "/api/v1/settings", map[string]any{"firefly_url": h.money.URL + "/firefly/", "firefly_token": "saved_token"})
	if code != 200 || st["money_mode"] != "direct" || st["money_via"] != "direct" {
		t.Fatalf("settings %d %v", code, st)
	}
	id := h.trip()
	h.reset()
	if code, out := h.do("POST", "/api/v1/trips/"+itoa(id)+"/money/sync", nil); code != 200 {
		t.Fatalf("sync %d %v", code, out)
	}
	h.mu.Lock()
	if len(h.actAs) != 0 {
		t.Fatalf("direct mode asked Core for a token")
	}
	for _, a := range h.moneyAuth {
		if a != "Bearer saved_token" {
			t.Fatalf("money auth %q", a)
		}
	}
	h.mu.Unlock()
	if rows := h.synced(id); len(rows) != 2 || rows[0].Source != "firefly" {
		t.Fatalf("rows %+v", rows)
	}
	// the old route still works
	if code, _ := h.do("POST", "/api/v1/trips/"+itoa(id)+"/firefly/sync", nil); code != 200 {
		t.Fatalf("old route %d", code)
	}
	// switching to Pocket Core keeps the token but stops using it
	_, st = h.do("PUT", "/api/v1/settings", map[string]any{"money_mode": "core"})
	if st["money_mode"] != "core" || st["firefly_token"] == "" {
		t.Fatalf("after switch %v", st)
	}
	// Disconnect clears URL and token
	_, st = h.do("PUT", "/api/v1/settings", map[string]any{"firefly_url": "", "firefly_token": ""})
	if st["firefly_token"] != "" || st["firefly_url"] != "" {
		t.Fatalf("after disconnect %v", st)
	}
}

// Off: nothing is read, neither on demand nor by the background loop.
func TestMoneyOff(t *testing.T) {
	h := newMoneyHarness(t, nil)
	h.do("PUT", "/api/v1/settings", map[string]any{"money_mode": "off"})
	_, st := h.do("GET", "/api/v1/settings", nil)
	if st["money_mode"] != "off" || st["money_via"] != "" || st["firefly_configured"] != false {
		t.Fatalf("settings %v", st)
	}
	id := h.trip()
	h.reset()
	code, out := h.do("POST", "/api/v1/trips/"+itoa(id)+"/money/sync", nil)
	if code != 502 || !strings.Contains(out["error"].(string), "switched off") {
		t.Fatalf("sync %d %v", code, out)
	}
	h.srv.autoSync(context.Background())
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.moneyPaths) != 0 || len(h.actAs) != 0 {
		t.Fatalf("off still called: money %v core %d", h.moneyPaths, len(h.actAs))
	}
}

// The link switched off in Core: not connected, and no sync.
func TestMoneyLinkOff(t *testing.T) {
	h := newMoneyHarness(t, nil)
	id := h.trip()
	h.linkOn = false
	_, st := h.do("GET", "/api/v1/settings", nil)
	if st["money_mode"] != "core" || st["money_via"] != "" || !strings.Contains(st["money_detail"].(string), "link off in Pocket Core") {
		t.Fatalf("settings %v", st)
	}
	_, ov := h.do("GET", "/api/v1/trips/"+itoa(id)+"/overview", nil)
	if ff := ov["integrations"].(map[string]any)["firefly"].(map[string]any); ff["configured"] != false {
		t.Fatalf("overview %v", ff)
	}
	h.reset()
	code, out := h.do("POST", "/api/v1/trips/"+itoa(id)+"/money/sync", nil)
	if code != 502 || !strings.Contains(out["error"].(string), "link off") {
		t.Fatalf("sync %d %v", code, out)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.moneyPaths) != 0 {
		t.Fatalf("money called %v", h.moneyPaths)
	}
	if rows := h.synced(id); len(rows) != 0 {
		t.Fatalf("rows %+v", rows)
	}
}

// Costs synced from Firefly before are not imported a second time when the
// same transactions come from Pocket Money's mirror (with other ids).
func TestMoneyDedupeAfterFirefly(t *testing.T) {
	h := newMoneyHarness(t, nil)
	id := h.trip()
	ctx := context.Background()
	legacy := func(ext, date, name string, amount int64, cat string) int64 {
		e := &store.Expense{TripID: id, Name: name, Category: cat, Amount: amount, Currency: "EUR", AmountHome: amount, FXRate: 1, Date: date, Kind: "actual", Source: "firefly", ExternalID: ext}
		if err := h.st.CreateExpense(ctx, e); err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	a := legacy("501", "2026-05-02", "Pasteis de Belem", 1240, "shopping") // re-categorised by hand
	b := legacy("502", "2026-05-03", "Tram 28", 600, "transport")
	legacy("503", "2026-05-04", "Untagged since", 999, "other")
	manual := &store.Expense{TripID: id, Name: "Cash tip", Amount: 500, AmountHome: 500, Currency: "EUR", Kind: "actual", Source: "manual", Date: "2026-05-04"}
	_ = h.st.CreateExpense(ctx, manual)
	h.txs = []fakeTx{
		{id: "9001", date: "2026-05-02", desc: "Pastéis de Belém", amount: "12.40", cur: "EUR", ext: "firefly:501"}, // mirrored, says so
		{id: "9002", date: "2026-05-03", desc: "Tram 28", amount: "6.00", cur: "EUR"},                               // mirrored, says nothing
		{id: "9003", date: "2026-05-04", desc: "Oceanário", amount: "25.00", cur: "EUR"},                            // new in Money
	}
	for i := 0; i < 2; i++ {
		if code, out := h.do("POST", "/api/v1/trips/"+itoa(id)+"/money/sync", nil); code != 200 {
			t.Fatalf("sync %d %v", code, out)
		}
		rows := h.synced(id)
		if len(rows) != 3 {
			t.Fatalf("pass %d: want 3 synced rows, got %+v", i, rows)
		}
		got := map[string]store.Expense{}
		for _, e := range rows {
			if e.Source != "money" {
				t.Fatalf("left a %s row: %+v", e.Source, e)
			}
			got[e.ExternalID] = e
		}
		if got["9001"].ID != a || got["9001"].Category != "shopping" || got["9001"].Name != "Pastéis de Belém" {
			t.Fatalf("9001 %+v", got["9001"])
		}
		if got["9002"].ID != b || got["9002"].Category != "transport" {
			t.Fatalf("9002 %+v", got["9002"])
		}
		if _, ok := got["9003"]; !ok {
			t.Fatalf("new cost missing")
		}
	}
	if _, err := h.st.GetExpense(ctx, id, manual.ID); err != nil {
		t.Fatalf("manual cost gone: %v", err)
	}
	_, bud := h.do("GET", "/api/v1/trips/"+itoa(id)+"/budget", nil)
	if bud["actual"].(float64) != 1240+600+2500+500 || bud["firefly_count"].(float64) != 3 {
		t.Fatalf("budget %v", bud)
	}
	// a synced row only takes a new category, not a new amount
	code, out := h.do("PUT", "/api/v1/trips/"+itoa(id)+"/expenses/"+itoa(a), map[string]any{"amount": 1, "category": "food"})
	if code != 200 || out["amount"].(float64) != 1240 || out["category"] != "food" {
		t.Fatalf("edit %d %v", code, out)
	}
}

// Environment values seed the owner's settings once; cleared, they stay
// cleared.
func TestEnvSeedOnce(t *testing.T) {
	h := newMoneyHarness(t, func(c *config.Config) { c.FireflyURL, c.FireflyToken = "http://ff.example", "env_token" })
	ctx := context.Background()
	if err := h.srv.seedOwnerSettings(ctx, h.owner.ID); err != nil {
		t.Fatal(err)
	}
	_, st := h.do("GET", "/api/v1/settings", nil)
	if st["firefly_url"] != "http://ff.example" || st["money_mode"] != "direct" {
		t.Fatalf("seeded %v", st)
	}
	h.do("PUT", "/api/v1/settings", map[string]any{"firefly_url": "", "firefly_token": ""})
	if err := h.srv.seedOwnerSettings(ctx, h.owner.ID); err != nil {
		t.Fatal(err)
	}
	_, st = h.do("GET", "/api/v1/settings", nil)
	if st["firefly_url"] != "" || st["firefly_token"] != "" || st["money_mode"] != "core" {
		t.Fatalf("came back %v", st)
	}
}
