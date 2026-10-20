package server

import (
	"context"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

func f(v float64) *float64 { return &v }

// seedDemo creates a user and three trips (one running now, one past, one ahead) so the
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

	// A running trip: Barcelona, started 3 days ago.
	t := &store.Trip{OwnerID: u.ID, Name: "Barcelona 2026", Destination: "Barcelona, Spain", Description: "Ten days of Gaudí, tapas, the beach and one day in the mountains at Montserrat.", StartDate: d(-3), EndDate: d(6), Status: store.StatusActive, Currency: "EUR", Budget: 260000, Lat: f(41.3874), Lng: f(2.1686), Timezone: "Europe/Madrid", FireflyTag: "barcelona 2026", Dawarich: true, Immich: true, Color: "#0284c7"}
	if err := s.store.CreateTrip(ctx, t); err != nil {
		return err
	}
	_ = s.store.SetTripMember(ctx, t.ID, m.ID, "editor")
	places := []store.Place{
		{Name: "Sagrada Família", Category: "sight", Address: "Carrer de Mallorca 401", Lat: f(41.4036), Lng: f(2.1744), DayDate: d(-2), StartTime: "09:00", DurationMin: 120, Status: "visited", VisitedAt: d(-2), Rating: f(5), Price: 2600, Currency: "EUR", Link: "https://sagradafamilia.org/", BookingRef: "SF-88213", Notes: "Timed entry — the 9:00 slot had no queue. Tower ticket worth it."},
		{Name: "Park Güell", Category: "nature", Address: "Carrer d'Olot", Lat: f(41.4145), Lng: f(2.1527), DayDate: d(-2), StartTime: "12:30", DurationMin: 120, Status: "visited", VisitedAt: d(-2), Rating: f(4), Price: 1000, Currency: "EUR"},
		{Name: "La Boqueria market", Category: "food", Address: "La Rambla 91", Lat: f(41.3817), Lng: f(2.1716), DayDate: d(-1), StartTime: "10:00", DurationMin: 90, Status: "visited", VisitedAt: d(-1), Notes: "Juice stalls at the back are half the price of the front ones."},
		{Name: "Gothic Quarter walk", Category: "sight", Address: "Barri Gòtic", Lat: f(41.3833), Lng: f(2.1766), DayDate: d(-1), StartTime: "12:00", DurationMin: 180, Status: "visited", VisitedAt: d(-1)},
		{Name: "Bar del Pla", Category: "food", Address: "Carrer de Montcada 2", Lat: f(41.3853), Lng: f(2.1811), DayDate: d(0), StartTime: "20:30", DurationMin: 120, Status: "booked", Phone: "+34 932 68 30 03", BookingRef: "table for 4"},
		{Name: "Barceloneta beach", Category: "beach", Lat: f(41.3785), Lng: f(2.1925), DayDate: d(0), StartTime: "11:00", DurationMin: 240, Status: "planned"},
		{Name: "Montserrat monastery & hike", Category: "activity", Address: "Montserrat", Lat: f(41.5931), Lng: f(1.8378), DayDate: d(1), StartTime: "08:00", DurationMin: 480, Status: "booked", Price: 4200, Currency: "EUR", Link: "https://www.montserratvisita.com/", Notes: "R5 train from Plaça Espanya + rack railway. Boys' choir sings at 13:00."},
		{Name: "Picasso Museum", Category: "museum", Address: "Carrer de Montcada 15-23", Lat: f(41.3852), Lng: f(2.1809), DayDate: d(2), StartTime: "10:00", DurationMin: 120, Status: "planned", Price: 1400, Currency: "EUR"},
		{Name: "Bunkers del Carmel sunset", Category: "viewpoint", Lat: f(41.4192), Lng: f(2.1618), DayDate: d(2), StartTime: "19:00", DurationMin: 90, Status: "planned", Notes: "Bring a bottle; it gets crowded after 19:30."},
		{Name: "Camp Nou tour", Category: "activity", Lat: f(41.3809), Lng: f(2.1228), DayDate: d(4), StartTime: "11:00", DurationMin: 150, Status: "planned", Price: 2800, Currency: "EUR"},
		{Name: "Sitges day trip", Category: "beach", Address: "Sitges", Lat: f(41.2371), Lng: f(1.8056), Status: "idea"},
		{Name: "Casa Batlló", Category: "sight", Address: "Passeig de Gràcia 43", Lat: f(41.3917), Lng: f(2.1650), Status: "idea", Price: 3500, Currency: "EUR", Link: "https://www.casabatllo.es/"},
		{Name: "Tickets Bar", Category: "food", Address: "Av. del Paral·lel 164", Lat: f(41.3746), Lng: f(2.1590), Status: "idea"},
	}
	for i := range places {
		places[i].TripID = t.ID
		if places[i].Currency == "" {
			places[i].Currency = "EUR"
		}
		_ = s.store.CreatePlace(ctx, &places[i])
	}
	l := &store.Lodging{TripID: t.ID, Name: "El Born apartment", Type: "apartment", Address: "Carrer de la Princesa 21, Barcelona", Lat: f(41.3857), Lng: f(2.1795), CheckIn: d(-3), CheckOut: d(7), ReservationNumber: "HM-4471920", Link: "https://www.airbnb.com/", Phone: "+34 600 000 000", Price: 118000, Currency: "EUR", Paid: true, Rating: f(4.7), Notes: "Key box code 2580. Third floor, no lift."}
	_ = s.store.CreateLodging(ctx, l)
	s.syncItemCost(ctx, t, "lodging", l.ID, l.Name, "lodging", l.Price, l.Currency, l.CheckIn, l.Paid)
	tr := []store.Transport{
		{TripID: t.ID, Type: "flight", Name: "Flight out", Carrier: "Vueling", Number: "VY 8713", FromName: "Vienna (VIE)", FromLat: f(48.1103), FromLng: f(16.5697), ToName: "Barcelona (BCN)", ToLat: f(41.2974), ToLng: f(2.0833), DepartAt: d(-3) + "T07:05", ArriveAt: d(-3) + "T09:30", BookingRef: "K7XP2Q", Seat: "14A", Price: 17800, Currency: "EUR", Paid: true},
		{TripID: t.ID, Type: "train", Name: "Aerobús to Plaça Catalunya", FromName: "BCN T1", ToName: "Plaça de Catalunya", DepartAt: d(-3) + "T10:00", ArriveAt: d(-3) + "T10:35", Price: 1500, Currency: "EUR"},
		{TripID: t.ID, Type: "train", Name: "R5 to Montserrat", Carrier: "FGC", FromName: "Plaça Espanya", ToName: "Monistrol de Montserrat", DepartAt: d(1) + "T08:36", ArriveAt: d(1) + "T09:40", Price: 2400, Currency: "EUR", Notes: "Tot Montserrat ticket includes the rack railway."},
		{TripID: t.ID, Type: "flight", Name: "Flight home", Carrier: "Vueling", Number: "VY 8712", FromName: "Barcelona (BCN)", FromLat: f(41.2974), FromLng: f(2.0833), ToName: "Vienna (VIE)", ToLat: f(48.1103), ToLng: f(16.5697), DepartAt: d(7) + "T18:20", ArriveAt: d(7) + "T20:45", BookingRef: "K7XP2Q", Seat: "14A", Price: 17800, Currency: "EUR", Paid: true},
	}
	for i := range tr {
		_ = s.store.CreateTransport(ctx, &tr[i])
		s.syncItemCost(ctx, t, "transport", tr[i].ID, transportLabel(&tr[i]), "transport", tr[i].Price, tr[i].Currency, tr[i].DepartAt, tr[i].Paid)
	}
	exp := []store.Expense{
		{Name: "Travel insurance", Category: "insurance", Amount: 3190, Currency: "EUR", Kind: "planned", Date: d(-12)},
		{Name: "Food budget", Category: "food", Amount: 60000, Currency: "EUR", Kind: "planned"},
		{Name: "Museums & tickets", Category: "activities", Amount: 20000, Currency: "EUR", Kind: "planned"},
		{Name: "Taxi from the airport", Category: "transport", Amount: 3800, Currency: "EUR", Kind: "actual", Date: d(-3), Source: "money", ExternalID: "demo-1"},
		{Name: "Cal Pep", Category: "food", Amount: 8640, Currency: "EUR", Kind: "actual", Date: d(-3), Source: "money", ExternalID: "demo-2"},
		{Name: "Mercadona", Category: "groceries", Amount: 4215, Currency: "EUR", Kind: "actual", Date: d(-2), Source: "money", ExternalID: "demo-3"},
		{Name: "Sagrada Família tickets", Category: "activities", Amount: 10400, Currency: "EUR", Kind: "actual", Date: d(-2), Source: "money", ExternalID: "demo-4"},
		{Name: "Park Güell", Category: "activities", Amount: 4000, Currency: "EUR", Kind: "actual", Date: d(-2), Source: "money", ExternalID: "demo-5"},
		{Name: "La Boqueria", Category: "food", Amount: 2350, Currency: "EUR", Kind: "actual", Date: d(-1), Source: "money", ExternalID: "demo-6"},
		{Name: "T-casual metro passes", Category: "transport", Amount: 4880, Currency: "EUR", Kind: "actual", Date: d(-1), Source: "money", ExternalID: "demo-7"},
		{Name: "Gelato on the Rambla", Category: "food", Amount: 900, Currency: "EUR", Kind: "actual", Date: d(0), Source: "money", ExternalID: "demo-8"},
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
	for i, it := range PackingTemplates["city"] {
		_ = s.store.AddChecklistItem(ctx, t.ID, &store.ChecklistItem{ChecklistID: pack.ID, Text: it, Done: i < 5})
	}
	todo := &store.Checklist{TripID: t.ID, Name: "Before we leave", Kind: "todo"}
	_ = s.store.CreateChecklist(ctx, todo)
	for _, it := range PackingTemplates["todo"] {
		_ = s.store.AddChecklistItem(ctx, t.ID, &store.ChecklistItem{ChecklistID: todo.ID, Text: it, Done: true})
	}
	_ = s.store.CreateJournal(ctx, &store.JournalEntry{TripID: t.ID, UserID: u.ID, Date: d(-3), Title: "Landed", Body: "Up at four, in El Born by eleven. The apartment is three floors of narrow stairs and a terrace with a view of the church. Lunch at Cal Pep standing at the bar: razor clams, a tortilla, the house cava.", Mood: "😊"})
	_ = s.store.CreateJournal(ctx, &store.JournalEntry{TripID: t.ID, UserID: m.ID, Date: d(-2), Title: "Gaudí day", Body: "Sagrada Família at nine before the crowds — the light through the west windows at that hour is unreal. Park Güell after, Marko fell asleep on the bench with the mosaic lizard.", Mood: "🤩"})

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
	n := &store.Trip{OwnerID: u.ID, Name: "Tokyo in spring", Destination: "Tokyo, Japan", StartDate: d(62), EndDate: d(71), Status: store.StatusPlanning, Currency: "EUR", Budget: 380000, Lat: f(35.6762), Lng: f(139.6503), Timezone: "Asia/Tokyo", Dawarich: true, Immich: true, Color: "#d97706"}
	if err := s.store.CreateTrip(ctx, n); err != nil {
		return err
	}
	for _, pl := range []store.Place{
		{Name: "Senso-ji", Category: "sight", Address: "Asakusa", Lat: f(35.7148), Lng: f(139.7967), Status: "idea", Notes: "Go at 7am before the shops open on Nakamise."},
		{Name: "Shibuya crossing & Sky", Category: "viewpoint", Lat: f(35.6595), Lng: f(139.7004), Status: "idea", Price: 250000, Currency: "JPY"},
		{Name: "Tsukiji outer market", Category: "food", Lat: f(35.6654), Lng: f(139.7707), Status: "idea"},
		{Name: "teamLab Planets", Category: "activity", Lat: f(35.6490), Lng: f(139.7899), Status: "idea", Price: 380000, Currency: "JPY", Link: "https://www.teamlab.art/e/planets/"},
		{Name: "Fuji day trip (Kawaguchiko)", Category: "nature", Lat: f(35.5171), Lng: f(138.7510), Status: "idea", Notes: "Clear mornings only — check the forecast the night before."},
	} {
		pl.TripID = n.ID
		if pl.Currency == "" {
			pl.Currency = "EUR"
		}
		_ = s.store.CreatePlace(ctx, &pl)
	}
	_ = s.store.CreateChecklist(ctx, &store.Checklist{TripID: n.ID, Name: "Packing", Kind: "packing"})
	bl := &store.Checklist{TripID: n.ID, Name: "Before we leave", Kind: "todo"}
	_ = s.store.CreateChecklist(ctx, bl)
	_ = s.store.AddChecklistItem(ctx, n.ID, &store.ChecklistItem{ChecklistID: bl.ID, Text: "Book flights"})
	_ = s.store.AddChecklistItem(ctx, n.ID, &store.ChecklistItem{ChecklistID: bl.ID, Text: "Order the JR Pass"})
	_ = s.store.AddChecklistItem(ctx, n.ID, &store.ChecklistItem{ChecklistID: bl.ID, Text: "Pick a neighbourhood for the hotel"})
	s.log.Info("demo data seeded")
	return nil
}
