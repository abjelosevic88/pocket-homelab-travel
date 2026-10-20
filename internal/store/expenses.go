package store

import (
	"context"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"strings"
)

const expenseCols = `id, trip_id, name, category, amount, currency, amount_home, fx_rate, date, kind, source, external_id, link_kind, link_id, notes, created_at, updated_at`

func scanExpense(row interface{ Scan(...any) error }) (*Expense, error) {
	var e Expense
	if err := row.Scan(&e.ID, &e.TripID, &e.Name, &e.Category, &e.Amount, &e.Currency, &e.AmountHome, &e.FXRate, &e.Date, &e.Kind, &e.Source, &e.ExternalID, &e.LinkKind, &e.LinkID, &e.Notes, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &e, nil
}

// ListExpenses returns a trip's expenses, newest date first.
func (s *Store) ListExpenses(ctx context.Context, tripID int64) ([]Expense, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE trip_id = ? ORDER BY date DESC, id DESC`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Expense{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// GetExpense loads one expense.
func (s *Store) GetExpense(ctx context.Context, tripID, id int64) (*Expense, error) {
	return scanExpense(s.DB.QueryRowContext(ctx, `SELECT `+expenseCols+` FROM expenses WHERE id = ? AND trip_id = ?`, id, tripID))
}

// CreateExpense inserts an expense.
func (s *Store) CreateExpense(ctx context.Context, e *Expense, after ...core.CommitHook) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if e.Kind == "" {
		e.Kind = "planned"
	}
	if e.Source == "" {
		e.Source = "manual"
	}
	if e.Category == "" {
		e.Category = "other"
	}
	if e.FXRate == 0 {
		e.FXRate = 1
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO expenses (trip_id, name, category, amount, currency, amount_home, fx_rate, date, kind, source, external_id, link_kind, link_id, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TripID, e.Name, e.Category, e.Amount, e.Currency, e.AmountHome, e.FXRate, e.Date, e.Kind, e.Source, e.ExternalID, e.LinkKind, e.LinkID, e.Notes)
	if err != nil {
		return conflict(err)
	}
	e.ID, _ = res.LastInsertId()
	return core.Commit(tx, after...)
}

// UpdateExpense saves an expense.
func (s *Store) UpdateExpense(ctx context.Context, e *Expense) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE expenses SET name = ?, category = ?, amount = ?, currency = ?, amount_home = ?, fx_rate = ?, date = ?, kind = ?, link_kind = ?, link_id = ?, notes = ?, updated_at = ? WHERE id = ? AND trip_id = ?`,
		e.Name, e.Category, e.Amount, e.Currency, e.AmountHome, e.FXRate, e.Date, e.Kind, e.LinkKind, e.LinkID, e.Notes, Now(), e.ID, e.TripID)
	return err
}

// SyncedSources are the expense sources written by the trip-cost sync.
var SyncedSources = []string{"money", "firefly"}

// IsSynced reports whether an expense came from the trip-cost sync (its
// amount and date belong to Pocket Money / Firefly, not to this app).
func IsSynced(source string) bool { return source == "money" || source == "firefly" }

