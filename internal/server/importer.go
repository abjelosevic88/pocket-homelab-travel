package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/adventurelog"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

type alLogin struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleALCollections(w http.ResponseWriter, r *http.Request) {
	var in alLogin
	if err := decode(r, &in); err != nil || in.URL == "" || in.Username == "" {
		writeErr(w, http.StatusBadRequest, "url, username and password are required")
		return
	}
	c, err := adventurelog.Login(r.Context(), in.URL, in.Username, in.Password)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	cols, err := c.Collections(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if cols == nil {
		cols = []adventurelog.CollectionSummary{}
	}
	writeJSON(w, http.StatusOK, cols)
}

// alCategory maps AdventureLog category names to ours.
func alCategory(name string) string {
	n := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	switch {
	case strings.Contains(n, "food"), strings.Contains(n, "restaurant"):
		return "food"
	case strings.Contains(n, "cafe"), strings.Contains(n, "coffee"):
		return "cafe"
	case strings.Contains(n, "bar"), strings.Contains(n, "night"):
		return "bar"
	case strings.Contains(n, "beach"):
		return "beach"
	case strings.Contains(n, "museum"), strings.Contains(n, "gallery"):
		return "museum"
	case strings.Contains(n, "view"):
		return "viewpoint"
	case strings.Contains(n, "nature"), strings.Contains(n, "park"), strings.Contains(n, "hik"), strings.Contains(n, "trail"):
		return "nature"
	case strings.Contains(n, "shop"), strings.Contains(n, "market"):
		return "shop"
	case strings.Contains(n, "hotel"), strings.Contains(n, "lodg"):
		return "other"
	case strings.Contains(n, "event"), strings.Contains(n, "festival"):
		return "event"
	case strings.Contains(n, "activity"), strings.Contains(n, "experience"), strings.Contains(n, "sport"), strings.Contains(n, "marina"):
		return "activity"
	case strings.Contains(n, "spa"), strings.Contains(n, "wellness"):
		return "wellness"
	case strings.Contains(n, "transport"), strings.Contains(n, "station"), strings.Contains(n, "airport"):
		return "transport"
	}
	return "sight"
}

func alTransport(t string) string {
	switch strings.ToLower(t) {
	case "plane", "flight", "air":
		return "flight"
	case "train", "rail":
		return "train"
	case "bus", "coach":
		return "bus"
	case "car", "drive", "driving":
		return "car"
	case "boat", "ferry", "ship":
		return "ferry"
	case "bike", "bicycle", "cycling":
		return "bike"
	case "walking", "walk", "hike":
		return "walk"
	case "taxi":
		return "taxi"
	}
	return "other"
}

func alLodging(t string) string {
	switch strings.ToLower(t) {
	case "hotel", "resort", "motel":
		return "hotel"
	case "apartment", "villa", "vacation_rental", "airbnb":
		return "apartment"
	case "hostel":
		return "hostel"
	case "house", "cabin":
		return "house"
	case "camping", "campsite", "camp":
		return "camping"
	}
	return "other"
}

func toMinor(price *string, cur *string, fallback string) (int64, string) {
	if price == nil || *price == "" {
		return 0, ""
	}
	f, err := strconv.ParseFloat(*price, 64)
	if err != nil || f <= 0 {
		return 0, ""
	}
	c := fallback
	if cur != nil && currency.Valid(*cur) {
		c = strings.ToUpper(*cur)
	}
	return int64(math.Round(f * math.Pow10(currency.Decimals(c)))), c
}

var markerRe = regexp.MustCompile(`<!--[^>]*-->`)

func (s *Server) handleALImport(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var in struct {
		alLogin
		Collections []string `json:"collections"`
		Images      bool     `json:"images"`
	}
	if err := decode(r, &in); err != nil || in.URL == "" || len(in.Collections) == 0 {
		writeErr(w, http.StatusBadRequest, "url, credentials and at least one collection are required")
		return
	}
	c, err := adventurelog.Login(r.Context(), in.URL, in.Username, in.Password)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	st, _ := s.store.GetSettings(r.Context())
	var results []map[string]any
	for _, id := range in.Collections {
		res, err := s.importCollection(r.Context(), c, u, id, in.Images, st.HomeCurrency)
		if err != nil {
			results = append(results, map[string]any{"collection": id, "error": err.Error()})
			continue
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) importCollection(ctx context.Context, c *adventurelog.Client, u *store.User, id string, images bool, homeCur string) (map[string]any, error) {
	col, err := c.Collection(ctx, id)
	if err != nil {
		return nil, err
	}
	// Re-import updates the existing trip's name/dates but never duplicates items with the same external id.
	trips, _ := s.store.ListTrips(ctx, store.TripFilter{UserID: u.ID})
	var t *store.Trip
	for i := range trips {
		if trips[i].ExternalID == "adventurelog:"+col.ID {
			t = &trips[i]
			break
		}
	}
	status := store.StatusPlanning
	if col.EndDate != "" && col.EndDate < store.Today() {
		status = store.StatusCompleted
	}
	if t == nil {
		t = &store.Trip{OwnerID: u.ID, Name: col.Name, Description: col.Description, StartDate: col.StartDate, EndDate: col.EndDate, Status: status, Currency: homeCur, Dawarich: true, Immich: true, Archived: col.IsArchived, ExternalID: "adventurelog:" + col.ID}
		if col.Link != nil {
			t.Link = *col.Link
		}
		if err := s.store.CreateTrip(ctx, t); err != nil {
			return nil, err
		}
	}
	existingPlaces, _ := s.store.ListPlaces(ctx, t.ID)
	seen := map[string]bool{}
	for _, p := range existingPlaces {
		seen[p.ExternalID] = true
	}
	existingLodging, _ := s.store.ListLodging(ctx, t.ID)
	for _, l := range existingLodging {
		seen[l.ExternalID] = true
	}
	existingTransport, _ := s.store.ListTransport(ctx, t.ID)
	for _, tr := range existingTransport {
		seen[tr.ExternalID] = true
	}
	counts := map[string]int{}
	fetchImage := func(imgs []adventurelog.Image) string {
		if !images || len(imgs) == 0 {
			return ""
		}
		pick := imgs[0].Image
		for _, im := range imgs {
			if im.IsPrimary {
				pick = im.Image
			}
		}
		if pick == "" {
			return ""
		}
		b, ct, err := c.Download(ctx, pick)
		if err != nil || !strings.HasPrefix(ct, "image/") {
			return ""
		}
		rel, err := s.saveFile(t.ID, extOf(pick, ct), b)
		if err != nil {
			return ""
		}
		return rel
	}
	for _, l := range col.Locations {
		if seen["al:"+l.ID] {
			continue
		}
		p := store.Place{TripID: t.ID, Name: l.Name, Description: l.Description, Status: "idea", ExternalID: "al:" + l.ID, Currency: t.Currency}
		if l.Category != nil {
			p.Category = alCategory(l.Category.Name + " " + l.Category.DisplayName)
		}
		if l.Location != nil {
			p.Address = *l.Location
		}
		if l.Link != nil {
			p.Link = *l.Link
		}
		p.Lat, p.Lng = l.Latitude, l.Longitude
		p.Rating = l.Rating
		p.Tags = strings.Join(l.Tags, ", ")
		p.Price, p.Currency = toMinor(l.Price, l.PriceCur, t.Currency)
		if p.Currency == "" {
			p.Currency = t.Currency
		}
		if l.IsVisited || len(l.Visits) > 0 {
			p.Status = "visited"
			if len(l.Visits) > 0 {
				p.VisitedAt = l.Visits[0].StartDate
				if len(l.Visits[0].StartDate) >= 10 {
					p.DayDate = l.Visits[0].StartDate[:10]
				}
				if l.Visits[0].Notes != "" {
					p.Notes = l.Visits[0].Notes
				}
			}
		}
		p.ImagePath = fetchImage(l.Images)
		if err := s.store.CreatePlace(ctx, &p); err == nil {
			counts["places"]++
		}
	}
	for _, l := range col.Lodging {
		if seen["al:"+l.ID] {
			continue
		}
		// A stay on a trip that already happened was paid; keep it as an estimate only for future trips.
		lo := store.Lodging{TripID: t.ID, Name: l.Name, Type: alLodging(l.Type), Notes: l.Description, Rating: l.Rating, Lat: l.Latitude, Lng: l.Longitude, ExternalID: "al:" + l.ID, Currency: t.Currency, Paid: status == store.StatusCompleted}
		if l.Location != nil {
			lo.Address = *l.Location
		}
		if l.Link != nil {
			lo.Link = *l.Link
		}
		if l.CheckIn != nil {
			lo.CheckIn = shortDate(*l.CheckIn)
		}
		if l.CheckOut != nil {
			lo.CheckOut = shortDate(*l.CheckOut)
		}
		if l.ReservationNumber != nil {
			lo.ReservationNumber = *l.ReservationNumber
		}
		lo.Price, lo.Currency = toMinor(l.Price, l.PriceCurrency, t.Currency)
		if lo.Currency == "" {
			lo.Currency = t.Currency
		}
		lo.ImagePath = fetchImage(l.Images)
		if err := s.store.CreateLodging(ctx, &lo); err == nil {
			counts["lodging"]++
			s.syncItemCost(ctx, t, "lodging", lo.ID, lo.Name, "lodging", lo.Price, lo.Currency, lo.CheckIn, lo.Paid)
		}
	}
	for _, tr := range col.Transportations {
		if seen["al:"+tr.ID] {
			continue
		}
		x := store.Transport{TripID: t.ID, Type: alTransport(tr.Type), Name: tr.Name, Notes: tr.Description, FromLat: tr.OriginLat, FromLng: tr.OriginLng, ToLat: tr.DestLat, ToLng: tr.DestLng, ExternalID: "al:" + tr.ID, Currency: t.Currency}
		if strings.HasPrefix(x.Name, "[Dawarich]") {
			// Recorded legs come from Dawarich live now; keep only the real plans.
			continue
		}
		if tr.Link != nil {
			x.Link = *tr.Link
		}
		if tr.FlightNumber != nil {
			x.Number = *tr.FlightNumber
		}
		if tr.FromLocation != nil {
			x.FromName = *tr.FromLocation
		}
		if tr.ToLocation != nil {
			x.ToName = *tr.ToLocation
		}
		if tr.Date != nil {
			x.DepartAt = localStamp(*tr.Date)
		}
		if tr.EndDate != nil {
			x.ArriveAt = localStamp(*tr.EndDate)
		}
		if tr.Distance != nil {
			x.DistanceKm = *tr.Distance
		}
		x.Price, x.Currency = toMinor(tr.Price, tr.PriceCurrency, t.Currency)
		if x.Currency == "" {
			x.Currency = t.Currency
		}
		if err := s.store.CreateTransport(ctx, &x); err == nil {
			counts["transport"]++
			s.syncItemCost(ctx, t, "transport", x.ID, transportLabel(&x), "transport", x.Price, x.Currency, x.DepartAt, x.Paid)
		}
	}
	existingJournal, _ := s.store.ListJournal(ctx, t.ID)
	jseen := map[string]bool{}
	for _, j := range existingJournal {
		jseen[j.Title] = true
	}
	for _, n := range col.Notes {
		content := strings.TrimSpace(markerRe.ReplaceAllString(n.Content, ""))
		if strings.Contains(n.Content, "firefly-adventurelog:") || strings.Contains(n.Content, "planned-costs:") {
			// Machine notes from the old n8n/planned-costs flow: Firefly is live here, planned costs were parsed below.
			s.importPlannedCosts(ctx, t, n.Content)
			continue
		}
		if jseen[n.Name] {
			continue
		}
		j := store.JournalEntry{TripID: t.ID, UserID: u.ID, Title: n.Name, Body: content, Date: t.StartDate}
		if n.Date != nil && len(*n.Date) >= 10 {
			j.Date = (*n.Date)[:10]
		}
		if len(n.Links) > 0 {
			j.Body += "\n\n" + strings.Join(n.Links, "\n")
		}
		if err := s.store.CreateJournal(ctx, &j); err == nil {
			counts["notes"]++
		}
	}
	existingLists, _ := s.store.ListChecklists(ctx, t.ID)
	lseen := map[string]bool{}
	for _, l := range existingLists {
		lseen[l.Name] = true
	}
	for _, cl := range col.Checklists {
		if lseen[cl.Name] {
			continue
		}
		nc := store.Checklist{TripID: t.ID, Name: cl.Name, Kind: "todo"}
		if err := s.store.CreateChecklist(ctx, &nc); err == nil {
			counts["checklists"]++
			for _, it := range cl.Items {
				_ = s.store.AddChecklistItem(ctx, t.ID, &store.ChecklistItem{ChecklistID: nc.ID, Text: it.Name, Done: it.Checked})
			}
		}
	}
	// Centre the trip on its places when it has none.
	if t.Lat == nil {
		if lat, lng, ok := s.tripCenter(ctx, t); ok {
			t.Lat, t.Lng = &lat, &lng
			_ = s.store.UpdateTrip(ctx, t)
		}
	}
	s.store.LogActivity(ctx, t.ID, u.ID, "imported", fmt.Sprintf("from AdventureLog: %d places, %d stays, %d legs, %d notes, %d checklists", counts["places"], counts["lodging"], counts["transport"], counts["notes"], counts["checklists"]))
	return map[string]any{"collection": col.ID, "trip_id": t.ID, "name": t.Name, "counts": counts}, nil
}

var plannedRe = regexp.MustCompile(`planned-costs-data:(\{.*?\})\s*-->`)

// importPlannedCosts parses the old planned-costs marker note into planned expenses.
func (s *Server) importPlannedCosts(ctx context.Context, t *store.Trip, content string) {
	m := plannedRe.FindStringSubmatch(content)
	if m == nil {
		return
	}
	var data struct {
		Items []struct {
			Name     string  `json:"name"`
			Amount   float64 `json:"amount"`
			Currency string  `json:"currency"`
		} `json:"items"`
	}
	if err := jsonUnmarshal([]byte(m[1]), &data); err != nil {
		return
	}
	existing, _ := s.store.ListExpenses(ctx, t.ID)
	have := map[string]bool{}
	for _, e := range existing {
		have[e.Name] = true
	}
	for _, it := range data.Items {
		if have[it.Name] || it.Amount <= 0 {
			continue
		}
		cur := strings.ToUpper(it.Currency)
		if !currency.Valid(cur) {
			cur = t.Currency
		}
		minor := int64(math.Round(it.Amount * math.Pow10(currency.Decimals(cur))))
		rate, _ := s.rate(ctx, cur, t.Currency, t.StartDate)
		_ = s.store.CreateExpense(ctx, &store.Expense{TripID: t.ID, Name: it.Name, Category: "other", Amount: minor, Currency: cur, AmountHome: currency.Convert(minor, cur, t.Currency, rate), FXRate: rate, Date: t.StartDate, Kind: "planned", Source: "manual"})
	}
}

func shortDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// localStamp turns 2026-09-11T15:10:39Z into 2026-09-11T15:10 (wall time as stored upstream).
func localStamp(s string) string {
	if len(s) >= 16 {
		return s[:16]
	}
	return shortDate(s)
}
