package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core): another
// Pocket app may act here for a person with a pcx_ token, only within these
// scopes. Every route not listed in linkRoutes, portable or capabilities
// stays closed to it.

var linkOffers = []core.Offer{
	{Scope: "trips:read", Name: "See your trips", Description: "List your trips (name, destination, dates, currency, tag), so the app can show what happened during each."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "travel.json", Kind: core.KindPocket, Title: "Every trip you made, with its places, stays, travel, costs, lists and journal"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", MaxBytes: 32 << 20, Title: "Adds the trips from another box (or one trip's file); trips you already have are left alone"},
	},
}

// capabilities are what Pocket Search, Pocket Day and Pocket Hub may ask
// Travel (docs/capabilities.md in pocket-homelab-core); Register adds the
// pocket:* offers. The routes are in capabilities.go. There is no quick
// capture: a trip needs dates and a place, which one line cannot give.
var capabilities = core.Capabilities{
	Search: &core.Searchable{What: "Your trips, by name, destination and description, and the stays, travel and places in them",
		Kinds: []string{"trip", "booking", "stop"}, Bangs: []string{"trips", "trip", "travel"}},
	Day: &core.DayFeed{Label: "Travel", One: "plan", Many: "plans", Unit: "count",
		What: "The trip you are on that day and what its plan says: travel, check-in and check-out, and the places for the day"},
	Widget: &core.Widget{Title: "Next trip", What: "The trip you are on or the next one, with what comes up first"},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/trips$`), "trips:read"},
	{"GET", regexp.MustCompile(`^/api/v1/trips/\d+$`), "trips:read"},
}

var errLinkScope = errors.New("outside the link's scope")

func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) || capabilities.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token and maps it to the person it acts for.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, err
	}
	if !in.Active || !in.Delegated() || in.User == nil || in.User.Email == "" {
		return nil, http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, errLinkScope
	}
	return s.coreIdentityUser(ctx, in)
}
