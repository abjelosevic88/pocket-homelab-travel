package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// rate returns how many units of `to` one unit of `from` is worth on a date,
// from the cache or the FX providers. Falls back to 1 (and reports it) when
// nothing is available so an expense is never lost.
func (s *Server) rate(ctx context.Context, from, to, date string) (float64, string) {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == to || from == "" || to == "" {
		return 1, "same"
	}
	if date == "" || date > store.Today() {
		date = store.Today()
	}
	if r, src, ok := s.store.GetFXRate(ctx, from, to, date); ok {
		return r, src
	}
	st, _ := s.store.GetSettings(ctx, 0) // fx_auto is box-wide
	if !st.FXAuto {
		return 1, "disabled"
	}
	r, src, err := s.fx.Rate(ctx, from, to, date)
	if err != nil {
		s.log.Warn("fx lookup failed", "from", from, "to", to, "date", date, "err", err)
		return 1, "unavailable"
	}
	_ = s.store.PutFXRate(ctx, from, to, date, r, src)
	return r, src
}

func (s *Server) handleFX(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to, date := strings.ToUpper(q.Get("from")), strings.ToUpper(q.Get("to")), q.Get("date")
	if !currency.Valid(from) || !currency.Valid(to) {
		writeErr(w, http.StatusBadRequest, "unknown currency")
		return
	}
	rate, src := s.rate(r.Context(), from, to, date)
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "date": date, "rate": rate, "source": src})
}

func (s *Server) handleCurrencies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currency.All())
}

// syncItemCost keeps a planned expense in step with an item's price
// (lodging/transport): price 0 removes it.
func (s *Server) syncItemCost(ctx context.Context, t *store.Trip, kind string, id int64, name, category string, price int64, cur, date string, paid bool) {
	expenses, err := s.store.ListExpenses(ctx, t.ID)
	if err != nil {
		return
	}
	var existing *store.Expense
	for i := range expenses {
		if expenses[i].Source == "item" && expenses[i].LinkKind == kind && expenses[i].LinkID == id {
			existing = &expenses[i]
			break
		}
	}
	if price <= 0 {
		if existing != nil {
			_ = s.store.DeleteExpense(ctx, t.ID, existing.ID)
		}
		return
	}
	if cur == "" {
		cur = t.Currency
	}
	if len(date) > 10 {
		date = date[:10]
	}
	rate, _ := s.rate(ctx, cur, t.Currency, date)
	ek := "planned"
	if paid {
		ek = "actual"
	}
	e := store.Expense{TripID: t.ID, Name: name, Category: category, Amount: price, Currency: cur, AmountHome: currency.Convert(price, cur, t.Currency, rate), FXRate: rate, Date: date, Kind: ek, Source: "item", LinkKind: kind, LinkID: id}
	if existing != nil {
		e.ID = existing.ID
		_ = s.store.UpdateExpense(ctx, &e)
		return
	}
	_ = s.store.CreateExpense(ctx, &e)
}