// RekeyExternalExpense moves a synced row to another source/external id, so
// a cost first synced from Firefly keeps its row (and the category someone
// picked for it) when the same transaction now comes from Pocket Money.
func (s *Store) RekeyExternalExpense(ctx context.Context, tripID, id int64, source, externalID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE expenses SET source = ?, external_id = ?, updated_at = ? WHERE id = ? AND trip_id = ?`, source, externalID, Now(), id, tripID)
	return conflict(err)
}

// DeleteSyncedExpensesExcept removes the trip's synced rows (any of sources)
// whose id is not in keep: what the tag no longer carries upstream.
func (s *Store) DeleteSyncedExpensesExcept(ctx context.Context, tripID int64, sources []string, keep []int64) (int64, error) {
	if len(sources) == 0 {
		return 0, nil
	}
	q := `DELETE FROM expenses WHERE trip_id = ? AND source IN (?` + strings.Repeat(",?", len(sources)-1) + `)`
	args := []any{tripID}
	for _, src := range sources {
		args = append(args, src)
	}
	if len(keep) > 0 {
		q += ` AND id NOT IN (?` + strings.Repeat(",?", len(keep)-1) + `)`
		for _, id := range keep {
			args = append(args, id)
		}
	}
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpsertExternalExpense inserts or updates a synced row (Pocket Money / Firefly) by source + external id.
// Returns true when a new row was created.
func (s *Store) UpsertExternalExpense(ctx context.Context, e *Expense, after ...core.CommitHook) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM expenses WHERE trip_id = ? AND source = ? AND external_id = ?`, e.TripID, e.Source, e.ExternalID).Scan(&id)
	if err == nil {
		e.ID = id
		_, err = s.DB.ExecContext(ctx, `UPDATE expenses SET name = ?, category = ?, amount = ?, currency = ?, amount_home = ?, fx_rate = ?, date = ?, kind = ?, notes = ?, updated_at = ? WHERE id = ?`,
			e.Name, e.Category, e.Amount, e.Currency, e.AmountHome, e.FXRate, e.Date, e.Kind, e.Notes, Now(), id)
		return false, err
	}
	return true, s.CreateExpense(ctx, e, after...)
}

