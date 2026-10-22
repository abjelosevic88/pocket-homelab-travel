package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// aiClient follows the caller's provider choice and personal Core link or key.
func (s *Server) aiClient(ctx context.Context) *ai.Client {
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	c := ai.New(st.OpenAIBaseURL, st.OpenAIKey, st.OpenAIModel, 150*time.Second)
	c.Pocket, _ = s.inferenceFor(ctx, actorID(ctx), st.AIProvider, st.OpenAIKey != "")
	if c.Pocket != nil {
		c.Model = "pocket-ai"
	}
	return c
}

// tripBrief is the context every prompt starts with.
func (s *Server) tripBrief(ctx context.Context, t *store.Trip) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Trip: %s\nDestination: %s\n", t.Name, firstNonEmpty(t.Destination, "unknown"))
	if t.StartDate != "" {
		fmt.Fprintf(&b, "Dates: %s to %s (%d days)\n", t.StartDate, t.EndDate, t.Days)
	}
	if t.Description != "" {
		fmt.Fprintf(&b, "About: %s\n", strings.TrimSpace(t.Description))
	}
	if t.Lat != nil && t.Lng != nil {
		fmt.Fprintf(&b, "Centre: %.4f, %.4f\n", *t.Lat, *t.Lng)
	}
	lodging, _ := s.store.ListLodging(ctx, t.ID)
	for _, l := range lodging {
		fmt.Fprintf(&b, "Staying at: %s (%s) %s to %s", l.Name, l.Address, shortDate(l.CheckIn), shortDate(l.CheckOut))
		if l.Lat != nil && l.Lng != nil {
			fmt.Fprintf(&b, " at %.4f, %.4f", *l.Lat, *l.Lng)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func placesList(places []store.Place) string {
	if len(places) == 0 {
		return "(none yet)"
	}
	var b strings.Builder
	for _, p := range places {
		fmt.Fprintf(&b, "- #%d %s [%s]", p.ID, p.Name, p.Category)
		if p.Lat != nil && p.Lng != nil {
			fmt.Fprintf(&b, " at %.4f, %.4f", *p.Lat, *p.Lng)
		}
		if p.DayDate != "" {
			fmt.Fprintf(&b, " scheduled %s %s", p.DayDate, p.StartTime)
		}
		if p.Status == "booked" {
			b.WriteString(" (booked, keep its time)")
		}
		if p.Status == "visited" || p.Status == "skipped" {
			fmt.Fprintf(&b, " (%s)", p.Status)
		}
		if p.DurationMin > 0 {
			fmt.Fprintf(&b, " ~%d min", p.DurationMin)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Server) aiOrFail(w http.ResponseWriter, r *http.Request) *ai.Client {
	c := s.aiClient(r.Context())
	if !c.Configured() {
		writeErr(w, http.StatusBadRequest, "connect Pocket AI or add an OpenAI key under Settings → Integrations")
		return nil
	}
	return c
}

// ---- 1. suggest popular places ----

// Suggestion is one AI-proposed place, geocoded when possible.
type Suggestion struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Why      string   `json:"why"`
	Area     string   `json:"area"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	Address  string   `json:"address"`
	Website  string   `json:"website,omitempty"`
	Geocoded bool     `json:"geocoded"`
	Duration int      `json:"duration_min"`
	BestTime string   `json:"best_time"`
	Exists   bool     `json:"exists"`
}

func (s *Server) handleAISuggest(w http.ResponseWriter, r *http.Request) {
	c := s.aiOrFail(w, r)
	if c == nil {
		return
	}
	t := tripFrom(r.Context()).trip
	var in struct {
		Interests string `json:"interests"`
		Category  string `json:"category"`
		Count     int    `json:"count"`
		Near      string `json:"near"`
	}
	_ = decode(r, &in)
	if in.Count <= 0 || in.Count > 15 {
		in.Count = 8
	}
	ctx := r.Context()
	places, _ := s.store.ListPlaces(ctx, t.ID)
	system := `You are a well-travelled local guide. Suggest places worth visiting for the trip described.
Prefer genuinely popular, highly rated, real places (sights, nature, food, experiences) that match the dates and season.
Never repeat places the traveller already has. Mix famous must-sees with one or two local favourites.
Answer with a JSON object {"places":[{"name","category","why","area","lat","lng","website","duration_min","best_time"}]}.
category must be one of: ` + strings.Join(store.PlaceCategories, ", ") + `.
"why" is one or two sentences, concrete and specific, no marketing fluff. "area" is the town or neighbourhood.
lat/lng are your best estimate of the exact location as numbers. duration_min is a realistic visit length. best_time is e.g. "early morning", "sunset", "any".`
	user := s.tripBrief(ctx, t) + "\nAlready in the plan:\n" + placesList(places)
	if in.Interests != "" {
		user += "\nTraveller's interests / requests: " + in.Interests
	}
	if in.Category != "" {
		user += "\nOnly suggest places of category: " + in.Category
	}
	if in.Near != "" {
		user += "\nFocus on the area around: " + in.Near
	}
	user += fmt.Sprintf("\nSuggest %d places.", in.Count)
	var out struct {
		Places []struct {
			Name     string   `json:"name"`
			Category string   `json:"category"`
			Why      string   `json:"why"`
			Area     string   `json:"area"`
			Lat      *float64 `json:"lat"`
			Lng      *float64 `json:"lng"`
			Website  string   `json:"website"`
			Duration int      `json:"duration_min"`
			BestTime string   `json:"best_time"`
		} `json:"places"`
	}
	if err := c.JSON(ctx, system, user, &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	have := map[string]bool{}
	for _, p := range places {
		have[strings.ToLower(strings.TrimSpace(p.Name))] = true
	}
	res := make([]Suggestion, 0, len(out.Places))
	for i, p := range out.Places {
		sg := Suggestion{Name: strings.TrimSpace(p.Name), Category: p.Category, Why: p.Why, Area: p.Area, Lat: p.Lat, Lng: p.Lng, Website: p.Website, Duration: p.Duration, BestTime: p.BestTime}
		if sg.Name == "" {
			continue
		}
		if !validCategory(sg.Category) {
			sg.Category = "sight"
		}
		sg.Exists = have[strings.ToLower(sg.Name)]
		// Pin it down with Nominatim (politely: one request a second) and keep
		// the model's guess only when the geocoder finds nothing close.
		if i > 0 {
			time.Sleep(1100 * time.Millisecond)
		}
		// Two tries: "name, area" then "name, destination"; a hit must be
		// within 60 km of the model's own guess so a namesake elsewhere loses.
		queries := []string{sg.Name + ", " + firstNonEmpty(sg.Area, t.Destination)}
		if sg.Area != "" && t.Destination != "" && !strings.EqualFold(sg.Area, t.Destination) {
			queries = append(queries, sg.Name+", "+t.Destination)
		}
		for qi, q := range queries {
			if qi > 0 {
				time.Sleep(1100 * time.Millisecond)
			}
			hits, err := s.geo.Search(ctx, q, 3)
			if err != nil {
				break
			}
			for _, h := range hits {
				if sg.Lat != nil && sg.Lng != nil && geo.Haversine(*sg.Lat, *sg.Lng, h.Lat, h.Lng) > 60000 {
					continue
				}
				lat, lng := h.Lat, h.Lng
				sg.Lat, sg.Lng, sg.Address, sg.Geocoded = &lat, &lng, h.Display, true
				if sg.Website == "" {
					sg.Website = h.Website
				}
				break
			}
			if sg.Geocoded {
				break
			}
		}
		res = append(res, sg)
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": res, "model": c.Model})
}

func validCategory(c string) bool {
	for _, x := range store.PlaceCategories {
		if x == c {
			return true
		}
	}
	return false
}

// ---- 2. plan the days ----

// FlexID is an int64 that also accepts a quoted number (models do that).
type FlexID int64

// UnmarshalJSON accepts 12 or "12".
func (f *FlexID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return err
	}
	*f = FlexID(n)
	return nil
}

// PlanItem is one proposed slot.
type PlanItem struct {
	PlaceID     FlexID `json:"place_id"`
	Name        string `json:"name"`
	DayDate     string `json:"day_date"`
	StartTime   string `json:"start_time"`
	DurationMin int    `json:"duration_min"`
	Note        string `json:"note"`
}

func (s *Server) handleAIPlan(w http.ResponseWriter, r *http.Request) {
	c := s.aiOrFail(w, r)
	if c == nil {
		return
	}
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	if t.StartDate == "" || t.EndDate == "" {
		writeErr(w, http.StatusBadRequest, "set the trip dates first")
		return
	}
	var in struct {
		Apply      bool       `json:"apply"`
		Pace       string     `json:"pace"` // relaxed | normal | packed
		Notes      string     `json:"notes"`
		Reschedule bool       `json:"reschedule"` // also move already scheduled (unbooked) places
		Items      []PlanItem `json:"items"`      // when apply=true: the (possibly edited) proposal
	}
	_ = decode(r, &in)
	ctx := r.Context()
	if in.Apply {
		// Apply a proposal: group by day, keep order by start time.
		byDay := map[string][]PlanItem{}
		for _, it := range in.Items {
			if it.PlaceID == 0 || !validDate(it.DayDate) {
				continue
			}
			byDay[it.DayDate] = append(byDay[it.DayDate], it)
		}
		n := 0
		for day, items := range byDay {
			sort.Slice(items, func(i, j int) bool { return items[i].StartTime < items[j].StartTime })
			ids := make([]int64, 0, len(items))
			for _, it := range items {
				p, err := s.store.GetPlace(ctx, t.ID, int64(it.PlaceID))
				if err != nil {
					continue
				}
				p.StartTime = it.StartTime
				if it.DurationMin > 0 {
					p.DurationMin = it.DurationMin
				}
				if it.Note != "" && !strings.Contains(p.Notes, it.Note) {
					p.Notes = strings.TrimSpace(p.Notes + "\n" + it.Note)
				}
				_ = s.store.UpdatePlace(ctx, p)
				ids = append(ids, p.ID)
				n++
			}
			_ = s.store.ReorderPlaces(ctx, t.ID, day, ids)
		}
		s.store.LogActivity(ctx, t.ID, userFrom(ctx).ID, "planned", fmt.Sprintf("%d places with AI", n))
		places, _ := s.store.ListPlaces(ctx, t.ID)
		writeJSON(w, http.StatusOK, map[string]any{"applied": n, "places": places})
		return
	}
	places, _ := s.store.ListPlaces(ctx, t.ID)
	transport, _ := s.store.ListTransport(ctx, t.ID)
	var movable, fixed []store.Place
	for _, p := range places {
		switch {
		case p.Status == "visited" || p.Status == "skipped":
		case p.Status == "booked" && p.DayDate != "":
			fixed = append(fixed, p)
		case p.DayDate == "" || in.Reschedule:
			movable = append(movable, p)
		default:
			fixed = append(fixed, p)
		}
	}
	if len(movable) == 0 {
		writeErr(w, http.StatusBadRequest, "nothing to plan — every place already has a day (tick “also move scheduled places” to redo them)")
		return
	}
	pace := map[string]string{"relaxed": "2–3 things a day, long lunches, no early starts", "packed": "make the most of every day, 5+ stops, early starts", "": "3–4 things a day with time to wander"}[in.Pace]
	if pace == "" {
		pace = "3–4 things a day with time to wander"
	}
	system := `You are an experienced travel planner. Build a day-by-day itinerary by assigning each movable place a day and a start time.
Rules: group places that are close together on the same day (use the coordinates); respect fixed items and their times; keep arrival/departure days light;
beaches and viewpoints in good light, restaurants at meal times, museums when they are open (assume 10:00–18:00), markets in the morning;
leave realistic travel time between stops; never schedule outside the trip dates. If there are more places than fit comfortably, leave the least important ones unscheduled.
Answer with a JSON object {"items":[{"place_id","day_date":"YYYY-MM-DD","start_time":"HH:MM","duration_min","note"}],"summary":"two sentences on the logic"}.
"note" is a short practical tip or empty.`
	user := s.tripBrief(ctx, t) + "Pace: " + pace + "\n"
	if in.Notes != "" {
		user += "Traveller's wishes: " + in.Notes + "\n"
	}
	if len(transport) > 0 {
		user += "Transport (fixed):\n"
		for _, tr := range transport {
			user += fmt.Sprintf("- %s %s → %s departs %s arrives %s\n", tr.Type, tr.FromName, tr.ToName, tr.DepartAt, tr.ArriveAt)
		}
	}
	user += "Fixed places (do not move):\n" + placesList(fixed) + "Movable places to schedule:\n" + placesList(movable)
	var out struct {
		Items   []PlanItem `json:"items"`
		Summary string     `json:"summary"`
	}
	if err := c.JSON(ctx, system, user, &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	byID := map[int64]store.Place{}
	for _, p := range movable {
		byID[p.ID] = p
	}
	items := make([]PlanItem, 0, len(out.Items))
	for _, it := range out.Items {
		p, ok := byID[int64(it.PlaceID)]
		if !ok || !validDate(it.DayDate) || it.DayDate < t.StartDate || it.DayDate > t.EndDate {
			continue
		}
		it.Name = p.Name
		if len(it.StartTime) != 5 {
			it.StartTime = ""
		}
		if it.DurationMin <= 0 {
			it.DurationMin = p.DurationMin
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].DayDate < items[j].DayDate || (items[i].DayDate == items[j].DayDate && items[i].StartTime < items[j].StartTime)
	})
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "summary": out.Summary, "unplaced": len(movable) - len(items), "model": c.Model})
}

// ---- 3. describe a place ----

func (s *Server) handleAIDescribe(w http.ResponseWriter, r *http.Request) {
	c := s.aiOrFail(w, r)
	if c == nil {
		return
	}
	t := tripFrom(r.Context()).trip
	var in struct {
		PlaceID  int64  `json:"place_id"`
		Name     string `json:"name"`
		Address  string `json:"address"`
		Category string `json:"category"`
	}
	_ = decode(r, &in)
	if in.PlaceID > 0 {
		if p, err := s.store.GetPlace(r.Context(), t.ID, in.PlaceID); err == nil {
			in.Name, in.Address, in.Category = p.Name, p.Address, p.Category
		}
	}
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	system := `You write short, useful travel notes. Answer with a JSON object {"description":"...", "tips":"...", "duration_min": 90, "best_time": "..."}.
description: 3–5 sentences on what the place is and why it is worth the time — factual, no hype. If you are not sure the place exists, say so in the description.
tips: 2–4 practical lines (tickets, when to go, what to order, how to get there, what to avoid), separated by newlines.`
	user := fmt.Sprintf("Place: %s\nCategory: %s\nAddress: %s\n%s", in.Name, in.Category, in.Address, s.tripBrief(r.Context(), t))
	var out struct {
		Description string `json:"description"`
		Tips        string `json:"tips"`
		Duration    int    `json:"duration_min"`
		BestTime    string `json:"best_time"`
	}
	if err := c.JSON(r.Context(), system, user, &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- 4. packing list ----

func (s *Server) handleAIPacking(w http.ResponseWriter, r *http.Request) {
	c := s.aiOrFail(w, r)
	if c == nil {
		return
	}
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	var in struct {
		Name   string `json:"name"`
		Notes  string `json:"notes"` // who is travelling, activities, constraints
		Create bool   `json:"create"`
	}
	_ = decode(r, &in)
	ctx := r.Context()
	places, _ := s.store.ListPlaces(ctx, t.ID)
	transport, _ := s.store.ListTransport(ctx, t.ID)
	wx := ""
	if lat, lng, ok := s.tripCenter(ctx, t); ok && t.StartDate != "" {
		if days, err := s.geo.Weather(ctx, lat, lng, t.StartDate, t.EndDate); err == nil && len(days) > 0 {
			lo, hi, rain := days[0].TMin, days[0].TMax, 0.0
			for _, d := range days {
				if d.TMin < lo {
					lo = d.TMin
				}
				if d.TMax > hi {
					hi = d.TMax
				}
				rain += d.Precip
			}
			wx = fmt.Sprintf("Weather outlook: %.0f–%.0f °C, %.0f mm rain over the period.\n", lo, hi, rain)
		}
	}
	cats := map[string]int{}
	for _, p := range places {
		cats[p.Category]++
	}
	modes := map[string]bool{}
	for _, tr := range transport {
		modes[tr.Type] = true
	}
	system := `You write packing lists that are specific to the trip, not generic. Answer with a JSON object {"groups":[{"name":"Documents","items":["..."]}]}.
6–9 groups (documents & money, clothes, toiletries, tech, health, activity-specific, for the car/flight, kids if mentioned, misc). Items are short nouns, one thing per item, 5–12 per group.
Consider the season and weather, the planned activities, the transport, the length of the trip and the destination's quirks (adapter type, cash vs card, mosquito season, sun).`
	user := s.tripBrief(ctx, t) + wx + fmt.Sprintf("Planned activity categories: %v\nTransport modes: %v\n", cats, keys(modes))
	if in.Notes != "" {
		user += "Travellers and notes: " + in.Notes + "\n"
	}
	var out struct {
		Groups []struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		} `json:"groups"`
	}
	if err := c.JSON(ctx, system, user, &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if !in.Create {
		writeJSON(w, http.StatusOK, out)
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Packing"
	}
	cl := &store.Checklist{TripID: t.ID, Name: name, Kind: "packing"}
	if err := s.store.CreateChecklist(ctx, cl); err != nil {
		s.fail(w, err, "create checklist")
		return
	}
	for _, g := range out.Groups {
		for _, item := range g.Items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			it := store.ChecklistItem{ChecklistID: cl.ID, Text: g.Name + ": " + item}
			if len(out.Groups) == 1 {
				it.Text = item
			}
			if err := s.store.AddChecklistItem(ctx, t.ID, &it); err == nil {
				cl.Items = append(cl.Items, it)
			}
		}
	}
	s.store.LogActivity(ctx, t.ID, userFrom(ctx).ID, "added checklist", cl.Name+" (AI)")
	writeJSON(w, http.StatusCreated, cl)
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- 5. recap ----

func (s *Server) handleAIRecap(w http.ResponseWriter, r *http.Request) {
	c := s.aiOrFail(w, r)
	if c == nil {
		return
	}
	t := tripFrom(r.Context()).trip
	var in struct {
		Style string `json:"style"` // story | summary | social
		Save  bool   `json:"save"`
	}
	_ = decode(r, &in)
	ctx := r.Context()
	places, _ := s.store.ListPlaces(ctx, t.ID)
	journal, _ := s.store.ListJournal(ctx, t.ID)
	expenses, _ := s.store.ListExpenses(ctx, t.ID)
	b := s.budgetSummary(t, expenses)
	var sb strings.Builder
	sb.WriteString(s.tripBrief(ctx, t))
	sb.WriteString("Places visited:\n")
	for _, p := range places {
		if p.Status == "visited" {
			fmt.Fprintf(&sb, "- %s (%s)%s\n", p.Name, p.Category, ratingText(p.Rating))
		}
	}
	sb.WriteString("Places planned but not visited:\n")
	for _, p := range places {
		if p.Status != "visited" && p.DayDate != "" {
			fmt.Fprintf(&sb, "- %s\n", p.Name)
		}
	}
	if len(journal) > 0 {
		sb.WriteString("Journal:\n")
		for _, j := range journal {
			fmt.Fprintf(&sb, "[%s] %s %s: %s\n", j.Date, j.Mood, j.Title, strings.TrimSpace(j.Body))
		}
	}
	if b.Actual > 0 {
		fmt.Fprintf(&sb, "Spent: %s %.2f in total", t.Currency, float64(b.Actual)/float64(pow10(currency.Decimals(t.Currency))))
		for _, ct := range b.ByCategory {
			if ct.Actual > 0 {
				fmt.Fprintf(&sb, "; %s %.2f", ct.Category, float64(ct.Actual)/float64(pow10(currency.Decimals(t.Currency))))
			}
		}
		sb.WriteString("\n")
	}
	if v, _, ok := s.store.CacheGet(ctx, userCacheKey(t.ID, actorID(ctx), "dawarich-route")); ok {
		var rr RouteResponse
		if jsonUnmarshal([]byte(v), &rr) == nil && rr.DistanceKm > 0 {
			fmt.Fprintf(&sb, "Distance travelled (GPS): %.0f km over %d tracked days\n", rr.DistanceKm, len(rr.Days))
		}
	}
	style := map[string]string{
		"story":   "Write a warm, first-person travel story of 4–6 paragraphs, in the voice of the traveller, using the journal's details and tone. No bullet points.",
		"social":  "Write a short Instagram-style caption (under 120 words) plus 5 hashtags.",
		"summary": "Write a factual recap: a paragraph of highlights, then short sections 'What we did', 'What it cost', 'What we'd do differently', using markdown headings and bullets.",
	}[in.Style]
	if style == "" {
		style = "Write a warm, first-person travel story of 4–6 paragraphs, in the voice of the traveller, using the journal's details and tone. No bullet points."
	}
	system := "You turn trip data into text. Use only the facts given; do not invent events, prices or places. " + style
	text, err := c.Complete(ctx, system, sb.String(), false)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	res := map[string]any{"text": text, "model": c.Model}
	if in.Save {
		j := &store.JournalEntry{TripID: t.ID, UserID: userFrom(ctx).ID, Date: firstNonEmpty(t.EndDate, store.Today()), Title: "Recap", Body: text, Mood: "✨"}
		if err := s.store.CreateJournal(ctx, j); err == nil {
			res["journal_id"] = j.ID
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func ratingText(r *float64) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf(" rated %.0f/5", *r)
}

func pow10(n int) int {
	p := 1
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}