func (s *Server) handleListExpenses(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	e, err := s.store.ListExpenses(r.Context(), t.ID)
	if err != nil {
		s.fail(w, err, "list expenses")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

type expenseInput struct {
	Name     *string  `json:"name"`
	Category *string  `json:"category"`
	Amount   *int64   `json:"amount"`
	Currency *string  `json:"currency"`
	FXRate   *float64 `json:"fx_rate"`
	Date     *string  `json:"date"`
	Kind     *string  `json:"kind"`
	LinkKind *string  `json:"link_kind"`
	LinkID   *int64   `json:"link_id"`
	Notes    *string  `json:"notes"`
}

func (s *Server) applyExpense(ctx context.Context, t *store.Trip, e *store.Expense, in expenseInput, keys map[string]bool) string {
	if keys["name"] && in.Name != nil {
		e.Name = strings.TrimSpace(*in.Name)
	}
	if keys["category"] && in.Category != nil {
		e.Category = strings.TrimSpace(*in.Category)
	}
	if keys["amount"] && in.Amount != nil {
		e.Amount = *in.Amount
	}
	if keys["currency"] && in.Currency != nil {
		e.Currency = strings.ToUpper(strings.TrimSpace(*in.Currency))
	}
	if keys["date"] && in.Date != nil {
		e.Date = *in.Date
	}
	if keys["kind"] && in.Kind != nil {
		e.Kind = *in.Kind
	}
	if keys["link_kind"] && in.LinkKind != nil {
		e.LinkKind = *in.LinkKind
	}
	if keys["link_id"] && in.LinkID != nil {
		e.LinkID = *in.LinkID
	}
	if keys["notes"] && in.Notes != nil {
		e.Notes = *in.Notes
	}
	if e.Name == "" {
		return "name is required"
	}
	if e.Amount < 0 {
		return "amount cannot be negative"
	}
	if e.Currency == "" {
		e.Currency = t.Currency
	}
	if !currency.Valid(e.Currency) {
		return "unknown currency"
	}
	if e.Category == "" {
		e.Category = "other"
	}
	if e.Kind != "planned" && e.Kind != "actual" {
		e.Kind = "planned"
	}
	if !validDate(e.Date) {
		return "date must be YYYY-MM-DD"
	}
	if keys["fx_rate"] && in.FXRate != nil && *in.FXRate > 0 {
		e.FXRate = *in.FXRate
	} else if keys["amount"] || keys["currency"] || keys["date"] || e.FXRate == 0 {
		e.FXRate, _ = s.rate(ctx, e.Currency, t.Currency, e.Date)
	}
	e.AmountHome = currency.Convert(e.Amount, e.Currency, t.Currency, e.FXRate)
	return ""
}

func (s *Server) handleCreateExpense(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	var in expenseInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	e := &store.Expense{TripID: t.ID, Currency: t.Currency, Kind: "planned", Source: "manual"}
	if msg := s.applyExpense(r.Context(), t, e, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateExpense(r.Context(), e); err != nil {
		s.fail(w, err, "create expense")
		return
	}
	s.store.LogActivity(r.Context(), t.ID, userFrom(r.Context()).ID, "added cost", e.Name)
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleUpdateExpense(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	e, err := s.store.GetExpense(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get expense")
		return
	}
	if e.Source == "firefly" {
		// Only the category may be changed locally; amounts come from Firefly.
		var in expenseInput
		keys, err := decodeKeys(r, &in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if keys["category"] && in.Category != nil {
			e.Category = strings.TrimSpace(*in.Category)
		}
		if keys["notes"] && in.Notes != nil {
			e.Notes = *in.Notes
		}
		if err := s.store.UpdateExpense(r.Context(), e); err != nil {
			s.fail(w, err, "update expense")
			return
		}
		writeJSON(w, http.StatusOK, e)
		return
	}
	var in expenseInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := s.applyExpense(r.Context(), t, e, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateExpense(r.Context(), e); err != nil {
		s.fail(w, err, "update expense")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	if err := s.store.DeleteExpense(r.Context(), t.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete expense")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BudgetSummary is the money view of a trip, in the trip currency.
type BudgetSummary struct {
	Currency     string            `json:"currency"`
	Budget       int64             `json:"budget"`
	Planned      int64             `json:"planned"`
	Actual       int64             `json:"actual"`
	Remaining    int64             `json:"remaining"` // budget - actual (or - planned when no actuals yet)
	Total        int64             `json:"total"`     // actual + booked but unpaid stays/transport
	Bookings     int64             `json:"bookings"`  // the unpaid stays/transport part of Total
	PerDay       int64             `json:"per_day"`
	ByCategory   []CategoryTotal   `json:"by_category"`
	ByDay        []DayTotal        `json:"by_day"`
	ByCurrency   map[string]int64  `json:"by_currency"` // actual, in original currencies
	Top          []store.Expense   `json:"top"`
	FireflyCount int               `json:"firefly_count"`
	Days         int               `json:"days"`
	DaysElapsed  int               `json:"days_elapsed"`
	Sources      map[string]int    `json:"sources"`
	Status       map[string]string `json:"status"`
}

// CategoryTotal is planned vs actual per category.
type CategoryTotal struct {
	Category string `json:"category"`
	Planned  int64  `json:"planned"`
	Actual   int64  `json:"actual"`
}

// DayTotal is spending on one day.
type DayTotal struct {
	Date   string `json:"date"`
	Actual int64  `json:"actual"`
	Count  int    `json:"count"`
}

func (s *Server) budgetSummary(t *store.Trip, expenses []store.Expense) BudgetSummary {
	b := BudgetSummary{Currency: t.Currency, Budget: t.Budget, ByCurrency: map[string]int64{}, Sources: map[string]int{}, Days: t.Days, Status: map[string]string{}}
	cats := map[string]*CategoryTotal{}
	days := map[string]*DayTotal{}
	var actuals []store.Expense
	for _, e := range expenses {
		c := cats[e.Category]
		if c == nil {
			c = &CategoryTotal{Category: e.Category}
			cats[e.Category] = c
		}
		b.Sources[e.Source]++
		if e.Kind == "actual" {
			b.Actual += e.AmountHome
			c.Actual += e.AmountHome
			b.ByCurrency[e.Currency] += e.Amount
			if e.Source == "firefly" {
				b.FireflyCount++
			}
			if e.Date != "" {
				d := days[e.Date]
				if d == nil {
					d = &DayTotal{Date: e.Date}
					days[e.Date] = d
				}
				d.Actual += e.AmountHome
				d.Count++
			}
			actuals = append(actuals, e)
		} else {
			b.Planned += e.AmountHome
			c.Planned += e.AmountHome
			if e.Source == "item" {
				b.Bookings += e.AmountHome
			}
		}
	}
	for _, c := range cats {
		b.ByCategory = append(b.ByCategory, *c)
	}
	sort.Slice(b.ByCategory, func(i, j int) bool {
		return b.ByCategory[i].Actual+b.ByCategory[i].Planned > b.ByCategory[j].Actual+b.ByCategory[j].Planned
	})
	for _, d := range days {
		b.ByDay = append(b.ByDay, *d)
	}
	sort.Slice(b.ByDay, func(i, j int) bool { return b.ByDay[i].Date < b.ByDay[j].Date })
	sort.Slice(actuals, func(i, j int) bool { return actuals[i].AmountHome > actuals[j].AmountHome })
	if len(actuals) > 5 {
		actuals = actuals[:5]
	}
	if actuals == nil {
		actuals = []store.Expense{}
	}
	b.Top = actuals
	if b.ByCategory == nil {
		b.ByCategory = []CategoryTotal{}
	}
	if b.ByDay == nil {
		b.ByDay = []DayTotal{}
	}
	b.Total = b.Actual + b.Bookings
	if t.Budget > 0 {
		if b.Actual > 0 {
			b.Remaining = t.Budget - b.Actual
		} else {
			b.Remaining = t.Budget - b.Planned
		}
	}
	// Days elapsed so far (for the burn rate).
	today := store.Today()
	if t.StartDate != "" {
		if today >= t.StartDate {
			end := today
			if t.EndDate != "" && t.EndDate < today {
				end = t.EndDate
			}
			b.DaysElapsed = store.DaysBetween(t.StartDate, end)
		}
	}
	if b.DaysElapsed > 0 {
		b.PerDay = b.Actual / int64(b.DaysElapsed)
	} else if t.Days > 0 {
		b.PerDay = b.Planned / int64(t.Days)
	}
	return b
}

func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	e, err := s.store.ListExpenses(r.Context(), t.ID)
	if err != nil {
		s.fail(w, err, "expenses")
		return
	}
	writeJSON(w, http.StatusOK, s.budgetSummary(t, e))
}

// ---- Firefly ----

// fireflyCategory maps a Firefly category / description to one of ours.
func fireflyCategory(cat, desc, dest string) string {
	s := strings.ToLower(cat + " " + desc + " " + dest)
	switch {
	case containsAny(s, "hotel", "apartman", "apartment", "booking", "airbnb", "smještaj", "smestaj", "lodging", "hostel", "nocenje", "noćenje"):
		return "lodging"
	case containsAny(s, "gorivo", "fuel", "benzin", "petrol", "diesel", "dizel", "ina ", "petrol ", "pumpa", "nis "):
		return "fuel"
	case containsAny(s, "restoran", "restaurant", "hrana", "food", "pekara", "bakery", "kafa", "coffee", "cafe", "kafić", "pizza", "burek", "pica", "ćevap", "cevap", "konoba", "lunch", "dinner", "ručak", "večera", "rucak", "vecera", "dorucak", "doručak"):
		return "food"
	case containsAny(s, "market", "supermarket", "fasunga", "groceries", "mercator", "konzum", "lidl", "idea", "voli", "bingo", "roda", "maxi", "tropik", "prodavnica", "shop"):
		return "groceries"
	case containsAny(s, "transport", "bus", "autobus", "voz", "train", "taxi", "taksi", "let ", "flight", "avion", "ferry", "trajekt", "parking", "putarina", "toll", "rent", "uber", "bolt"):
		return "transport"
	case containsAny(s, "ulaznic", "ticket", "muzej", "museum", "tura", "tour", "izlet", "excursion", "boat", "brod", "kayak", "kajak", "spa", "activity", "aktivnost", "ulaz"):
		return "activities"
	case containsAny(s, "suvenir", "souvenir", "odjeća", "odjeca", "clothes", "shopping", "kupovina", "poklon", "gift"):
		return "shopping"
	case containsAny(s, "osiguranje", "insurance"):
		return "insurance"
	case containsAny(s, "naknada", "fee", "provizija", "bankomat", "atm", "mjenjačnica", "exchange"):
		return "fees"
	}
	return "other"
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// syncFirefly pulls the trip's tagged withdrawals into expenses (kind actual,
// source firefly). Returns the number of synced rows.
func (s *Server) syncFirefly(ctx context.Context, tripID int64) (int, error) {
	s.syncMu.Lock()
	if s.syncing[tripID] {
		s.syncMu.Unlock()
		return 0, errors.New("sync already running")
	}
	s.syncing[tripID] = true
	s.syncMu.Unlock()
	defer func() {
		s.syncMu.Lock()
		delete(s.syncing, tripID)
		s.syncMu.Unlock()
	}()

	t, err := s.store.GetTrip(ctx, tripID, 0)
	if err != nil {
		return 0, err
	}
	if t.FireflyTag == "" {
		return 0, errors.New("the trip has no Firefly tag")
	}
	// The trip's spending comes from its owner's Firefly account, whoever
	// presses sync (an editor of a shared trip) or the scheduler.
	_, ff, _, _ := s.clients(ctx, t.OwnerID)
	if !ff.Configured() {
		return 0, errors.New("firefly is not configured for the trip owner")
	}
	tag, err := ff.TagByName(ctx, t.FireflyTag)
	if err != nil {
		return 0, err
	}
	if tag == nil {
		return 0, fmt.Errorf("tag %q does not exist in Firefly", t.FireflyTag)
	}
	// A little slack around the trip: flights are paid before, the last
	// dinner is charged after. Expenses outside the window still count when
	// they carry the tag — the tag is the user's explicit decision.
	txs, err := ff.TagTransactions(ctx, tag.ID, "", "")
	if err != nil {
		return 0, err
	}
	// Keep existing categories the user may have edited.
	existing, _ := s.store.ListExpenses(ctx, t.ID)
	userCat := map[string]string{}
	for _, e := range existing {
		if e.Source == "firefly" {
			userCat[e.ExternalID] = e.Category
		}
	}
	keep := make([]string, 0, len(txs))
	n := 0
	for _, tx := range txs {
		dec := tx.Decimals
		if dec == 0 && currency.Decimals(tx.Currency) != 0 {
			dec = currency.Decimals(tx.Currency)
		}
		minor := int64(math.Round(tx.Amount * math.Pow10(dec)))
		cur := tx.Currency
		// Prefer the foreign amount when the card was charged in the trip currency.
		if tx.ForeignCur != "" && strings.EqualFold(tx.ForeignCur, t.Currency) {
			cur = tx.ForeignCur
			minor = int64(math.Round(tx.ForeignAmt * math.Pow10(currency.Decimals(cur))))
		}
		rate, _ := s.rate(ctx, cur, t.Currency, tx.Date)
		cat := userCat[tx.ID]
		if cat == "" {
			cat = fireflyCategory(tx.Category, tx.Description, tx.Destination)
		}
		notes := strings.TrimSpace(strings.Join(nonEmpty(tx.Category, tx.Destination, tx.Notes), " · "))
		e := &store.Expense{TripID: t.ID, Name: tx.Description, Category: cat, Amount: minor, Currency: cur, AmountHome: currency.Convert(minor, cur, t.Currency, rate), FXRate: rate, Date: tx.Date, Kind: "actual", Source: "firefly", ExternalID: tx.ID, Notes: notes}
		created, err := s.store.UpsertExternalExpense(ctx, e)
		if err != nil {
			s.log.Warn("firefly upsert", "err", err)
			continue
		}
		if created {
			s.publish("travel.expense_linked", fmt.Sprintf("expense:%d", e.ID), t.OwnerID, map[string]any{
				"expense_id": e.ID, "trip_id": t.ID, "trip": t.Name, "name": e.Name, "category": e.Category,
				"amount": e.Amount, "currency": e.Currency, "amount_home": e.AmountHome, "home_currency": t.Currency,
				"date": e.Date, "source": e.Source, "external_id": e.ExternalID, "firefly_tag": t.FireflyTag,
			})
		}
		keep = append(keep, tx.ID)
		n++
	}
	if _, err := s.store.DeleteExternalExpensesNotIn(ctx, t.ID, "firefly", keep); err != nil {
		s.log.Warn("firefly prune", "err", err)
	}
	_ = s.store.CachePut(ctx, cacheKey(t.ID, "firefly-synced"), fmt.Sprintf("%d", n))
	s.log.Info("firefly synced", "trip", t.Name, "tag", t.FireflyTag, "rows", n)
	return n, nil
}

func nonEmpty(v ...string) []string {
	var out []string
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func (s *Server) handleFireflySync(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	n, err := s.syncFirefly(r.Context(), t.ID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	e, _ := s.store.ListExpenses(r.Context(), t.ID)
	writeJSON(w, http.StatusOK, map[string]any{"synced": n, "budget": s.budgetSummary(t, e), "expenses": e})
}

// handleFireflyTags lists the caller's own Firefly tags.
func (s *Server) handleFireflyTags(w http.ResponseWriter, r *http.Request) {
	_, ff, _, _ := s.clients(r.Context(), actorID(r.Context()))
	if !ff.Configured() {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	tags, err := ff.Tags(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	sort.Slice(tags, func(i, j int) bool { return strings.ToLower(tags[i].Name) < strings.ToLower(tags[j].Name) })
	writeJSON(w, http.StatusOK, tags)
}

// autoSync refreshes Firefly for trips that are running or recently finished.
// Each trip syncs with its own owner's credentials (see syncFirefly), so the
// job only ever touches that owner's spending.
func (s *Server) autoSync(ctx context.Context) {
	trips, err := s.store.ListTrips(ctx, store.TripFilter{})
	if err != nil {
		return
	}
	today := store.Today()
	cutoff := time.Now().AddDate(0, 0, -14).Format("2006-01-02")
	for _, t := range trips {
		if t.FireflyTag == "" || t.Archived {
			continue
		}
		recent := t.EndDate == "" || t.EndDate >= cutoff
		upcoming := t.StartDate != "" && t.StartDate > today
		if recent || upcoming {
			if _, err := s.syncFirefly(ctx, t.ID); err != nil {
				s.log.Warn("auto sync", "trip", t.Name, "err", err)
			}
		}
	}
}
