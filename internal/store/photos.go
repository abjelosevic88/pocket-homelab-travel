package store

import "context"

// PlacePhoto is one picture of a place.
type PlacePhoto struct {
	ID        int64  `json:"id"`
	PlaceID   int64  `json:"place_id"`
	Path      string `json:"-"`
	Source    string `json:"source"`
	Credit    string `json:"credit"`
	License   string `json:"license"`
	SourceURL string `json:"source_url"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
}

// ListPlacePhotos returns a place's photos in order.
func (s *Store) ListPlacePhotos(ctx context.Context, placeID int64) ([]PlacePhoto, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, place_id, path, source, credit, license, source_url, sort_order, created_at FROM place_photos WHERE place_id = ? ORDER BY sort_order, id`, placeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlacePhoto{}
	for rows.Next() {
		var p PlacePhoto
		if err := rows.Scan(&p.ID, &p.PlaceID, &p.Path, &p.Source, &p.Credit, &p.License, &p.SourceURL, &p.SortOrder, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PhotosForTrip returns every photo of every place in a trip, keyed by place.
func (s *Store) PhotosForTrip(ctx context.Context, tripID int64) (map[int64][]PlacePhoto, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT ph.id, ph.place_id, ph.path, ph.source, ph.credit, ph.license, ph.source_url, ph.sort_order, ph.created_at FROM place_photos ph JOIN places p ON p.id = ph.place_id WHERE p.trip_id = ? ORDER BY ph.place_id, ph.sort_order, ph.id`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]PlacePhoto{}
	for rows.Next() {
		var p PlacePhoto
		if err := rows.Scan(&p.ID, &p.PlaceID, &p.Path, &p.Source, &p.Credit, &p.License, &p.SourceURL, &p.SortOrder, &p.CreatedAt); err != nil {
			return nil, err
		}
		out[p.PlaceID] = append(out[p.PlaceID], p)
	}
	return out, rows.Err()
}

// AddPlacePhoto appends a photo and keeps places.image_path pointing at the first one.
func (s *Store) AddPlacePhoto(ctx context.Context, p *PlacePhoto) error {
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM place_photos WHERE place_id = ?`, p.PlaceID).Scan(&p.SortOrder)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO place_photos (place_id, path, source, credit, license, source_url, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`, p.PlaceID, p.Path, p.Source, p.Credit, p.License, p.SourceURL, p.SortOrder)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return s.syncPlaceCover(ctx, p.PlaceID)
}

// GetPlacePhoto loads one photo (checked against the trip through the place).
func (s *Store) GetPlacePhoto(ctx context.Context, tripID, id int64) (*PlacePhoto, error) {
	var p PlacePhoto
	err := s.DB.QueryRowContext(ctx, `SELECT ph.id, ph.place_id, ph.path, ph.source, ph.credit, ph.license, ph.source_url, ph.sort_order, ph.created_at FROM place_photos ph JOIN places pl ON pl.id = ph.place_id WHERE ph.id = ? AND pl.trip_id = ?`, id, tripID).Scan(&p.ID, &p.PlaceID, &p.Path, &p.Source, &p.Credit, &p.License, &p.SourceURL, &p.SortOrder, &p.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

// DeletePlacePhoto removes a photo row.
func (s *Store) DeletePlacePhoto(ctx context.Context, placeID, id int64) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM place_photos WHERE id = ? AND place_id = ?`, id, placeID); err != nil {
		return err
	}
	return s.syncPlaceCover(ctx, placeID)
}

// SetPlaceCover moves a photo to the front.
func (s *Store) SetPlaceCover(ctx context.Context, placeID, id int64) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE place_photos SET sort_order = (SELECT COALESCE(MIN(sort_order), 1) - 1 FROM place_photos WHERE place_id = ?) WHERE id = ? AND place_id = ?`, placeID, id, placeID); err != nil {
		return err
	}
	return s.syncPlaceCover(ctx, placeID)
}

// PlacePhotoCount counts a place's photos.
func (s *Store) PlacePhotoCount(ctx context.Context, placeID int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM place_photos WHERE place_id = ?`, placeID).Scan(&n)
	return n
}

// PlacePhotoPaths lists file paths of a place's photos (for deletion).
func (s *Store) PlacePhotoPaths(ctx context.Context, placeID int64) []string {
	ps, _ := s.ListPlacePhotos(ctx, placeID)
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Path)
	}
	return out
}

func (s *Store) syncPlaceCover(ctx context.Context, placeID int64) error {
	var first string
	_ = s.DB.QueryRowContext(ctx, `SELECT path FROM place_photos WHERE place_id = ? ORDER BY sort_order, id LIMIT 1`, placeID).Scan(&first)
	_, err := s.DB.ExecContext(ctx, `UPDATE places SET image_path = ? WHERE id = ?`, first, placeID)
	return err
}
