package server

import (
	"context"

	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/dawarich"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/export"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/firefly"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/immich"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// ---- settings ----

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

func (s *Server) publicSettings(st store.Settings) map[string]any {
	return map[string]any{
		"title": st.Title, "home_currency": st.HomeCurrency, "date_format": st.DateFormat, "time_format": st.TimeFormat,
		"distance_unit": st.DistanceUnit, "fx_auto": st.FXAuto, "fx_provider": s.cfg.FXProvider, "map_tiles": st.MapTiles,
		"firefly_url": st.FireflyURL, "firefly_token": mask(st.FireflyToken), "firefly_configured": st.FireflyURL != "" && st.FireflyToken != "",
		"dawarich_url": st.DawarichURL, "dawarich_api_key": mask(st.DawarichAPIKey), "dawarich_configured": st.DawarichURL != "" && st.DawarichAPIKey != "",
		"immich_url": st.ImmichURL, "immich_public_url": st.ImmichPublicURL, "immich_api_key": mask(st.ImmichAPIKey), "immich_configured": st.ImmichURL != "" && st.ImmichAPIKey != "",
		"max_upload_bytes": s.cfg.MaxUploadBytes, "setup_complete": st.SetupComplete,
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(st))
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	cur, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in store.Settings
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["title"] {
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
	}
	if keys["home_currency"] {
		if !currency.Valid(in.HomeCurrency) {
			writeErr(w, http.StatusBadRequest, "unknown currency")
			return
		}
		cur.HomeCurrency = strings.ToUpper(in.HomeCurrency)
	}
	if keys["date_format"] {
		cur.DateFormat = in.DateFormat
	}
	if keys["time_format"] {
		cur.TimeFormat = in.TimeFormat
	}
	if keys["distance_unit"] {
		cur.DistanceUnit = in.DistanceUnit
	}
	if keys["fx_auto"] {
		cur.FXAuto = in.FXAuto
	}
	if keys["map_tiles"] {
		cur.MapTiles = in.MapTiles
	}
	// Secrets: a masked value (contains ••) means "keep".
	secret := func(k string, dst *string, v string) {
		if keys[k] && !strings.Contains(v, "••") {
			*dst = strings.TrimSpace(v)
		}
	}
	if keys["firefly_url"] {
		cur.FireflyURL = strings.TrimRight(strings.TrimSpace(in.FireflyURL), "/")
	}
	secret("firefly_token", &cur.FireflyToken, in.FireflyToken)
	if keys["dawarich_url"] {
		cur.DawarichURL = strings.TrimRight(strings.TrimSpace(in.DawarichURL), "/")
	}
	secret("dawarich_api_key", &cur.DawarichAPIKey, in.DawarichAPIKey)
	if keys["immich_url"] {
		cur.ImmichURL = strings.TrimRight(strings.TrimSpace(in.ImmichURL), "/")
	}
	secret("immich_api_key", &cur.ImmichAPIKey, in.ImmichAPIKey)
	if keys["immich_public_url"] {
		cur.ImmichPublicURL = strings.TrimRight(strings.TrimSpace(in.ImmichPublicURL), "/")
	}
	if err := s.store.SaveSettings(r.Context(), cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	_ = s.store.CacheDelete(r.Context(), "trip:")
	writeJSON(w, http.StatusOK, s.publicSettings(cur))
}

func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in struct {
		Which string `json:"which"`
		URL   string `json:"url"`
		Key   string `json:"key"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var msg string
	var err error
	switch in.Which {
	case "firefly":
		key := in.Key
		if strings.Contains(key, "••") || key == "" {
			key = st.FireflyToken
		}
		var v string
		v, err = firefly.New(in.URL, key, 15*time.Second).About(ctx)
		msg = "Firefly III " + v
	case "dawarich":
		key := in.Key
		if strings.Contains(key, "••") || key == "" {
			key = st.DawarichAPIKey
		}
		c := dawarich.New(in.URL, key, 15*time.Second)
		err = c.Health(ctx)
		if err == nil {
			var pts []dawarich.Point
			pts, err = c.Points(ctx, time.Now().AddDate(0, 0, -30), time.Now(), 1)
			msg = "Dawarich reachable"
			if err == nil && len(pts) > 0 {
				msg += ", points in the last 30 days"
			}
		}
	case "immich":
		key := in.Key
		if strings.Contains(key, "••") || key == "" {
			key = st.ImmichAPIKey
		}
		var v string
		v, err = immich.New(in.URL, key, 15*time.Second).Version(ctx)
		msg = "Immich " + v
	default:
		writeErr(w, http.StatusBadRequest, "unknown integration")
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size, files int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	_ = filepath.WalkDir(s.cfg.FilesDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				files += fi.Size()
			}
		}
		return nil
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size, "files_bytes": files,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.Counts(r.Context()),
	})
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-travel-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	defer os.Remove(dst)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	http.ServeFile(w, r, dst)
}

// handleMeta returns the static lists the UI needs once.
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	tpl := make([]string, 0, len(PackingTemplates))
	for k := range PackingTemplates {
		tpl = append(tpl, k)
	}
	sort.Strings(tpl)
	writeJSON(w, http.StatusOK, map[string]any{
		"place_categories":    store.PlaceCategories,
		"expense_categories":  store.ExpenseCategories,
		"discover_categories": geo.DiscoverCategories(),
		"packing_templates":   tpl,
		"lodging_types":       []string{"hotel", "apartment", "hostel", "house", "camping", "friends", "other"},
		"transport_types":     []string{"flight", "train", "bus", "car", "ferry", "taxi", "bike", "walk", "other"},
	})
}

// ---- home ----

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	stats, err := s.store.HomeStats(ctx, u.ID)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	trips, _ := s.store.ListTrips(ctx, store.TripFilter{UserID: u.ID})
	today := store.Today()
	var current, next *store.Trip
	recent := []store.Trip{}
	upcoming := []store.Trip{}
	for i := range trips {
		t := &trips[i]
		if t.Archived || t.Status == store.StatusCancelled {
			continue
		}
		t.Counts, _ = s.store.TripCounts(ctx, t.ID)
		switch {
		case t.StartDate != "" && t.StartDate <= today && t.EndDate >= today:
			if current == nil {
				current = t
			}
		case t.StartDate > today:
			upcoming = append(upcoming, *t)
		default:
			if len(recent) < 6 {
				recent = append(recent, *t)
			}
		}
	}
	sort.Slice(upcoming, func(i, j int) bool { return upcoming[i].StartDate < upcoming[j].StartDate })
	if len(upcoming) > 0 {
		next = &upcoming[0]
	}
	out := map[string]any{"stats": stats, "current": current, "next": next, "upcoming": upcoming, "recent": recent}
	// Today's plan for a running trip.
	if current != nil {
		places, _ := s.store.ListPlaces(ctx, current.ID)
		var todays []store.Place
		for _, p := range places {
			if p.DayDate == today {
				todays = append(todays, p)
			}
		}
		if todays == nil {
			todays = []store.Place{}
		}
		lodging, _ := s.store.ListLodging(ctx, current.ID)
		var stay *store.Lodging
		for i := range lodging {
			l := lodging[i]
			if len(l.CheckIn) >= 10 && len(l.CheckOut) >= 10 && l.CheckIn[:10] <= today && today < l.CheckOut[:10] {
				stay = &lodging[i]
			}
		}
		expenses, _ := s.store.ListExpenses(ctx, current.ID)
		out["today"] = map[string]any{"date": today, "day_number": store.DaysBetween(current.StartDate, today), "places": todays, "stay": stay, "budget": s.budgetSummary(current, expenses)}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDashboard is the Homepage widget: small, flat numbers.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	st, err := s.store.HomeStats(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	out := map[string]any{"trips": st.Trips, "upcoming": st.Upcoming, "days_traveled": st.Days, "places": st.Places, "next_trip": "", "next_in_days": 0, "next_trip_text": "no trip planned"}
	if st.NextTripID > 0 {
		if t, err := s.store.GetTrip(r.Context(), st.NextTripID, u.ID); err == nil {
			d, _ := store.ParseDate(t.StartDate)
			days := int(time.Until(d).Hours()/24) + 1
			out["next_trip"] = t.Name
			out["next_in_days"] = days
			out["next_trip_text"] = fmt.Sprintf("%s in %d days", t.Name, days)
		}
	}
	trips, _ := s.store.ListTrips(r.Context(), store.TripFilter{UserID: u.ID})
	today := store.Today()
	for _, t := range trips {
		if !t.Archived && t.StartDate != "" && t.StartDate <= today && t.EndDate >= today {
			out["current_trip"] = t.Name
			out["current_day"] = store.DaysBetween(t.StartDate, today)
			out["next_trip_text"] = fmt.Sprintf("%s · day %d of %d", t.Name, store.DaysBetween(t.StartDate, today), t.Days)
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- geo ----

func (s *Server) handleGeoSearch(w http.ResponseWriter, r *http.Request) {
	res, err := s.geo.Search(r.Context(), r.URL.Query().Get("q"), queryInt(r, "limit"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "search failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGeoReverse(w http.ResponseWriter, r *http.Request) {
	lat, ok1 := queryFloat(r, "lat")
	lng, ok2 := queryFloat(r, "lng")
	if !ok1 || !ok2 {
		writeErr(w, http.StatusBadRequest, "lat and lng are required")
		return
	}
	res, err := s.geo.Reverse(r.Context(), lat, lng)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "reverse geocoding failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGeoDiscover(w http.ResponseWriter, r *http.Request) {
	lat, ok1 := queryFloat(r, "lat")
	lng, ok2 := queryFloat(r, "lng")
	if !ok1 || !ok2 {
		writeErr(w, http.StatusBadRequest, "lat and lng are required")
		return
	}
	cat := r.URL.Query().Get("category")
	if cat == "" {
		cat = "sight"
	}
	key := fmt.Sprintf("discover:%s:%.3f:%.3f:%d", cat, lat, lng, queryInt(r, "radius"))
	if v, at, ok := s.store.CacheGet(r.Context(), key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 24*time.Hour {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(v))
			return
		}
	}
	res, err := s.geo.Discover(r.Context(), lat, lng, queryInt(r, "radius"), cat)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "discover failed: "+err.Error())
		return
	}
	b, _ := json.Marshal(res)
	_ = s.store.CachePut(r.Context(), key, string(b))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) handleGeoWiki(w http.ResponseWriter, r *http.Request) {
	res, err := s.geo.Wikipedia(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("lang"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGeoWeather(w http.ResponseWriter, r *http.Request) {
	lat, ok1 := queryFloat(r, "lat")
	lng, ok2 := queryFloat(r, "lng")
	if !ok1 || !ok2 {
		writeErr(w, http.StatusBadRequest, "lat and lng are required")
		return
	}
	res, err := s.geo.Weather(r.Context(), lat, lng, r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// tripCenter picks coordinates for weather: the trip's own, else the first stay, else the first place.
func (s *Server) tripCenter(ctx context.Context, t *store.Trip) (float64, float64, bool) {
	if t.Lat != nil && t.Lng != nil {
		return *t.Lat, *t.Lng, true
	}
	if lodging, _ := s.store.ListLodging(ctx, t.ID); len(lodging) > 0 {
		for _, l := range lodging {
			if l.Lat != nil && l.Lng != nil {
				return *l.Lat, *l.Lng, true
			}
		}
	}
	if places, _ := s.store.ListPlaces(ctx, t.ID); len(places) > 0 {
		for _, p := range places {
			if p.Lat != nil && p.Lng != nil {
				return *p.Lat, *p.Lng, true
			}
		}
	}
	return 0, 0, false
}

func (s *Server) handleTripWeather(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	if t.StartDate == "" || t.EndDate == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	lat, lng, ok := s.tripCenter(r.Context(), t)
	if !ok {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	key := cacheKey(t.ID, fmt.Sprintf("weather:%.2f:%.2f:%s:%s", lat, lng, t.StartDate, t.EndDate))
	if v, at, ok := s.store.CacheGet(r.Context(), key); ok && !queryBool(r, "refresh") {
		if ts, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(ts) < 3*time.Hour {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(v))
			return
		}
	}
	res, err := s.geo.Weather(r.Context(), lat, lng, t.StartDate, t.EndDate)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	b, _ := json.Marshal(res)
	_ = s.store.CachePut(r.Context(), key, string(b))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

// ---- Dawarich ----

// tripWindow returns the trip's dates as instants in the server timezone (whole days).
func tripWindow(t *store.Trip) (time.Time, time.Time, bool) {
	if t.StartDate == "" || t.EndDate == "" {
		return time.Time{}, time.Time{}, false
	}
	loc := time.Local
	if t.Timezone != "" {
		if l, err := time.LoadLocation(t.Timezone); err == nil {
			loc = l
		}
	}
	a, err1 := time.ParseInLocation("2006-01-02", t.StartDate, loc)
	b, err2 := time.ParseInLocation("2006-01-02", t.EndDate, loc)
	if err1 != nil || err2 != nil {
		return time.Time{}, time.Time{}, false
	}
	return a, b.AddDate(0, 0, 1), true
}

// cacheTTL: finished trips change rarely, running ones every hour.
func cacheTTL(t *store.Trip) time.Duration {
	if t.EndDate != "" && t.EndDate < store.Today() {
		return 7 * 24 * time.Hour
	}
	return time.Hour
}

func (s *Server) cached(r *http.Request, t *store.Trip, key string, fetch func() (any, error)) ([]byte, error) {
	k := cacheKey(t.ID, key)
	if !queryBool(r, "refresh") {
		if v, at, ok := s.store.CacheGet(r.Context(), k); ok {
			if ts, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(ts) < cacheTTL(t) {
				return []byte(v), nil
			}
		}
	}
	v, err := fetch()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	_ = s.store.CachePut(r.Context(), k, string(b))
	return b, nil
}

// RouteResponse is the recorded route, simplified for the map.
type RouteResponse struct {
	Points     [][3]float64       `json:"points"` // lat, lng, ts
	RawCount   int                `json:"raw_count"`
	DistanceKm float64            `json:"distance_km"`
	Days       map[string]float64 `json:"days_km"`
	Bounds     *[4]float64        `json:"bounds,omitempty"` // minLat, minLng, maxLat, maxLng
}

func (s *Server) fetchRoute(ctx context.Context, dw *dawarich.Client, t *store.Trip) (*RouteResponse, error) {
	from, to, ok := tripWindow(t)
	if !ok {
		return &RouteResponse{Points: [][3]float64{}, Days: map[string]float64{}}, nil
	}
	pts, err := dw.Points(ctx, from, to, 400000)
	if err != nil {
		return nil, err
	}
	raw := make([]geo.Pt, 0, len(pts))
	for _, p := range pts {
		raw = append(raw, geo.Pt{Lat: p.Lat, Lng: p.Lng, TS: p.TS})
	}
	// Distance per day over the raw points, then simplify for drawing.
	days := map[string]float64{}
	loc := from.Location()
	for i := 1; i < len(raw); i++ {
		d := geo.Haversine(raw[i-1].Lat, raw[i-1].Lng, raw[i].Lat, raw[i].Lng)
		dt := raw[i].TS - raw[i-1].TS
		// A GPS jump (implied speed over 250 km/h) or a gap of more than a day is not travel.
		if dt <= 0 || dt > 24*3600 || (d > 1500 && d/float64(dt) > 70) {
			continue
		}
		day := time.Unix(raw[i].TS, 0).In(loc).Format("2006-01-02")
		days[day] += d / 1000
	}
	// Dawarich's timeline knows about tracks and gaps the raw points miss (a
	// phone that slept through a motorway); prefer its per-day distances.
	if tl, err := dw.Timeline(ctx, from, to); err == nil && len(tl) > 0 {
		tlDays := map[string]float64{}
		for _, d := range tl {
			if d.Summary.TotalDistance > 0 {
				tlDays[d.Date] = d.Summary.TotalDistance
			}
		}
		for k, v := range tlDays {
			if v > days[k] {
				days[k] = v
			}
		}
	}
	total := 0.0
	for _, v := range days {
		total += v
	}
	simp := geo.Simplify(raw, 15)
	if len(simp) > 6000 {
		simp = geo.Simplify(raw, 40)
	}
	out := &RouteResponse{Points: make([][3]float64, 0, len(simp)), RawCount: len(raw), DistanceKm: total, Days: days}
	for _, p := range simp {
		out.Points = append(out.Points, [3]float64{p.Lat, p.Lng, float64(p.TS)})
		if out.Bounds == nil {
			out.Bounds = &[4]float64{p.Lat, p.Lng, p.Lat, p.Lng}
		} else {
			if p.Lat < out.Bounds[0] {
				out.Bounds[0] = p.Lat
			}
			if p.Lng < out.Bounds[1] {
				out.Bounds[1] = p.Lng
			}
			if p.Lat > out.Bounds[2] {
				out.Bounds[2] = p.Lat
			}
			if p.Lng > out.Bounds[3] {
				out.Bounds[3] = p.Lng
			}
		}
	}
	return out, nil
}

func (s *Server) handleDawarichRoute(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	_, _, dw, _ := s.clients(r.Context())
	if !dw.Configured() || !t.Dawarich {
		writeJSON(w, http.StatusOK, RouteResponse{Points: [][3]float64{}, Days: map[string]float64{}})
		return
	}
	b, err := s.cached(r, t, "dawarich-route", func() (any, error) { return s.fetchRoute(r.Context(), dw, t) })
	if err != nil {
		writeErr(w, http.StatusBadGateway, "dawarich: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) handleDawarichTimeline(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	_, _, dw, _ := s.clients(r.Context())
	if !dw.Configured() || !t.Dawarich {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	b, err := s.cached(r, t, "dawarich-timeline", func() (any, error) {
		from, to, ok := tripWindow(t)
		if !ok {
			return []any{}, nil
		}
		days, err := dw.Timeline(r.Context(), from, to)
		if err != nil {
			return nil, err
		}
		if days == nil {
			days = []dawarich.Day{}
		}
		return days, nil
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "dawarich: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

// ---- Immich ----

func (s *Server) handleTripPhotos(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	_, _, _, im := s.clients(r.Context())
	if !im.Configured() || !t.Immich {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	b, err := s.cached(r, t, "immich-photos", func() (any, error) {
		from, to, ok := tripWindow(t)
		if !ok {
			return []any{}, nil
		}
		a, err := im.Search(r.Context(), from, to, 5000)
		if err != nil {
			return nil, err
		}
		if a == nil {
			a = []immich.Asset{}
		}
		return a, nil
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "immich: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) handlePhotoThumb(w http.ResponseWriter, r *http.Request) {
	_, _, _, im := s.clients(r.Context())
	if !im.Configured() {
		writeErr(w, http.StatusNotFound, "immich not configured")
		return
	}
	id := strings.Map(func(c rune) rune {
		if c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			return c
		}
		return -1
	}, chi.URLParam(r, "id"))
	if err := im.Thumbnail(r.Context(), w, id, r.URL.Query().Get("size")); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
	}
}

// ---- exports ----

func (s *Server) handleExportICS(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	s.writeICS(w, r, t)
}

func (s *Server) writeICS(w http.ResponseWriter, r *http.Request, t *store.Trip) {
	ctx := r.Context()
	places, _ := s.store.ListPlaces(ctx, t.ID)
	lodging, _ := s.store.ListLodging(ctx, t.ID)
	transport, _ := s.store.ListTransport(ctx, t.ID)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(t.Name)+`.ics"`)
	_, _ = w.Write([]byte(export.ICS(t, places, lodging, transport, s.cfg.BaseURL)))
}

// handleCalendarFeed serves the ICS for subscriptions: /cal/{id}.ics?token=pt_…
func (s *Server) handleCalendarFeed(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	u, err := s.store.GetUserByAPIToken(r.Context(), tok)
	if err != nil || u == nil {
		writeErr(w, http.StatusUnauthorized, "a valid API token is required (?token=…)")
		return
	}
	id := idParam(r, "tripID")
	if s.store.TripRole(r.Context(), id, u) == "" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	t, err := s.store.GetTrip(r.Context(), id, u.ID)
	if err != nil {
		s.fail(w, err, "trip")
		return
	}
	s.writeICS(w, r, t)
}

func safeName(s string) string {
	s = strings.Map(func(c rune) rune {
		if c == '"' || c == '/' || c == '\\' || c < 32 {
			return '-'
		}
		return c
	}, s)
	if s == "" {
		return "trip"
	}
	return s
}

func (s *Server) handleExportGPX(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	ctx := r.Context()
	places, _ := s.store.ListPlaces(ctx, t.ID)
	lodging, _ := s.store.ListLodging(ctx, t.ID)
	var route []export.RoutePoint
	if queryBool(r, "route") {
		_, _, dw, _ := s.clients(ctx)
		if dw.Configured() && t.Dawarich {
			if b, err := s.cached(r, t, "dawarich-route", func() (any, error) { return s.fetchRoute(ctx, dw, t) }); err == nil {
				var rr RouteResponse
				if json.Unmarshal(b, &rr) == nil {
					for _, p := range rr.Points {
						route = append(route, export.RoutePoint{Lat: p[0], Lng: p[1], TS: int64(p[2])})
					}
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(t.Name)+`.gpx"`)
	_, _ = w.Write([]byte(export.GPX(t, places, lodging, route)))
}

// TripExport is the portable JSON form of a trip.
type TripExport struct {
	Format     string               `json:"format"`
	Version    string               `json:"version"`
	ExportedAt string               `json:"exported_at"`
	Trip       *store.Trip          `json:"trip"`
	Places     []store.Place        `json:"places"`
	Lodging    []store.Lodging      `json:"lodging"`
	Transport  []store.Transport    `json:"transport"`
	Expenses   []store.Expense      `json:"expenses"`
	Checklists []store.Checklist    `json:"checklists"`
	Journal    []store.JournalEntry `json:"journal"`
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	ctx := r.Context()
	ex := TripExport{Format: "pocket-travel-trip", Version: Version, ExportedAt: store.Now(), Trip: t}
	ex.Places, _ = s.store.ListPlaces(ctx, t.ID)
	ex.Lodging, _ = s.store.ListLodging(ctx, t.ID)
	ex.Transport, _ = s.store.ListTransport(ctx, t.ID)
	ex.Expenses, _ = s.store.ListExpenses(ctx, t.ID)
	ex.Checklists, _ = s.store.ListChecklists(ctx, t.ID)
	ex.Journal, _ = s.store.ListJournal(ctx, t.ID)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(t.Name)+`.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(ex)
}

func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var ex TripExport
	if err := decode(r, &ex); err != nil || ex.Trip == nil {
		writeErr(w, http.StatusBadRequest, "not a Pocket Travel trip export")
		return
	}
	ctx := r.Context()
	t := *ex.Trip
	t.ID, t.OwnerID, t.CoverPath, t.ExternalID = 0, u.ID, "", ""
	if t.Currency == "" || !currency.Valid(t.Currency) {
		t.Currency = "EUR"
	}
	if !store.ValidStatus(t.Status) {
		t.Status = store.StatusPlanning
	}
	if err := s.store.CreateTrip(ctx, &t); err != nil {
		s.fail(w, err, "create trip")
		return
	}
	for _, p := range ex.Places {
		p.ID, p.TripID, p.ImagePath = 0, t.ID, ""
		_ = s.store.CreatePlace(ctx, &p)
	}
	for _, l := range ex.Lodging {
		l.ID, l.TripID, l.ImagePath = 0, t.ID, ""
		_ = s.store.CreateLodging(ctx, &l)
	}
	for _, tr := range ex.Transport {
		tr.ID, tr.TripID = 0, t.ID
		_ = s.store.CreateTransport(ctx, &tr)
	}
	for _, e := range ex.Expenses {
		e.ID, e.TripID = 0, t.ID
		if e.Source == "item" {
			e.LinkKind, e.LinkID, e.Source = "", 0, "manual"
		}
		_ = s.store.CreateExpense(ctx, &e)
	}
	for _, c := range ex.Checklists {
		nc := store.Checklist{TripID: t.ID, Name: c.Name, Kind: c.Kind}
		if err := s.store.CreateChecklist(ctx, &nc); err == nil {
			for _, it := range c.Items {
				_ = s.store.AddChecklistItem(ctx, t.ID, &store.ChecklistItem{ChecklistID: nc.ID, Text: it.Text, Done: it.Done})
			}
		}
	}
	for _, j := range ex.Journal {
		j.ID, j.TripID, j.UserID = 0, t.ID, u.ID
		_ = s.store.CreateJournal(ctx, &j)
	}
	s.store.LogActivity(ctx, t.ID, u.ID, "imported", "from JSON")
	out, _ := s.store.GetTrip(ctx, t.ID, u.ID)
	writeJSON(w, http.StatusCreated, out)
}
