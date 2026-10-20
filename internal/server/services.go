package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/dawarich"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/immich"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// ---- third-party services: Dawarich and Immich ----
//
// Like Pocket Money, chosen per person and per service in Settings:
//
//   - core:   Pocket Core hands out the service's address and the person's
//     own key there (the box's key only when Travel acts for an admin),
//     asked for at every use; nothing to paste.
//   - direct: the URL + key saved in Settings, the way it always worked.
//   - off:    Travel does not call the service.
//
// With nothing chosen: direct while a key is saved (so nothing changes for
// whoever already set one up), else core when Travel is connected to Pocket
// Core, else direct. Route and photos always use the viewer's own
// connection, never the trip owner's.

// serviceUses is the manifest's Services: what Travel reaches for a person.
var serviceUses = []core.ServiceUse{
	{Type: "dawarich", Reason: "Routes and the timeline of your trips"},
	{Type: "immich", Reason: "Photos from your trips"},
}

// serviceScope is the single scope Core keeps a service link under.
const serviceScope = "account"

// serviceConn is one person's way into a service right now.
type serviceConn struct {
	url    string
	public string // what a browser opens
	key    string
	via    string // core | direct
	source string // core only: personal | box
}

// serviceSaved is the person's mode and Direct settings for a service.
type serviceSaved struct {
	typ, name, mode, url, key, public string
}

func savedDawarich(st store.Settings) serviceSaved {
	return serviceSaved{typ: "dawarich", name: "Dawarich", mode: st.DawarichMode, url: st.DawarichURL, key: st.DawarichAPIKey}
}

func savedImmich(st store.Settings) serviceSaved {
	return serviceSaved{typ: "immich", name: "Immich", mode: st.ImmichMode, url: st.ImmichURL, key: st.ImmichAPIKey, public: st.ImmichPublicURL}
}

// serviceMode is the mode in effect for a service.
func (s *Server) serviceMode(mode, key string) string {
	fallback := modeDirect
	if s.core.Configured() {
		fallback = modeCore
	}
	return connModeOr(mode, key, fallback)
}

func serviceReason(name, reason string) string {
	if reason == "target_missing" {
		return name + " is not added to Pocket Core (Services)"
	}
	return linkReason(name, reason)
}

// serviceFor connects to the person's service the way they chose.
func (s *Server) serviceFor(ctx context.Context, u *store.User, sv serviceSaved) (*serviceConn, error) {
	switch s.serviceMode(sv.mode, sv.key) {
	case modeOff:
		return nil, fmt.Errorf("%s is switched off in Settings → Integrations", sv.name)
	case modeDirect:
		if sv.key == "" {
			if sv.url == "" {
				return nil, fmt.Errorf("%s is not configured: add the URL and an API key in Settings → Integrations", sv.name)
			}
			return nil, fmt.Errorf("%s is set to a direct connection but has no API key: add one in Settings → Integrations, or switch to Pocket Core", sv.name)
		}
		if sv.url == "" {
			return nil, fmt.Errorf("set the %s URL in Settings → Integrations", sv.name)
		}
		return &serviceConn{url: sv.url, public: firstNonEmpty(sv.public, sv.url), key: sv.key, via: modeDirect}, nil
	}
	if !s.core.Configured() {
		return nil, fmt.Errorf("%s through Pocket Core: there is no Pocket Core on this box; use a direct connection", sv.name)
	}
	if u == nil {
		return nil, errors.New("nobody to act for")
	}
	cred, err := s.core.ServiceFor(ctx, u.Email, sv.typ)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			if t := serviceReason(sv.name, le.Reason); t != "" {
				return nil, errors.New(t)
			}
		}
		return nil, err
	}
	if cred.URL == "" {
		return nil, fmt.Errorf("Pocket Core did not say where %s lives", sv.name)
	}
	if cred.Key == "" {
		return nil, fmt.Errorf("Pocket Core has no %s API key for you", sv.name)
	}
	return &serviceConn{url: strings.TrimRight(cred.URL, "/"), public: strings.TrimRight(firstNonEmpty(cred.PublicURL, cred.URL), "/"), key: cred.Key, via: modeCore, source: cred.Source}, nil
}

// serviceStatus says how a person reaches a service: the mode in effect,
// whether it works ("core" / "direct" / "" for not connected), why not, and
// whose key Core hands out. Without detail it only asks Core for the link's
// state (no key leaves Core); with detail it asks for the credential too, to
// learn whose key it is.
func (s *Server) serviceStatus(ctx context.Context, u *store.User, sv serviceSaved, detail bool) (eff, via, why, source string) {
	eff = s.serviceMode(sv.mode, sv.key)
	switch eff {
	case modeOff:
		return eff, "", "switched off", ""
	case modeDirect:
		if sv.key == "" {
			return eff, "", "no API key yet", ""
		}
		if sv.url == "" {
			return eff, "", "no URL yet", ""
		}
		return eff, modeDirect, "", ""
	}
	if u == nil || !s.core.Configured() {
		return eff, "", "there is no Pocket Core on this box", ""
	}
	if detail {
		c, err := s.serviceFor(ctx, u, sv)
		if err != nil {
			return eff, "", err.Error(), ""
		}
		return eff, modeCore, "", c.source
	}
	on, reason := s.core.LinkOn(ctx, u.Email, "service:"+sv.typ, serviceScope)
	if on {
		return eff, modeCore, "", ""
	}
	if t := serviceReason(sv.name, reason); t != "" {
		return eff, "", t, ""
	}
	return eff, "", reason, ""
}

// personSettings is userID and their settings (nil user when unknown).
func (s *Server) personSettings(ctx context.Context, userID int64) (*store.User, store.Settings) {
	st, _ := s.store.GetSettings(ctx, userID)
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, st
	}
	return u, st
}

// dawarichFor is userID's Dawarich, the way they chose.
func (s *Server) dawarichFor(ctx context.Context, userID int64) (*dawarich.Client, *serviceConn, error) {
	u, st := s.personSettings(ctx, userID)
	c, err := s.serviceFor(ctx, u, savedDawarich(st))
	if err != nil {
		return nil, nil, err
	}
	return dawarich.New(c.url, c.key, 60*time.Second), c, nil
}

// immichFor is userID's Immich, the way they chose; conn.public is what the
// browser opens for "Open in Immich".
func (s *Server) immichFor(ctx context.Context, userID int64) (*immich.Client, *serviceConn, error) {
	u, st := s.personSettings(ctx, userID)
	c, err := s.serviceFor(ctx, u, savedImmich(st))
	if err != nil {
		return nil, nil, err
	}
	return immich.New(c.url, c.key, 30*time.Second), c, nil
}

// tripDawarich is the viewer's Dawarich for a trip that shows the route.
func (s *Server) tripDawarich(ctx context.Context, t *store.Trip) (*dawarich.Client, *serviceConn, error) {
	if !t.Dawarich {
		return nil, nil, errors.New("the recorded route is off for this trip")
	}
	return s.dawarichFor(ctx, actorID(ctx))
}

// dawarichVia is serviceStatus's "does it work" part for userID's Dawarich.
func (s *Server) dawarichVia(ctx context.Context, userID int64) string {
	u, st := s.personSettings(ctx, userID)
	_, via, _, _ := s.serviceStatus(ctx, u, savedDawarich(st), false)
	return via
}

// viaText is how a working connection reads in a test message.
func viaText(c *serviceConn) string {
	if c.via != modeCore {
		return "direct connection"
	}
	if c.source == "box" {
		return "through Pocket Core (the box's key)"
	}
	return "through Pocket Core (your own key)"
}
