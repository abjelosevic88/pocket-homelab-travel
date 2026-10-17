package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// photoLookups serialises outbound Wikipedia/Commons work: one place at a time
// keeps us polite with the APIs and the download bandwidth sane.
var photoLookups = make(chan struct{}, 1)

// fetchPlacePhotos finds and stores up to max pictures for a place. It is
// safe to call in a goroutine; failures are logged, never surfaced.
func (s *Server) fetchPlacePhotos(ctx context.Context, t *store.Trip, p *store.Place, max int) int {
	photoLookups <- struct{}{}
	defer func() { <-photoLookups }()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	hint := t.Destination
	if hint == "" {
		hint = p.Address
	}
	// For eateries and shops an article is rare and "nearby" is a street
	// scene; keep those to a tight radius so we do not decorate a bar with the
	// cathedral next door.
	strict := p.Category == "food" || p.Category == "cafe" || p.Category == "bar" || p.Category == "shop" || p.Category == "nightlife" || p.Category == "other"
	var lat, lng *float64
	if !strict {
		lat, lng = p.Lat, p.Lng
	}
	cands, err := s.geo.FindPhotos(ctx, p.Name, hint, lat, lng, max)
	if err != nil {
		s.log.Warn("find photos", "place", p.Name, "err", err)
		return 0
	}
	if len(cands) == 0 && strict && p.Lat != nil {
		// Tight radius only.
		cands, _ = s.geo.FindPhotos(ctx, p.Name, hint, nil, nil, max)
	}
	have, _ := s.store.ListPlacePhotos(ctx, p.ID)
	known := map[string]bool{}
	for _, h := range have {
		known[h.SourceURL] = true
	}
	n := 0
	for _, c := range cands {
		if known[c.PageURL] {
			continue
		}
		b, ct, err := s.geo.Download(ctx, c.URL)
		if err != nil || len(b) == 0 || !strings.HasPrefix(ct, "image/") {
			continue
		}
		rel, err := s.saveFile(t.ID, extOf(c.URL, ct), b)
		if err != nil {
			continue
		}
		ph := &store.PlacePhoto{PlaceID: p.ID, Path: rel, Source: c.Source, Credit: c.Credit, License: c.License, SourceURL: c.PageURL}
		if err := s.store.AddPlacePhoto(ctx, ph); err != nil {
			s.removeFile(rel)
			continue
		}
		n++
	}
	if n > 0 {
		s.store.TouchTrip(ctx, t.ID)
		s.log.Info("photos found", "place", p.Name, "n", n)
	}
	return n
}

// autoPhotos kicks a background lookup for a new place without photos.
func (s *Server) autoPhotos(t *store.Trip, p *store.Place) {
	st, _ := s.store.GetSettings(context.Background())
	if !st.AutoPhotos || p.Name == "" {
		return
	}
	tt, pp := *t, *p
	go s.fetchPlacePhotos(context.Background(), &tt, &pp, 4)
}

func (s *Server) handleListPlacePhotos(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	p, err := s.store.GetPlace(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get place")
		return
	}
	ph, err := s.store.ListPlacePhotos(r.Context(), p.ID)
	if err != nil {
		s.fail(w, err, "photos")
		return
	}
	writeJSON(w, http.StatusOK, ph)
}

// handleFindPlacePhotos runs the lookup now and returns the gallery.
func (s *Server) handleFindPlacePhotos(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	p, err := s.store.GetPlace(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get place")
		return
	}
	n := s.fetchPlacePhotos(r.Context(), t, p, 4)
	ph, _ := s.store.ListPlacePhotos(r.Context(), p.ID)
	writeJSON(w, http.StatusOK, map[string]any{"added": n, "photos": ph})
}

// handleFindMissingPhotos looks up every place of the trip that has none, in the background.
func (s *Server) handleFindMissingPhotos(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	places, err := s.store.ListPlaces(r.Context(), t.ID)
	if err != nil {
		s.fail(w, err, "places")
		return
	}
	var todo []store.Place
	for _, p := range places {
		if len(p.Photos) == 0 {
			todo = append(todo, p)
		}
	}
	tt := *t
	go func() {
		var wg sync.WaitGroup
		for _, p := range todo {
			wg.Add(1)
			pp := p
			go func() { defer wg.Done(); s.fetchPlacePhotos(context.Background(), &tt, &pp, 4) }()
		}
		wg.Wait()
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": len(todo)})
}

func (s *Server) handleUploadPlacePhoto(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	p, err := s.store.GetPlace(r.Context(), t.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get place")
		return
	}
	b, name, ct, err := s.readUpload(r, "file")
	if err != nil || len(b) == 0 || !strings.HasPrefix(ct, "image/") {
		writeErr(w, http.StatusBadRequest, "an image file is required")
		return
	}
	rel, err := s.saveFile(t.ID, extOf(name, ct), b)
	if err != nil {
		s.fail(w, err, "save image")
		return
	}
	ph := &store.PlacePhoto{PlaceID: p.ID, Path: rel, Source: "upload"}
	if err := s.store.AddPlacePhoto(r.Context(), ph); err != nil {
		s.fail(w, err, "add photo")
		return
	}
	list, _ := s.store.ListPlacePhotos(r.Context(), p.ID)
	writeJSON(w, http.StatusCreated, list)
}

func (s *Server) handlePlacePhotoFile(w http.ResponseWriter, r *http.Request) {
	t := tripFrom(r.Context()).trip
	ph, err := s.store.GetPlacePhoto(r.Context(), t.ID, idParam(r, "photoID"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	s.serveFile(w, r, ph.Path, "", "")
}

func (s *Server) handleDeletePlacePhoto(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	ph, err := s.store.GetPlacePhoto(r.Context(), t.ID, idParam(r, "photoID"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	if err := s.store.DeletePlacePhoto(r.Context(), ph.PlaceID, ph.ID); err != nil {
		s.fail(w, err, "delete photo")
		return
	}
	s.removeFile(ph.Path)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetPlaceCover(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	t := tripFrom(r.Context()).trip
	ph, err := s.store.GetPlacePhoto(r.Context(), t.ID, idParam(r, "photoID"))
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	if err := s.store.SetPlaceCover(r.Context(), ph.PlaceID, ph.ID); err != nil {
		s.fail(w, err, "set cover")
		return
	}
	list, _ := s.store.ListPlacePhotos(r.Context(), ph.PlaceID)
	writeJSON(w, http.StatusOK, list)
}
