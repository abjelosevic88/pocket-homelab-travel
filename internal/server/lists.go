package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// ---- checklists ----

func (s *Server) handleListChecklists(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	l, err := s.store.ListChecklists(r.Context(), t.ID)
	if err != nil {
		s.fail(w, err, "checklists")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// PackingTemplates are starter lists offered in the UI.
var PackingTemplates = map[string][]string{
	"essentials": {"Passport / ID", "Wallet & cards", "Phone + charger", "Travel insurance docs", "Medication", "Sunglasses", "Headphones", "Power bank", "Keys"},
	"beach":      {"Swimwear", "Sunscreen", "Beach towel", "Flip-flops", "Hat", "Snorkel", "Waterproof phone pouch", "After-sun"},
	"city":       {"Comfortable shoes", "Day backpack", "Rain jacket", "Reusable bottle", "Transit card", "Museum tickets", "Universal adapter"},
	"hiking":     {"Hiking boots", "Rain shell", "Layers", "Map / offline maps", "First-aid kit", "Headlamp", "Trekking poles", "Snacks", "2 l water"},
	"winter":     {"Thermal layers", "Gloves", "Beanie", "Ski pass", "Goggles", "Lip balm", "Hand warmers"},
	"camping":    {"Tent", "Sleeping bag", "Mat", "Stove + gas", "Lighter", "Cookware", "Lantern", "Insect repellent", "Trash bags"},
	"kids":       {"Snacks", "Wipes", "Tablet + downloads", "Favourite toy", "Spare clothes", "Child seat / booster"},
	"car":        {"Driving licence", "Car docs & insurance", "Vignette / toll pass", "Phone mount", "Charger cable", "Spare water", "Tyre check"},
	"todo":       {"Hold the mail", "Water the plants", "Check passport expiry", "Notify the bank", "Download offline maps", "Print bookings", "Charge everything", "Empty the fridge"},
}

func (s *Server) handleCreateChecklist(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	var in struct {
		Name     string   `json:"name"`
		Kind     string   `json:"kind"`
		Template string   `json:"template"`
		Items    []string `json:"items"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	c := &store.Checklist{TripID: t.ID, Name: in.Name, Kind: in.Kind}
	if err := s.store.CreateChecklist(r.Context(), c); err != nil {
		s.fail(w, err, "create checklist")
		return
	}
	items := in.Items
	if tpl, ok := PackingTemplates[in.Template]; ok {
		items = append(items, tpl...)
	}
	for _, txt := range items {
		if txt = strings.TrimSpace(txt); txt != "" {
			it := store.ChecklistItem{ChecklistID: c.ID, Text: txt}
			if err := s.store.AddChecklistItem(r.Context(), t.ID, &it); err == nil {
				c.Items = append(c.Items, it)
			}
		}
	}
	s.store.LogActivity(r.Context(), t.ID, userFrom(r.Context()).ID, "added checklist", c.Name)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateChecklist(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	var in struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if in.Kind == "" {
		in.Kind = "todo"
	}
	if err := s.store.UpdateChecklist(r.Context(), t.ID, idParam(r, "id"), strings.TrimSpace(in.Name), in.Kind); err != nil {
		s.fail(w, err, "update checklist")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteChecklist(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	if err := s.store.DeleteChecklist(r.Context(), t.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete checklist")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddChecklistItem(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	var in struct {
		Text       string `json:"text"`
		AssigneeID *int64 `json:"assignee_id"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	// Several lines at once: one item per line.
	var out []store.ChecklistItem
	for _, line := range strings.Split(in.Text, "\n") {
		if line = strings.TrimSpace(strings.TrimLeft(line, "-*• ")); line == "" {
			continue
		}
		it := store.ChecklistItem{ChecklistID: idParam(r, "id"), Text: line, AssigneeID: in.AssigneeID}
		if err := s.store.AddChecklistItem(r.Context(), t.ID, &it); err != nil {
			s.fail(w, err, "add item")
			return
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateChecklistItem(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	it, err := s.store.GetChecklistItem(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get item")
		return
	}
	var in struct {
		Text       *string `json:"text"`
		Done       *bool   `json:"done"`
		AssigneeID *int64  `json:"assignee_id"`
		SortOrder  *int    `json:"sort_order"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Text != nil && strings.TrimSpace(*in.Text) != "" {
		it.Text = strings.TrimSpace(*in.Text)
	}
	if in.Done != nil {
		it.Done = *in.Done
	}
	if keys["assignee_id"] {
		it.AssigneeID = in.AssigneeID
	}
	if in.SortOrder != nil {
		it.SortOrder = *in.SortOrder
	}
	if err := s.store.UpdateChecklistItem(r.Context(), t.ID, it); err != nil {
		s.fail(w, err, "update item")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleDeleteChecklistItem(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	if err := s.store.DeleteChecklistItem(r.Context(), t.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete item")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- journal ----

func (s *Server) handleListJournal(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	j, err := s.store.ListJournal(r.Context(), t.ID)
	if err != nil {
		s.fail(w, err, "journal")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

type journalInput struct {
	Date  string   `json:"date"`
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Mood  string   `json:"mood"`
	Lat   *float64 `json:"lat"`
	Lng   *float64 `json:"lng"`
}

func (s *Server) handleCreateJournal(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	var in journalInput
	if err := decode(r, &in); err != nil || !validDate(in.Date) {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Body) == "" && strings.TrimSpace(in.Title) == "" {
		writeErr(w, http.StatusBadRequest, "write something first")
		return
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	j := &store.JournalEntry{TripID: t.ID, UserID: userFrom(r.Context()).ID, Date: in.Date, Title: strings.TrimSpace(in.Title), Body: in.Body, Mood: in.Mood, Lat: in.Lat, Lng: in.Lng}
	if err := s.store.CreateJournal(r.Context(), j); err != nil {
		s.fail(w, err, "create journal")
		return
	}
	s.store.LogActivity(r.Context(), t.ID, j.UserID, "wrote", firstLine(j.Title, j.Body))
	j, _ = s.store.GetJournal(r.Context(), t.ID, j.ID)
	writeJSON(w, http.StatusCreated, j)
}

func firstLine(title, body string) string {
	if title != "" {
		return title
	}
	l := strings.SplitN(strings.TrimSpace(body), "\n", 2)[0]
	if len(l) > 60 {
		l = l[:60] + "…"
	}
	return l
}

func (s *Server) handleUpdateJournal(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	j, err := s.store.GetJournal(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get journal")
		return
	}
	var in journalInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["date"] && validDate(in.Date) && in.Date != "" {
		j.Date = in.Date
	}
	if keys["title"] {
		j.Title = strings.TrimSpace(in.Title)
	}
	if keys["body"] {
		j.Body = in.Body
	}
	if keys["mood"] {
		j.Mood = in.Mood
	}
	if keys["lat"] {
		j.Lat = in.Lat
	}
	if keys["lng"] {
		j.Lng = in.Lng
	}
	if err := s.store.UpdateJournal(r.Context(), j); err != nil {
		s.fail(w, err, "update journal")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleDeleteJournal(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	if err := s.store.DeleteJournal(r.Context(), t.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete journal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- documents ----

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	d, err := s.store.ListDocuments(r.Context(), t.ID)
	if err != nil {
		s.fail(w, err, "documents")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	b, name, ct, err := s.readUpload(r, "file")
	if err != nil || len(b) == 0 {
		writeErr(w, http.StatusBadRequest, "no file")
		return
	}
	rel, err := s.saveFile(t.ID, extOf(name, ct), b)
	if err != nil {
		s.fail(w, err, "save file")
		return
	}
	d := &store.Document{TripID: t.ID, UserID: userFrom(r.Context()).ID, Name: name, Filename: rel, Mime: ct, Bytes: int64(len(b)), Kind: r.FormValue("kind"), LinkKind: r.FormValue("link_kind")}
	if n := r.FormValue("name"); n != "" {
		d.Name = n
	}
	if d.Name == "" {
		d.Name = "file"
	}
	if id := r.FormValue("link_id"); id != "" {
		d.LinkID = idOf(id)
	}
	if err := s.store.CreateDocument(r.Context(), d); err != nil {
		s.fail(w, err, "create document")
		return
	}
	s.store.LogActivity(r.Context(), t.ID, d.UserID, "uploaded", d.Name)
	writeJSON(w, http.StatusCreated, d)
}

func idOf(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	d, err := s.store.GetDocument(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get document")
		return
	}
	var in struct {
		Name     *string `json:"name"`
		Kind     *string `json:"kind"`
		LinkKind *string `json:"link_kind"`
		LinkID   *int64  `json:"link_id"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		d.Name = strings.TrimSpace(*in.Name)
	}
	if in.Kind != nil {
		d.Kind = *in.Kind
	}
	if keys["link_kind"] && in.LinkKind != nil {
		d.LinkKind = *in.LinkKind
	}
	if keys["link_id"] && in.LinkID != nil {
		d.LinkID = *in.LinkID
	}
	if err := s.store.UpdateDocument(r.Context(), d); err != nil {
		s.fail(w, err, "update document")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDocumentFile(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	d, err := s.store.GetDocument(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get document")
		return
	}
	dl := ""
	if queryBool(r, "download") {
		dl = d.Name
	}
	s.serveFile(w, r, d.Filename, d.Mime, dl)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	d, err := s.store.GetDocument(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get document")
		return
	}
	if err := s.store.DeleteDocument(r.Context(), t.ID, d.ID); err != nil {
		s.fail(w, err, "delete document")
		return
	}
	s.removeFile(d.Filename)
	w.WriteHeader(http.StatusNoContent)
}
