package server

import (
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

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/config"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// svcHarness: Travel with a fake Dawarich, a fake Immich and a fake Pocket
// Core that hands out their credentials (the box's key for the owner, the
// member's own key or no_account for the member).
type svcHarness struct {
	t                   *testing.T
	ts                  *httptest.Server
	st                  *store.Store
	srv                 *Server
	owner, member       *store.User
	ownerTok, memberTok string
	dawarich, immich    *httptest.Server

	mu          sync.Mutex
	memberOwn   bool
	dwKeys      []string // api_key values Dawarich saw
	imKeys      []string // x-api-key values Immich saw
	creds       []string // "user type" Core handed a credential for
	statusCalls int
	registered  core.Manifest
}

func newSvcHarness(t *testing.T, withCore bool) *svcHarness {
	t.Helper()
	h := &svcHarness{t: t}
	h.dawarich = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("api_key")
		h.mu.Lock()
		h.dwKeys = append(h.dwKeys, key)
		h.mu.Unlock()
		if key == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/points":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/timeline":
			_, _ = w.Write([]byte(`{"days":[{"date":"2026-05-02","summary":{"total_distance":12,"distance_unit":"km"},"entries":[]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(h.dawarich.Close)
	h.immich = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-api-key")
		h.mu.Lock()
		h.imKeys = append(h.imKeys, key)
		h.mu.Unlock()
		if key == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/server/version":
			_, _ = w.Write([]byte(`{"major":1,"minor":140,"patch":2}`))
		case r.URL.Path == "/api/search/metadata":
			_, _ = w.Write([]byte(`{"assets":{"items":[{"id":"ab12-` + key + `","type":"IMAGE","originalFileName":"a.jpg","localDateTime":"2026-05-02T10:00:00.000Z"}],"nextPage":null}}`))
		case strings.HasSuffix(r.URL.Path, "/thumbnail"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg:" + key))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(h.immich.Close)
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		h.mu.Lock()
		defer h.mu.Unlock()
		state := func(user string) (string, string) { // state, source
			switch {
			case user == "owner@example.com":
				return "on", "box"
			case user == "member@example.com" && h.memberOwn:
				return "on", "personal"
			}
			return "no_account", ""
		}
		switch r.URL.Path {
		case "/api/v1/apps/register":
			_ = json.NewDecoder(r.Body).Decode(&h.registered)
			_, _ = w.Write([]byte(`{"app":{}}`))
		case "/api/v1/links/status":
			app := r.URL.Query().Get("app")
			if !strings.HasPrefix(app, "service:") {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			h.statusCalls++
			st, _ := state(r.URL.Query().Get("user"))
			_ = json.NewEncoder(w).Encode([]map[string]any{{"app": app, "scope": "account", "state": st, "on": st == "on"}})
		case "/api/v1/services/credential":
			var in struct{ User, Type string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			h.creds = append(h.creds, in.User+" "+in.Type)
			st, source := state(in.User)
			url := map[string]string{"dawarich": h.dawarich.URL, "immich": h.immich.URL}[in.Type]
			if url == "" || st != "on" {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": in.Type + ": no account", "reason": st})
				return
			}
			who := map[string]string{"box": "box", "personal": "member"}[source]
			key := who + "-" + in.Type + "-key"
			_ = json.NewEncoder(w).Encode(map[string]any{"type": in.Type, "url": url, "public_url": "https://" + in.Type + ".example", "key": key,
				"fields": map[string]string{"API_KEY": key}, "source": source})
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(coreSrv.Close)
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, DBPath: dir + "/t.db", FilesDir: dir + "/files", AuthMode: "local", DefaultCurrency: "EUR", FXProvider: "none",
		SessionTTL: time.Hour, MaxUploadBytes: 10 << 20, UserAgent: "test"}
	if withCore {
		cfg.CoreURL, cfg.CoreRegistrationKey = coreSrv.URL, "pcr_test"
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
	return h
}

func (h *svcHarness) do(tok, method, path string, body any) (int, string) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, h.ts.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (h *svcHarness) settings(tok string) map[string]any {
	h.t.Helper()
	code, body := h.do(tok, "GET", "/api/v1/settings", nil)
	if code != 200 {
		h.t.Fatalf("settings %d %s", code, body)
	}
	var st map[string]any
	_ = json.Unmarshal([]byte(body), &st)
	return st
}

func (h *svcHarness) reset() {
	h.mu.Lock()
	h.dwKeys, h.imKeys, h.creds, h.statusCalls = nil, nil, nil, 0
	h.mu.Unlock()
}

func (h *svcHarness) seen() (dw, im, creds []string, status int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.dwKeys...), append([]string(nil), h.imKeys...), append([]string(nil), h.creds...), h.statusCalls
}

func allEqual(keys []string, want string) bool {
	for _, k := range keys {
		if k != want {
			return false
		}
	}
	return len(keys) > 0
}

// sharedTrip is the owner's trip, shared with the member as a viewer.
func (h *svcHarness) sharedTrip() string {
	h.t.Helper()
	code, body := h.do(h.ownerTok, "POST", "/api/v1/trips", map[string]any{"name": "Lisbon", "start_date": "2026-05-01", "end_date": "2026-05-05", "currency": "EUR"})
	if code != 201 {
		h.t.Fatalf("create trip %d %s", code, body)
	}
	var tr struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &tr)
	base := "/api/v1/trips/" + itoa(tr.ID)
	if code, body := h.do(h.ownerTok, "PUT", base+"/members", map[string]any{"members": []any{map[string]any{"user_id": h.member.ID, "role": "viewer"}}}); code != 200 {
		h.t.Fatalf("share %d %s", code, body)
	}
	return base
}

func (h *svcHarness) overview(tok, base string) map[string]map[string]any {
	h.t.Helper()
	_, body := h.do(tok, "GET", base+"/overview", nil)
	var ov struct {
		Integrations map[string]map[string]any `json:"integrations"`
	}
	if err := json.Unmarshal([]byte(body), &ov); err != nil {
		h.t.Fatalf("overview %s", body)
	}
	return ov.Integrations
}

func TestServiceConnectionsManifest(t *testing.T) {
	h := newSvcHarness(t, true)
	if err := h.srv.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range h.registered.Services {
		got[s.Type] = s.Reason
	}
	if len(got) != 2 || got["dawarich"] != "Routes and the timeline of your trips" || got["immich"] != "Photos from your trips" {
		t.Fatalf("manifest services %+v", h.registered.Services)
	}
}

func TestServiceConnectionModes(t *testing.T) {
	h := newSvcHarness(t, true)
	base := h.sharedTrip()

	// The owner saved keys: Direct for both, nothing asked of Core.
	if code, body := h.do(h.ownerTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_url": h.dawarich.URL, "dawarich_api_key": "own-dawarich-key",
		"immich_url": h.immich.URL, "immich_api_key": "own-immich-key"}); code != 200 {
		t.Fatalf("save %d %s", code, body)
	}
	st := h.settings(h.ownerTok)
	if st["dawarich_mode"] != "direct" || st["dawarich_via"] != "direct" || st["immich_mode"] != "direct" || st["immich_via"] != "direct" || st["immich_configured"] != true {
		t.Fatalf("owner default %v", st)
	}
	h.reset()
	if code, body := h.do(h.ownerTok, "GET", base+"/dawarich/timeline", nil); code != 200 || !strings.Contains(body, "2026-05-02") {
		t.Fatalf("direct timeline %d %s", code, body)
	}
	if code, body := h.do(h.ownerTok, "GET", base+"/photos", nil); code != 200 || !strings.Contains(body, "ab12-own-immich-key") {
		t.Fatalf("direct photos %d %s", code, body)
	}
	if code, body := h.do(h.ownerTok, "GET", base+"/photos/ab12/thumb", nil); code != 200 || body != "jpeg:own-immich-key" {
		t.Fatalf("direct thumb %d %s", code, body)
	}
	dw, im, creds, _ := h.seen()
	if !allEqual(dw, "own-dawarich-key") || !allEqual(im, "own-immich-key") || len(creds) != 0 {
		t.Fatalf("direct: dawarich %v immich %v core %v", dw, im, creds)
	}

	// Pocket Core: Core's address and key, asked for at every use.
	h.do(h.ownerTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_mode": "core", "immich_mode": "core"})
	st = h.settings(h.ownerTok)
	if st["dawarich_via"] != "core" || st["dawarich_source"] != "box" || st["immich_via"] != "core" || st["immich_source"] != "box" {
		t.Fatalf("owner core %v", st)
	}
	h.reset()
	if code, body := h.do(h.ownerTok, "GET", base+"/dawarich/timeline", nil); code != 200 || !strings.Contains(body, "2026-05-02") {
		t.Fatalf("core timeline %d %s", code, body)
	}
	if code, body := h.do(h.ownerTok, "GET", base+"/photos", nil); code != 200 || !strings.Contains(body, "ab12-box-immich-key") {
		t.Fatalf("core photos %d %s", code, body)
	}
	for i := 0; i < 2; i++ {
		if code, body := h.do(h.ownerTok, "GET", base+"/photos/ab12/thumb", nil); code != 200 || body != "jpeg:box-immich-key" {
			t.Fatalf("core thumb %d %s", code, body)
		}
	}
	dw, im, creds, _ = h.seen()
	if !allEqual(dw, "box-dawarich-key") || !allEqual(im, "box-immich-key") {
		t.Fatalf("core: dawarich %v immich %v", dw, im)
	}
	if strings.Join(creds, ",") != "owner@example.com dawarich,owner@example.com immich,owner@example.com immich,owner@example.com immich" {
		t.Fatalf("want a credential per use, Core saw %v", creds)
	}
	ov := h.overview(h.ownerTok, base)
	if ov["dawarich"]["configured"] != true || ov["immich"]["configured"] != true || ov["immich"]["url"] != "https://immich.example" {
		t.Fatalf("overview core %v", ov)
	}
	if _, body := h.do(h.ownerTok, "POST", "/api/v1/integrations/test", map[string]any{"which": "immich"}); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "Immich 1.140.2, through Pocket Core (the box's key)") {
		t.Fatalf("test immich %s", body)
	}
	if _, body := h.do(h.ownerTok, "POST", "/api/v1/integrations/test", map[string]any{"which": "dawarich"}); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "through Pocket Core (the box's key)") {
		t.Fatalf("test dawarich %s", body)
	}

	// Off: no calls at all.
	h.do(h.ownerTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_mode": "off", "immich_mode": "off"})
	st = h.settings(h.ownerTok)
	if st["dawarich_mode"] != "off" || st["dawarich_via"] != "" || st["immich_mode"] != "off" || st["immich_configured"] != false {
		t.Fatalf("owner off %v", st)
	}
	h.reset()
	if code, body := h.do(h.ownerTok, "GET", base+"/dawarich/timeline", nil); code != 200 || body != "[]\n" && body != "[]" {
		t.Fatalf("off timeline %d %q", code, body)
	}
	if code, body := h.do(h.ownerTok, "GET", base+"/photos", nil); code != 200 || strings.Contains(body, "ab12") {
		t.Fatalf("off photos %d %s", code, body)
	}
	if code, _ := h.do(h.ownerTok, "GET", base+"/photos/ab12/thumb", nil); code != http.StatusNotFound {
		t.Fatalf("off thumb %d", code)
	}
	ov = h.overview(h.ownerTok, base)
	if ov["dawarich"]["configured"] != false || ov["immich"]["configured"] != false {
		t.Fatalf("overview off %v", ov)
	}
	if dw, im, creds, status := h.seen(); len(dw)+len(im)+len(creds)+status != 0 {
		t.Fatalf("off: dawarich %v immich %v core %v status %d", dw, im, creds, status)
	}

	// The member saved nothing: Pocket Core by default, and Core has no
	// account of theirs. The shared trip shows nothing of the owner's.
	st = h.settings(h.memberTok)
	want := "you have no Dawarich account of your own yet; ask the admin or let a connector create one"
	if st["dawarich_mode"] != "core" || st["dawarich_via"] != "" || !strings.Contains(st["dawarich_detail"].(string), want) ||
		st["immich_mode"] != "core" || !strings.Contains(st["immich_detail"].(string), "you have no Immich account of your own yet") {
		t.Fatalf("member no_account %v", st)
	}
	h.reset()
	if _, body := h.do(h.memberTok, "GET", base+"/photos", nil); strings.Contains(body, "ab12") {
		t.Fatalf("member photos without account %s", body)
	}
	if code, body := h.do(h.memberTok, "GET", base+"/photos/ab12/thumb", nil); code != http.StatusNotFound || !strings.Contains(body, "no Immich account of your own") {
		t.Fatalf("member thumb without account %d %s", code, body)
	}
	if dw, im, _, _ := h.seen(); len(dw)+len(im) != 0 {
		t.Fatalf("no_account: dawarich %v immich %v", dw, im)
	}

	// With their own key in Core, the viewer's key, not the owner's.
	h.mu.Lock()
	h.memberOwn = true
	h.mu.Unlock()
	st = h.settings(h.memberTok)
	if st["immich_via"] != "core" || st["immich_source"] != "personal" || st["dawarich_source"] != "personal" {
		t.Fatalf("member personal %v", st)
	}
	h.reset()
	if code, body := h.do(h.memberTok, "GET", base+"/photos/ab12/thumb", nil); code != 200 || body != "jpeg:member-immich-key" {
		t.Fatalf("member thumb %d %s", code, body)
	}
	if code, body := h.do(h.memberTok, "GET", base+"/dawarich/timeline", nil); code != 200 || !strings.Contains(body, "2026-05-02") {
		t.Fatalf("member timeline %d %s", code, body)
	}
	dw, im, creds, _ = h.seen()
	if !allEqual(im, "member-immich-key") || !allEqual(dw, "member-dawarich-key") || !allEqual(creds[:1], "member@example.com immich") {
		t.Fatalf("member: dawarich %v immich %v core %v", dw, im, creds)
	}
}

// Without Pocket Core and without a key, both stay Direct (not connected).
func TestServiceConnectionsWithoutCore(t *testing.T) {
	h := newSvcHarness(t, false)
	st := h.settings(h.memberTok)
	if st["dawarich_mode"] != "direct" || st["dawarich_via"] != "" || st["dawarich_detail"] != "no API key yet" || st["immich_mode"] != "direct" || st["immich_configured"] != false {
		t.Fatalf("without core %v", st)
	}
	if code, body := h.do(h.memberTok, "PUT", "/api/v1/settings", map[string]any{"immich_mode": "core"}); code != 200 || !strings.Contains(body, "there is no Pocket Core on this box") {
		t.Fatalf("core without core %d %s", code, body)
	}
}
