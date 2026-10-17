package server

import (
	"context"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

func f(v float64) *float64 { return &v }

// seedDemo creates a user and two trips (one running now, one past) so the
// UI has something to show. Idempotent.
func (s *Server) seedDemo(ctx context.Context) error {
	if n, _ := s.store.CountUsers(ctx); n > 0 {
		if trips, _ := s.store.ListTrips(ctx, store.TripFilter{}); len(trips) > 0 {
			return nil
		}
	}
	u, err := s.store.GetUserByEmail(ctx, "admin@localhost")
	if err != nil {
		u = &store.User{Email: "admin@localhost", Name: "Ana", Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
	}
	m := &store.User{Email: "marko@localhost", Name: "Marko", Role: "member"}
	_ = s.store.CreateUser(ctx, m)
	today := time.Now()
	d := func(off int) string { return today.AddDate(0, 0, off).Format("2006-01-02") }

	// A running trip: Montenegro coast, started 3 days ago.
	t := &store.Trip{OwnerID: u.ID, Name: "Luštica 2026", Destination: "Montenegro", Description: "Two weeks on the Luštica peninsula: beaches, the Bay of Kotor and too much seafood.", StartDate: d(-3), EndDate: d(10), Status: store.StatusActive, Currency: "EUR", Budget: 320000, Lat: f(42.3866), Lng: f(18.6642), Timezone: "Europe/Podgorica", FireflyTag: "lustica 2026", Dawarich: true, Immich: true, Color: "#0284c7"}
	if err := s.store.CreateTrip(ctx, t); err != nil {
		return err
	}
	_ = s.store.SetTripMember(ctx, t.ID, m.ID, "editor")
	places := []store.Place{
		{Name: "Plavi Horizonti beach", Category: "beach", Address: "Pržno, Luštica", Lat: f(42.3836), Lng: f(18.6579), DayDate: d(-2), StartTime: "10:00", DurationMin: 240, Status: "visited", VisitedAt: d(-2), Rating: f(5), Notes: "Shallow, warm, sandy. Get there before 10 for a spot."},
		{Name: "Kotor old town", Category: "sight", Address: "Kotor", Lat: f(42.4247), Lng: f(18.7712), DayDate: d(-1), StartTime: "09:30", DurationMin: 300, Status: "visited", VisitedAt: d(-1), Link: "https://en.wikipedia.org/wiki/Kotor", Price: 1500, Currency: "EUR"},
		{Name: "Konoba Ćatovića Mlini", Category: "food", Address: "Morinj", Lat: f(42.4902), Lng: f(18.6498), DayDate: d(0), StartTime: "19:30", DurationMin: 120, Status: "booked", Phone: "+382 32 373 030", BookingRef: "table for 4"},
		{Name: "Blue Cave boat tour", Category: "activity", Address: "Herceg Novi marina", Lat: f(42.4530), Lng: f(18.5370), DayDate: d(1), StartTime: "08:30", DurationMin: 360, Status: "booked", Price: 12000, Currency: "EUR", Link: "https://www.bokaboat.me/"},
		{Name: "Mamula island", Category: "sight", Lat: f(42.3952), Lng: f(18.5590), DayDate: d(1), StartTime: "14:30", Status: "planned"},
		{Name: "Perast & Our Lady of the Rocks", Category: "sight", Address: "Perast", Lat: f(42.4869), Lng: f(18.6995), DayDate: d(2), StartTime: "10:00", DurationMin: 240, Status: "planned", Price: 800, Currency: "EUR"},
		{Name: "Lovćen mausoleum", Category: "viewpoint", Lat: f(42.3990), Lng: f(18.8375), DayDate: d(4), StartTime: "09:00", DurationMin: 360, Status: "planned", Notes: "461 steps. Go early, clouds roll in after lunch."},
		{Name: "Žanjic beach", Category: "beach", Lat: f(42.3975), Lng: f(18.5945), Status: "idea"},
		{Name: "Forte Rose", Category: "food", Address: "Rose", Lat: f(42.4287), Lng: f(18.5562), Status: "idea", Link: "https://forterose.me/"},
		{Name: "Tivat Porto Montenegro", Category: "shop", Lat: f(42.4353), Lng: f(18.6934), Status: "idea"},
	}
	for i := range places {
		places[i].TripID = t.ID
		if places[i].Currency == "" {
			places[i].Currency = "EUR"
		}
		_ = s.store.CreatePlace(ctx, &places[i])
	}
	l := &store.Lodging{TripID: t.ID, Name: "Luštica Bay Vacation Homes", Type: "apartment", Address: "Novo Naselje b.b., Radovići, Tivat", Lat: f(42.3866), Lng: f(18.6642), CheckIn: d(-3), CheckOut: d(11), ReservationNumber: "123171", Link: "https://lusticabay.com/", Phone: "+382 67 070 104", Price: 168000, Currency: "EUR", Paid: true, Rating: f(4.8)}
	_ = s.store.CreateLodging(ctx, l)
	s.syncItemCost(ctx, t, "lodging", l.ID, l.Name, "lodging", l.Price, l.Currency, l.CheckIn, l.Paid)
	tr := []store.Transport{
		{TripID: t.ID, Type: "car", Name: "Drive down", FromName: "Banja Luka", FromLat: f(44.7722), FromLng: f(17.1910), ToName: "Luštica Bay", ToLat: f(42.3866), ToLng: f(18.6642), DepartAt: d(-3) + "T06:30", ArriveAt: d(-3) + "T13:00", DistanceKm: 420, Price: 9000, Currency: "EUR", Notes: "Via Trebinje, coffee stop in Bileća."},
		{TripID: t.ID, Type: "ferry", Name: "Kamenari–Lepetane ferry", FromName: "Lepetane", ToName: "Kamenari", DepartAt: d(2) + "T09:15", Price: 450, Currency: "EUR"},
		{TripID: t.ID, Type: "car", Name: "Drive home", FromName: "Luštica Bay", FromLat: f(42.3866), FromLng: f(18.6642), ToName: "Banja Luka", ToLat: f(44.7722), ToLng: f(17.1910), DepartAt: d(11) + "T08:00", ArriveAt: d(11) + "T15:00", DistanceKm: 420, Price: 9000, Currency: "EUR"},
	}
	for i := range tr {
		_ = s.store.CreateTransport(ctx, &tr[i])
		s.syncItemCost(ctx, t, "transport", tr[i].ID, transportLabel(&tr[i]), "transport", tr[i].Price, tr[i].Currency, tr[i].DepartAt, tr[i].Paid)
	}
	exp := []store.Expense{
		{Name: "Travel insurance", Category: "insurance", Amount: 2710, Currency: "EUR", Kind: "planned", Date: d(-10)},
		{Name: "Food budget", Category: "food", Amount: 70000, Currency: "EUR", Kind: "planned"},
		{Name: "Boat tours", Category: "activities", Amount: 20000, Currency: "EUR", Kind: "planned"},
		{Name: "Fuel Bileća", Category: "fuel", Amount: 6540, Currency: "BAM", Kind: "actual", Date: d(-3), Source: "firefly", ExternalID: "demo-1"},
		{Name: "Konoba Portun", Category: "food", Amount: 4820, Currency: "EUR", Kind: "actual", Date: d(-3), Source: "firefly", ExternalID: "demo-2"},
		{Name: "Voli supermarket", Category: "groceries", Amount: 6315, Currency: "EUR", Kind: "actual", Date: d(-2), Source: "firefly", ExternalID: "demo-3"},
		{Name: "Kotor city walls", Category: "activities", Amount: 3000, Currency: "EUR", Kind: "actual", Date: d(-1), Source: "firefly", ExternalID: "demo-4"},
		{Name: "Pizzeria Pronto", Category: "food", Amount: 2650, Currency: "EUR", Kind: "actual", Date: d(-1), Source: "firefly", ExternalID: "demo-5"},
		{Name: "Parking Kotor", Category: "transport", Amount: 600, Currency: "EUR", Kind: "actual", Date: d(-1), Source: "firefly", ExternalID: "demo-6"},
		{Name: "Ice cream", Category: "food", Amount: 700, Currency: "EUR", Kind: "actual", Date: d(0), Source: "firefly", ExternalID: "demo-7"},
	}
	for i := range exp {
		exp[i].TripID = t.ID
		rate, _ := s.rate(ctx, exp[i].Currency, "EUR", exp[i].Date)
		exp[i].FXRate = rate
		exp[i].AmountHome = convert(exp[i].Amount, exp[i].Currency, "EUR", rate)
		_ = s.store.CreateExpense(ctx, &exp[i])
	}
	pack := &store.Checklist{TripID: t.ID, Name: "Packing", Kind: "packing"}
	_ = s.store.CreateChecklist(ctx, pack)
	for i, it := range PackingTemplates["beach"] {
		_ = s.store.AddChecklistItem(ctx, t.ID, &store.ChecklistItem{ChecklistID: pack.ID, Text: it, Done: i < 6})
	}
	todo := &store.Checklist{TripID: t.ID, Name: "Before we leave", Kind: "todo"}
	_ = s.store.CreateChecklist(ctx, todo)
	for _, it := range PackingTemplates["todo"] {
		_ = s.store.AddChecklistItem(ctx, t.ID, &store.ChecklistItem{ChecklistID: todo.ID, Text: it, Done: true})
	}
	_ = s.store.CreateJournal(ctx, &store.JournalEntry{TripID: t.ID, UserID: u.ID, Date: d(-3), Title: "Arrived", Body: "Seven hours with one coffee stop. The apartment looks out over the marina and the water is absurdly clear. Dinner at Portun: grilled squid, a litre of Vranac.", Mood: "😊"})
	_ = s.store.CreateJournal(ctx, &store.JournalEntry{TripID: t.ID, UserID: m.ID, Date: d(-1), Title: "Kotor", Body: "Walls at 9am before the cruise ship crowd. 1,350 steps, worth every one. Marko found the cat museum.", Mood: "🥾"})

	// A finished trip from last spring.
	p := &store.Trip{OwnerID: u.ID, Name: "Vienna long weekend", Destination: "Austria", Description: "Museums, coffee houses, one opera.", StartDate: today.AddDate(0, -5, -4).Format("2006-01-02"), EndDate: today.AddDate(0, -5, 0).Format("2006-01-02"), Status: store.StatusCompleted, Currency: "EUR", Budget: 90000, Lat: f(48.2082), Lng: f(16.3738), Timezone: "Europe/Vienna", Dawarich: true, Immich: true, Color: "#7c3aed"}
	if err := s.store.CreateTrip(ctx, p); err != nil {
		return err
	}
	for _, pl := range []store.Place{
		{Name: "Kunsthistorisches Museum", Category: "museum", Lat: f(48.2038), Lng: f(16.3617), Status: "visited", Rating: f(5), DayDate: p.StartDate},
		{Name: "Café Central", Category: "cafe", Lat: f(48.2103), Lng: f(16.3655), Status: "visited", Rating: f(4), DayDate: p.StartDate},
		{Name: "Staatsoper — Tosca", Category: "event", Lat: f(48.2030), Lng: f(16.3690), Status: "visited", Price: 9600, Currency: "EUR", DayDate: p.EndDate},
		{Name: "Naschmarkt", Category: "shop", Lat: f(48.1985), Lng: f(16.3637), Status: "skipped"},
	} {
		pl.TripID = p.ID
		if pl.Currency == "" {
			pl.Currency = "EUR"
		}
		_ = s.store.CreatePlace(ctx, &pl)
	}
	pl := &store.Lodging{TripID: p.ID, Name: "Hotel Beethoven", Type: "hotel", Address: "Papagenogasse 6, Wien", Lat: f(48.1990), Lng: f(16.3650), CheckIn: p.StartDate, CheckOut: p.EndDate, Price: 54000, Currency: "EUR", Paid: true}
	_ = s.store.CreateLodging(ctx, pl)
	s.syncItemCost(ctx, p, "lodging", pl.ID, pl.Name, "lodging", pl.Price, pl.Currency, pl.CheckIn, pl.Paid)
	for _, e := range []store.Expense{
		{Name: "Opera tickets", Category: "activities", Amount: 19200, Kind: "actual", Date: p.StartDate},
		{Name: "Coffee & cake, repeatedly", Category: "food", Amount: 8700, Kind: "actual", Date: p.StartDate},
		{Name: "Train Vienna", Category: "transport", Amount: 15800, Kind: "actual", Date: p.StartDate},
	} {
		e.TripID, e.Currency, e.AmountHome, e.FXRate, e.Source = p.ID, "EUR", e.Amount, 1, "manual"
		_ = s.store.CreateExpense(ctx, &e)
	}

	// Something to look forward to.
	n := &store.Trip{OwnerID: u.ID, Name: "Lisbon in spring", Destination: "Portugal", StartDate: d(62), EndDate: d(68), Status: store.StatusPlanning, Currency: "EUR", Budget: 150000, Lat: f(38.7223), Lng: f(-9.1393), Timezone: "Europe/Lisbon", Dawarich: true, Immich: true, Color: "#d97706"}
	if err := s.store.CreateTrip(ctx, n); err != nil {
		return err
	}
	for _, pl := range []store.Place{
		{Name: "Belém Tower", Category: "sight", Lat: f(38.6916), Lng: f(-9.2160), Status: "idea"},
		{Name: "Time Out Market", Category: "food", Lat: f(38.7070), Lng: f(-9.1458), Status: "idea"},
		{Name: "Sintra day trip", Category: "activity", Lat: f(38.7979), Lng: f(-9.3907), Status: "idea", Notes: "Pena Palace tickets sell out — book the 9:30 slot."},
	} {
		pl.TripID, pl.Currency = n.ID, "EUR"
		_ = s.store.CreatePlace(ctx, &pl)
	}
	_ = s.store.CreateChecklist(ctx, &store.Checklist{TripID: n.ID, Name: "Packing", Kind: "packing"})
	bl := &store.Checklist{TripID: n.ID, Name: "Before we leave", Kind: "todo"}
	_ = s.store.CreateChecklist(ctx, bl)
	_ = s.store.AddChecklistItem(ctx, n.ID, &store.ChecklistItem{ChecklistID: bl.ID, Text: "Book flights"})
	_ = s.store.AddChecklistItem(ctx, n.ID, &store.ChecklistItem{ChecklistID: bl.ID, Text: "Pick a neighbourhood for the apartment"})
	s.log.Info("demo data seeded")
	return nil
}
