package server

import (
	"context"
	"errors"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/dawarich"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/movement"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// ---- where you actually went: Pocket Movement or Dawarich ----
//
// The recorded route, the timeline, distances and countries of a trip come
// from one of two places, chosen per person in Settings (location_source):
//
//   - movement: Pocket Movement, the person's own location history, ONLY
//     through Pocket Core (a five-minute pcx_ link token for movement:read +
//     movement:points, asked for each piece of work). Movement speaks
//     Dawarich's API, so points and the timeline (named visits, journeys
//     with their mode) read the same; per-day distances and countries come
//     from its day summaries.
//   - dawarich: Dawarich, direct or through Core, as in services.go.
//
// With nothing chosen: Pocket Movement while Travel is connected to Core and
// the link is on for the person, else Dawarich. A saved or env Dawarich does
// not decide it; only picking Dawarich in Settings does. When Movement is
// chosen but Core refuses (no Core, link switched off, Movement missing),
// Travel falls back to Dawarich meanwhile and Settings says why.

// movementScopes is what Travel reads in Pocket Movement for a person.
var movementScopes = []string{"movement:read", "movement:points"}

// movementUses is the manifest's Uses for Pocket Movement.
var movementUses = []core.Use{
	{App: "movement", Scopes: movementScopes, Reason: "Where you actually went on your trips: the route on the map, the day-by-day timeline, distances and countries"},
}

const (
	srcMovement = "movement"
	srcDawarich = "dawarich"
)

// movementSeen is Core's last answer on the Movement link for a person,
// reused for a minute so a page of trips does not ask Core for each.
type movementSeen struct {
	on     bool
	reason string
	at     time.Time
}

const movementSeenFor = time.Minute

// coreReady: Travel is connected to a Core and registered with it (what
// Settings needs before it offers anything that only works through Core).
func (s *Server) coreReady() bool {
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	return s.core.Configured() && s.coreOK
}

// movementState says whether Pocket Movement is open to the person through
// Core, and Core's reason when it is not ("no_core" without a Core).
func (s *Server) movementState(ctx context.Context, u *store.User, fresh bool) (bool, string) {
	if !s.core.Configured() {
		return false, "no_core"
	}
	if u == nil {
		return false, "unknown_user"
	}
	if !fresh {
		s.mvMu.Lock()
		seen, ok := s.mvSeen[u.Email]
		s.mvMu.Unlock()
		if ok && time.Since(seen.at) < movementSeenFor {
			return seen.on, seen.reason
		}
	}
	on, reason := s.core.LinkOn(ctx, u.Email, "movement", movementScopes...)
	s.sawMovement(u.Email, on, reason)
	return on, reason
}

// sawMovement remembers Core's answer (logging when it changes).
func (s *Server) sawMovement(email string, on bool, reason string) {
	s.mvMu.Lock()
	prev, had := s.mvSeen[email]
	s.mvSeen[email] = movementSeen{on: on, reason: reason, at: time.Now()}
	s.mvMu.Unlock()
	if had && prev.on == on && prev.reason == reason {
		return
	}
	if on {
		s.log.Info("pocket movement: link on, routes come from Movement", "user", email)
	} else {
		s.log.Info("pocket movement: not available, Dawarich meanwhile", "user", email, "reason", reason)
	}
}

// movementReason is Core's reason in words.
func movementReason(reason string) string {
	switch reason {
	case "":
		return ""
	case "no_core":
		return "there is no Pocket Core on this box"
	case "target_missing":
		return "Pocket Movement is not registered with Pocket Core"
	case "not_offered":
		return "this Pocket Movement version does not share its history with other apps yet"
	}
	if t := linkReason("Pocket Movement", reason); t != "" {
		return t
	}
	return reason
}

// locationChoice is the source in effect for the person, and (when
// Movement was wanted but cannot be used) why not.
func (s *Server) locationChoice(ctx context.Context, u *store.User, st store.Settings, fresh bool) (src, why string) {
	if st.LocationSource == srcDawarich {
		return srcDawarich, ""
	}
	on, reason := s.movementState(ctx, u, fresh)
	if on {
		return srcMovement, ""
	}
	return srcDawarich, movementReason(reason)
}

// locConn is one person's location history, opened for a piece of work.
type locConn struct {
	src string
	dw  *dawarich.Client // Dawarich, or Movement through its Dawarich API
	mv  *movement.Client // Movement only
}

// cacheName keys what came from this source, so switching sources never
// shows the other one's cached route.
func (c *locConn) cacheName(what string) string { return locCacheName(c.src, what) }

func locCacheName(src, what string) string { return src + "-" + what }

// label is how the source reads in messages.
func (c *locConn) label() string {
	if c.src == srcMovement {
		return "Pocket Movement"
	}
	return "Dawarich"
}

// locationFor opens userID's location history the way they chose: Movement
// through Core (a fresh link token), falling back to their Dawarich while
// Core refuses.
func (s *Server) locationFor(ctx context.Context, userID int64) (*locConn, error) {
	u, st := s.personSettings(ctx, userID)
	if st.LocationSource != srcDawarich && u != nil && s.core.Configured() {
		c, err := s.openMovement(ctx, u)
		if err == nil {
			return c, nil
		}
		s.log.Debug("pocket movement: not used", "user", u.Email, "err", err)
	}
	dw, _, err := s.dawarichFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &locConn{src: srcDawarich, dw: dw}, nil
}

// openMovement asks Core for a link token to the person's Pocket Movement.
func (s *Server) openMovement(ctx context.Context, u *store.User) (*locConn, error) {
	tok, err := s.core.ActAs(ctx, u.Email, "movement", movementScopes...)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			s.sawMovement(u.Email, false, le.Reason)
			return nil, errors.New(movementReason(le.Reason))
		}
		return nil, err
	}
	if tok.URL == "" {
		return nil, errors.New("Pocket Core did not say where Pocket Movement lives")
	}
	s.sawMovement(u.Email, true, "")
	mv := movement.New(strings.TrimRight(tok.URL, "/"), tok.AccessToken, 60*time.Second)
	return &locConn{src: srcMovement, dw: mv.Client, mv: mv}, nil
}

// tripLocation is the viewer's location history for a trip that shows the route.
func (s *Server) tripLocation(ctx context.Context, t *store.Trip) (*locConn, error) {
	if !t.Dawarich {
		return nil, errors.New("the recorded route is off for this trip")
	}
	return s.locationFor(ctx, actorID(ctx))
}

// locationStatus is the source in effect for userID and whether it works,
// without opening anything (for the trip page and the home page).
func (s *Server) locationStatus(ctx context.Context, userID int64) (src string, ok bool) {
	u, st := s.personSettings(ctx, userID)
	src, _ = s.locationChoice(ctx, u, st, false)
	if src == srcMovement {
		return src, true
	}
	return src, s.dawarichVia(ctx, userID) != ""
}

// locationMode is the person's Movement link as Core's map shows it: core
// unless they picked Dawarich.
func locationMode(st store.Settings) string {
	if st.LocationSource == srcDawarich {
		return modeOff
	}
	return modeCore
}
