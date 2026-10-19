package server

import (
	"strconv"
	"strings"
	"testing"
)

// A linked app (Pocket Garage) acting for the member sees only the member's
// trips, and nothing else in the app.
func TestCoreLinkScopes(t *testing.T) {
	h := newIsoHarness(t)
	h.create(h.ownerTok, "/api/v1/trips", map[string]any{"name": "Owner Lisbon", "start_date": "2026-05-01", "end_date": "2026-05-05"})
	mine := h.create(h.memberTok, "/api/v1/trips", map[string]any{"name": "Member Rome", "start_date": "2026-06-01", "end_date": "2026-06-04"})
	tok := "pcx_member_trips"
	code, body := h.do(tok, "GET", "/api/v1/trips", nil)
	if code != 200 || !strings.Contains(string(body), "Member Rome") || strings.Contains(string(body), "Owner Lisbon") {
		t.Fatalf("linked trips %d %s", code, body)
	}
	if code, _ := h.do(tok, "GET", "/api/v1/trips/"+itoa(mine), nil); code != 200 {
		t.Fatalf("linked trip %d", code)
	}
	for _, c := range []struct{ m, p string }{
		{"POST", "/api/v1/trips"}, {"PUT", "/api/v1/trips/" + itoa(mine)}, {"DELETE", "/api/v1/trips/" + itoa(mine)},
		{"GET", "/api/v1/trips/" + itoa(mine) + "/expenses"}, {"GET", "/api/v1/backup"}, {"GET", "/api/v1/tokens"},
	} {
		if code, _ := h.do(tok, c.m, c.p, map[string]any{}); code != 403 {
			t.Fatalf("%s %s with a link token: %d, want 403", c.m, c.p, code)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
