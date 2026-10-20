package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/firefly"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// ---- Pocket Money: where trip costs come from ----
//
// Trip costs are the withdrawals carrying the trip's tag, read through the
// Firefly III API. Pocket Money serves that API under <money>/firefly, so the
// same client works three ways, chosen per person in Settings:
//
//   - core:   Travel acts for the person through a Pocket Core app link
//     (transactions:read on Money) with a five-minute pcx_ token asked for
//     at every sync; nothing to paste, each person reads their own Money.
//   - direct: a URL + token typed into Settings: Money's /firefly with a
//     Money API token, or a real Firefly III with a personal access token.
//   - off:    no costs are read.

var moneyScopes = []string{"transactions:read"}

// moneyUses is what Travel does in Pocket Money for a person (the manifest's
// Uses, shown to them and the admin in Pocket Core).
var moneyUses = []core.Use{
	{App: "money", Scopes: moneyScopes, Reason: "Trip costs from the transactions tagged with the trip"},
}

// Connection modes, chosen per person in Settings.
const (
	modeCore   = "core"
	modeDirect = "direct"
	modeOff    = "off"
)

// errMoneyOff: the person switched Pocket Money off; nothing to sync.
var errMoneyOff = errors.New("Pocket Money is switched off in Settings → Integrations")

// connMode is the mode in effect: the chosen one, else direct when a token
// was entered (how it always worked), else Pocket Core.
func connMode(mode, token string) string {
	switch mode {
	case modeCore, modeDirect, modeOff:
		return mode
	}
	if token != "" {
		return modeDirect
	}
	return modeCore
}

func linkReason(reason string) string {
	switch reason {
	case "off_person":
		return "you switched the Travel → Pocket Money link off in Pocket Core (Settings → Apps acting for me)"
	case "off_admin":
		return "the Travel → Pocket Money link is switched off in Pocket Core (Links)"
	case "target_missing":
		return "Pocket Money is not registered with Pocket Core"
	case "not_offered":
		return "this Pocket Money version does not let apps read transactions yet"
	case "not_declared":
		return "Pocket Core has not seen this Travel version yet; it re-registers within a minute of starting"
	case "not_in_audience":
		return "you do not have access to Pocket Money in Pocket Core"
	case "unknown_user":
		return "your email is not a person in Pocket Core"
	}
	return ""
}

// moneyConn is how Travel reads one person's costs.
type moneyConn struct {
	client *firefly.Client
	via    string // core | direct
}

// moneyFor connects to userID's Pocket Money (or Firefly) the way they chose.
func (s *Server) moneyFor(ctx context.Context, userID int64) (*moneyConn, error) {
	st, _ := s.store.GetSettings(ctx, userID)
	switch connMode(st.MoneyMode, st.FireflyToken) {
	case modeOff:
		return nil, errMoneyOff
	case modeDirect:
		if st.FireflyToken == "" {
			return nil, errors.New("Pocket Money is set to a direct connection but has no token: add one in Settings → Integrations, or switch to Pocket Core")
		}
		base := st.FireflyURL
		if base == "" {
			// Pocket Money serves its Firefly-compatible API under /firefly.
			if u := s.resolveSibling(ctx, "money"); u != "" {
				base = u + "/firefly"
			}
		}
		if base == "" {
			return nil, errors.New("set the Pocket Money (or Firefly III) URL in Settings → Integrations")
		}
		return &moneyConn{client: firefly.New(base, st.FireflyToken, 20*time.Second), via: modeDirect}, nil
	}
	if !s.core.Configured() {
		return nil, errors.New("Pocket Money through Pocket Core: there is no Pocket Core on this box; use a direct connection")
	}
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("nobody to act for: %w", err)
	}
	tok, err := s.core.ActAs(ctx, u.Email, "money", moneyScopes...)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			if t := linkReason(le.Reason); t != "" {
				return nil, errors.New(t)
			}
		}
		return nil, err
	}
	if tok.URL == "" {
		return nil, errors.New("Pocket Core did not say where Pocket Money lives")
	}
	return &moneyConn{client: firefly.New(strings.TrimRight(tok.URL, "/")+"/firefly", tok.AccessToken, 20*time.Second), via: modeCore}, nil
}

// moneyStatus says, without issuing a token, how a person reaches Pocket
// Money: the mode in effect, whether it works ("core" / "direct" / "" for
// not connected) and why not.
func (s *Server) moneyStatus(ctx context.Context, u *store.User, st store.Settings) (eff, via, detail string) {
	eff = connMode(st.MoneyMode, st.FireflyToken)
	switch eff {
	case modeOff:
		return eff, "", "switched off"
	case modeDirect:
		if st.FireflyToken == "" {
			return eff, "", "no token yet"
		}
		if st.FireflyURL == "" && s.resolveSibling(ctx, "money") == "" {
			return eff, "", "no URL yet"
		}
		return eff, modeDirect, ""
	}
	if u == nil || !s.core.Configured() {
		return eff, "", "there is no Pocket Core on this box"
	}
	on, reason := s.core.LinkOn(ctx, u.Email, "money", moneyScopes...)
	if on {
		return eff, modeCore, ""
	}
	if t := linkReason(reason); t != "" {
		return eff, "", t
	}
	return eff, "", reason
}

// moneyVia is moneyStatus's "does it work" part for userID.
func (s *Server) moneyVia(ctx context.Context, userID int64) string {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return ""
	}
	st, _ := s.store.GetSettings(ctx, userID)
	_, via, _ := s.moneyStatus(ctx, u, st)
	return via
}