// DeleteExpense removes an expense.
func (s *Store) DeleteExpense(ctx context.Context, tripID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM expenses WHERE id = ? AND trip_id = ?`, id, tripID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExternalExpensesNotIn removes synced rows whose external id is no longer present upstream.
func (s *Store) DeleteExternalExpensesNotIn(ctx context.Context, tripID int64, source string, keep []string) (int64, error) {
	if len(keep) == 0 {
		res, err := s.DB.ExecContext(ctx, `DELETE FROM expenses WHERE trip_id = ? AND source = ?`, tripID, source)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	q := `DELETE FROM expenses WHERE trip_id = ? AND source = ? AND external_id NOT IN (`
	args := []any{tripID, source}
	for i, k := range keep {
		if i > 0 {
			q += ","
		}
		q += "?"
		args = append(args, k)
	}
	q += ")"
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- checklists ----

// ListChecklists returns a trip's checklists with items.
func (s *Store) ListChecklists(ctx context.Context, tripID int64) ([]Checklist, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, trip_id, name, kind, sort_order, created_at FROM checklists WHERE trip_id = ? ORDER BY sort_order, id`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Checklist{}
	idx := map[int64]int{}
	for rows.Next() {
		var c Checklist
		if err := rows.Scan(&c.ID, &c.TripID, &c.Name, &c.Kind, &c.SortOrder, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Items = []ChecklistItem{}
		idx[c.ID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	irows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.checklist_id, i.text, i.done, i.assignee_id, i.sort_order, i.created_at FROM checklist_items i JOIN checklists c ON c.id = i.checklist_id WHERE c.trip_id = ? ORDER BY i.sort_order, i.id`, tripID)
	if err != nil {
		return nil, err
	}
	defer irows.Close()
	for irows.Next() {
		var it ChecklistItem
		var done int
		if err := irows.Scan(&it.ID, &it.ChecklistID, &it.Text, &done, &it.AssigneeID, &it.SortOrder, &it.CreatedAt); err != nil {
			return nil, err
		}
		it.Done = done == 1
		if i, ok := idx[it.ChecklistID]; ok {
			out[i].Items = append(out[i].Items, it)
		}
	}
	return out, irows.Err()
}

// CreateChecklist inserts a list.
func (s *Store) CreateChecklist(ctx context.Context, c *Checklist) error {
	if c.Kind == "" {
		c.Kind = "todo"
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM checklists WHERE trip_id = ?`, c.TripID).Scan(&c.SortOrder)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO checklists (trip_id, name, kind, sort_order) VALUES (?, ?, ?, ?)`, c.TripID, c.Name, c.Kind, c.SortOrder)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	c.Items = []ChecklistItem{}
	return nil
}

// UpdateChecklist renames a list.
func (s *Store) UpdateChecklist(ctx context.Context, tripID, id int64, name, kind string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE checklists SET name = ?, kind = ? WHERE id = ? AND trip_id = ?`, name, kind, id, tripID)
	return err
}

// DeleteChecklist removes a list and its items.
func (s *Store) DeleteChecklist(ctx context.Context, tripID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM checklists WHERE id = ? AND trip_id = ?`, id, tripID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// checklistOwned checks the list belongs to the trip.
func (s *Store) checklistOwned(ctx context.Context, tripID, listID int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM checklists WHERE id = ? AND trip_id = ?`, listID, tripID).Scan(&n)
	return n > 0
}

// AddChecklistItem appends an item.
func (s *Store) AddChecklistItem(ctx context.Context, tripID int64, it *ChecklistItem) error {
	if !s.checklistOwned(ctx, tripID, it.ChecklistID) {
		return ErrNotFound
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM checklist_items WHERE checklist_id = ?`, it.ChecklistID).Scan(&it.SortOrder)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO checklist_items (checklist_id, text, done, assignee_id, sort_order) VALUES (?, ?, ?, ?, ?)`, it.ChecklistID, it.Text, boolInt(it.Done), it.AssigneeID, it.SortOrder)
	if err != nil {
		return err
	}
	it.ID, _ = res.LastInsertId()
	return nil
}

// UpdateChecklistItem saves text/done/assignee.
func (s *Store) UpdateChecklistItem(ctx context.Context, tripID int64, it *ChecklistItem) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE checklist_items SET text = ?, done = ?, assignee_id = ?, sort_order = ? WHERE id = ? AND checklist_id IN (SELECT id FROM checklists WHERE trip_id = ?)`, it.Text, boolInt(it.Done), it.AssigneeID, it.SortOrder, it.ID, tripID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetChecklistItem loads one item.
func (s *Store) GetChecklistItem(ctx context.Context, tripID, id int64) (*ChecklistItem, error) {
	var it ChecklistItem
	var done int
	err := s.DB.QueryRowContext(ctx, `SELECT i.id, i.checklist_id, i.text, i.done, i.assignee_id, i.sort_order, i.created_at FROM checklist_items i JOIN checklists c ON c.id = i.checklist_id WHERE i.id = ? AND c.trip_id = ?`, id, tripID).Scan(&it.ID, &it.ChecklistID, &it.Text, &done, &it.AssigneeID, &it.SortOrder, &it.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	it.Done = done == 1
	return &it, nil
}

// DeleteChecklistItem removes an item.
func (s *Store) DeleteChecklistItem(ctx context.Context, tripID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM checklist_items WHERE id = ? AND checklist_id IN (SELECT id FROM checklists WHERE trip_id = ?)`, id, tripID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- journal ----

const journalCols = `j.id, j.trip_id, j.user_id, COALESCE(u.name, ''), j.date, j.title, j.body, j.mood, j.lat, j.lng, j.created_at, j.updated_at`

func scanJournal(row interface{ Scan(...any) error }) (*JournalEntry, error) {
	var j JournalEntry
	if err := row.Scan(&j.ID, &j.TripID, &j.UserID, &j.UserName, &j.Date, &j.Title, &j.Body, &j.Mood, &j.Lat, &j.Lng, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &j, nil
}

// ListJournal returns entries by date.
func (s *Store) ListJournal(ctx context.Context, tripID int64) ([]JournalEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+journalCols+` FROM journal j LEFT JOIN users u ON u.id = j.user_id WHERE j.trip_id = ? ORDER BY j.date, j.id`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JournalEntry{}
	for rows.Next() {
		j, err := scanJournal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// LatestJournal returns the newest entries across the trips a user can see.
func (s *Store) LatestJournal(ctx context.Context, userID int64, limit int) ([]JournalEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+journalCols+`, t.name FROM journal j JOIN trips t ON t.id = j.trip_id LEFT JOIN users u ON u.id = j.user_id
		WHERE t.archived = 0 AND (t.owner_id = ? OR t.id IN (SELECT trip_id FROM trip_members WHERE user_id = ?)) ORDER BY j.date DESC, j.id DESC LIMIT ?`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JournalEntry{}
	for rows.Next() {
		var j JournalEntry
		if err := rows.Scan(&j.ID, &j.TripID, &j.UserID, &j.UserName, &j.Date, &j.Title, &j.Body, &j.Mood, &j.Lat, &j.Lng, &j.CreatedAt, &j.UpdatedAt, &j.TripName); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// GetJournal loads one entry.
func (s *Store) GetJournal(ctx context.Context, tripID, id int64) (*JournalEntry, error) {
	return scanJournal(s.DB.QueryRowContext(ctx, `SELECT `+journalCols+` FROM journal j LEFT JOIN users u ON u.id = j.user_id WHERE j.id = ? AND j.trip_id = ?`, id, tripID))
}

// CreateJournal inserts an entry.
func (s *Store) CreateJournal(ctx context.Context, j *JournalEntry) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO journal (trip_id, user_id, date, title, body, mood, lat, lng) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, j.TripID, j.UserID, j.Date, j.Title, j.Body, j.Mood, j.Lat, j.Lng)
	if err != nil {
		return err
	}
	j.ID, _ = res.LastInsertId()
	return nil
}

// UpdateJournal saves an entry.
func (s *Store) UpdateJournal(ctx context.Context, j *JournalEntry) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE journal SET date = ?, title = ?, body = ?, mood = ?, lat = ?, lng = ?, updated_at = ? WHERE id = ? AND trip_id = ?`, j.Date, j.Title, j.Body, j.Mood, j.Lat, j.Lng, Now(), j.ID, j.TripID)
	return err
}

// DeleteJournal removes an entry.
func (s *Store) DeleteJournal(ctx context.Context, tripID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM journal WHERE id = ? AND trip_id = ?`, id, tripID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- documents ----

const docCols = `id, trip_id, user_id, name, filename, mime, bytes, kind, link_kind, link_id, created_at`

func scanDoc(row interface{ Scan(...any) error }) (*Document, error) {
	var d Document
	if err := row.Scan(&d.ID, &d.TripID, &d.UserID, &d.Name, &d.Filename, &d.Mime, &d.Bytes, &d.Kind, &d.LinkKind, &d.LinkID, &d.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &d, nil
}

// ListDocuments returns a trip's files.
func (s *Store) ListDocuments(ctx context.Context, tripID int64) ([]Document, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+docCols+` FROM documents WHERE trip_id = ? ORDER BY kind, name`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// GetDocument loads one file record.
func (s *Store) GetDocument(ctx context.Context, tripID, id int64) (*Document, error) {
	return scanDoc(s.DB.QueryRowContext(ctx, `SELECT `+docCols+` FROM documents WHERE id = ? AND trip_id = ?`, id, tripID))
}

// CreateDocument inserts a record.
func (s *Store) CreateDocument(ctx context.Context, d *Document) error {
	if d.Kind == "" {
		d.Kind = "other"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO documents (trip_id, user_id, name, filename, mime, bytes, kind, link_kind, link_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, d.TripID, d.UserID, d.Name, d.Filename, d.Mime, d.Bytes, d.Kind, d.LinkKind, d.LinkID)
	if err != nil {
		return err
	}
	d.ID, _ = res.LastInsertId()
	return nil
}

// UpdateDocument renames / re-links.
func (s *Store) UpdateDocument(ctx context.Context, d *Document) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE documents SET name = ?, kind = ?, link_kind = ?, link_id = ? WHERE id = ? AND trip_id = ?`, d.Name, d.Kind, d.LinkKind, d.LinkID, d.ID, d.TripID)
	return err
}

// DeleteDocument removes the record (the caller removes the file).
func (s *Store) DeleteDocument(ctx context.Context, tripID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM documents WHERE id = ? AND trip_id = ?`, id, tripID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TripFilenames lists every stored file of a trip (for deletion).
func (s *Store) TripFilenames(ctx context.Context, tripID int64) []string {
	var out []string
	rows, err := s.DB.QueryContext(ctx, `SELECT filename FROM documents WHERE trip_id = ? UNION SELECT image_path FROM places WHERE trip_id = ? AND image_path <> '' UNION SELECT image_path FROM lodging WHERE trip_id = ? AND image_path <> '' UNION SELECT cover_path FROM trips WHERE id = ? AND cover_path <> ''`, tripID, tripID, tripID, tripID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil && f != "" {
			out = append(out, f)
		}
	}
	return out
}
