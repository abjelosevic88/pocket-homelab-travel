// Package server wires the HTTP API, auth and the embedded SPA.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/config"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/dawarich"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/firefly"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/fx"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/geo"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/immich"
	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

// Version is injected at build time.
var Version = "dev"

// Server holds dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	log      *slog.Logger
	webFS    fs.FS
	started  time.Time
	requests atomic.Int64
	errors   atomic.Int64
	geo      *geo.Client
	fx       *fx.Client

	// one in-flight sync per trip
	syncMu  sync.Mutex
	syncing map[int64]bool
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS) *Server {
	return &Server{cfg: cfg, store: st, log: log, webFS: webFS, started: time.Now(),
		geo: geo.New(cfg.UserAgent, 25*time.Second), fx: fx.New(cfg.FXProvider, cfg.UserAgent, 8*time.Second), syncing: map[int64]bool{}}
}

// Bootstrap seeds the first admin (if configured), integration defaults and demo data.
func (s *Server) Bootstrap(ctx context.Context) error {
	if err := os.MkdirAll(s.cfg.FilesDir, 0o755); err != nil {
		return fmt.Errorf("files dir: %w", err)
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 && s.cfg.AuthMode == "local" && s.cfg.AdminEmail != "" && s.cfg.AdminPassword != "" {
		if len(s.cfg.AdminPassword) < 8 {
			return errors.New("ADMIN_PASSWORD must be at least 8 characters")
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), bcrypt.DefaultCost)
		u := &store.User{Email: s.cfg.AdminEmail, Name: s.cfg.AdminName, PasswordHash: string(h), Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		st, _ := s.store.GetSettings(ctx)
		st.SetupComplete = true
		if err := s.store.SaveSettings(ctx, st); err != nil {
			return err
		}
		s.log.Info("admin user created from environment", "email", u.Email)
	}
	// Environment integration defaults fill empty settings (the UI values win afterwards).
	st, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	changed := false
	set := func(dst *string, v string) {
		if *dst == "" && v != "" {
			*dst = v
			changed = true
		}
	}
	set(&st.FireflyURL, s.cfg.FireflyURL)
	set(&st.FireflyToken, s.cfg.FireflyToken)
	set(&st.DawarichURL, s.cfg.DawarichURL)
	set(&st.DawarichAPIKey, s.cfg.DawarichAPIKey)
	set(&st.ImmichURL, s.cfg.ImmichURL)
	set(&st.ImmichAPIKey, s.cfg.ImmichAPIKey)
	set(&st.ImmichPublicURL, s.cfg.ImmichPublicURL)
	set(&st.OpenAIKey, s.cfg.OpenAIKey)
	set(&st.OpenAIModel, s.cfg.OpenAIModel)
	set(&st.OpenAIBaseURL, s.cfg.OpenAIBaseURL)
	if st.HomeCurrency == "EUR" && s.cfg.DefaultCurrency != "EUR" {
		st.HomeCurrency = s.cfg.DefaultCurrency
		changed = true
	}
	if changed {
		if err := s.store.SaveSettings(ctx, st); err != nil {
			return err
		}
	}
	if s.cfg.DemoData {
		if err := s.seedDemo(ctx); err != nil {
			return err
		}
	}
	_ = s.store.PurgeSessions(ctx)
	go s.janitor(ctx)
	return nil
}

// janitor syncs Firefly for active trips every 30 minutes and trims caches.
func (s *Server) janitor(ctx context.Context) {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.autoSync(ctx)
		}
	}
}

// ---- integration clients, built from the current settings ----

func (s *Server) clients(ctx context.Context) (store.Settings, *firefly.Client, *dawarich.Client, *immich.Client) {
	st, _ := s.store.GetSettings(ctx)
	return st, firefly.New(st.FireflyURL, st.FireflyToken, 20*time.Second), dawarich.New(st.DawarichURL, st.DawarichAPIKey, 60*time.Second), immich.New(st.ImmichURL, st.ImmichAPIKey, 30*time.Second)
}

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(120 * time.Second))

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/setup", s.handleSetup)
		r.Get("/auth/status", s.handleAuthStatus)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)

			r.Get("/dashboard", s.handleDashboard)
			r.Get("/home", s.handleHome)
			r.Get("/currencies", s.handleCurrencies)
			r.Get("/fx", s.handleFX)
			r.Get("/meta", s.handleMeta)

			// outside world
			r.Get("/geo/search", s.handleGeoSearch)
			r.Get("/geo/reverse", s.handleGeoReverse)
			r.Get("/geo/discover", s.handleGeoDiscover)
			r.Get("/geo/wiki", s.handleGeoWiki)
			r.Get("/geo/weather", s.handleGeoWeather)

			r.Get("/trips", s.handleListTrips)
			r.Post("/trips", s.handleCreateTrip)
			r.Route("/trips/{tripID}", func(r chi.Router) {
				r.Use(s.tripAccess)
				r.Get("/", s.handleGetTrip)
				r.Put("/", s.handleUpdateTrip)
				r.Delete("/", s.handleDeleteTrip)
				r.Get("/overview", s.handleTripOverview)
				r.Get("/activity", s.handleTripActivity)
				r.Put("/members", s.handleTripMembers)
				r.Post("/cover", s.handleTripCover)
				r.Get("/cover", s.handleTripCoverImage)
				r.Delete("/cover", s.handleTripCoverDelete)
				r.Post("/duplicate", s.handleDuplicateTrip)

				r.Get("/places", s.handleListPlaces)
				r.Post("/places", s.handleCreatePlace)
				r.Put("/places/reorder", s.handleReorderPlaces)
				r.Get("/places/{id}", s.handleGetPlace)
				r.Put("/places/{id}", s.handleUpdatePlace)
				r.Delete("/places/{id}", s.handleDeletePlace)
				r.Post("/places/{id}/image", s.handlePlaceImage)
				r.Get("/places/{id}/image", s.handlePlaceImageGet)

				r.Get("/lodging", s.handleListLodging)
				r.Post("/lodging", s.handleCreateLodging)
				r.Put("/lodging/{id}", s.handleUpdateLodging)
				r.Delete("/lodging/{id}", s.handleDeleteLodging)
				r.Post("/lodging/{id}/image", s.handleLodgingImage)
				r.Get("/lodging/{id}/image", s.handleLodgingImageGet)

				r.Get("/transport", s.handleListTransport)
				r.Post("/transport", s.handleCreateTransport)
				r.Put("/transport/{id}", s.handleUpdateTransport)
				r.Delete("/transport/{id}", s.handleDeleteTransport)

				r.Get("/expenses", s.handleListExpenses)
				r.Post("/expenses", s.handleCreateExpense)
				r.Put("/expenses/{id}", s.handleUpdateExpense)
				r.Delete("/expenses/{id}", s.handleDeleteExpense)
				r.Get("/budget", s.handleBudget)
				r.Post("/firefly/sync", s.handleFireflySync)

				r.Get("/checklists", s.handleListChecklists)
				r.Post("/checklists", s.handleCreateChecklist)
				r.Put("/checklists/{id}", s.handleUpdateChecklist)
				r.Delete("/checklists/{id}", s.handleDeleteChecklist)
				r.Post("/checklists/{id}/items", s.handleAddChecklistItem)
				r.Put("/checklists/items/{id}", s.handleUpdateChecklistItem)
				r.Delete("/checklists/items/{id}", s.handleDeleteChecklistItem)

				r.Get("/journal", s.handleListJournal)
				r.Post("/journal", s.handleCreateJournal)
				r.Put("/journal/{id}", s.handleUpdateJournal)
				r.Delete("/journal/{id}", s.handleDeleteJournal)

				r.Get("/documents", s.handleListDocuments)
				r.Post("/documents", s.handleUploadDocument)
				r.Put("/documents/{id}", s.handleUpdateDocument)
				r.Get("/documents/{id}/file", s.handleDocumentFile)
				r.Delete("/documents/{id}", s.handleDeleteDocument)

				r.Get("/weather", s.handleTripWeather)
				r.Get("/dawarich/route", s.handleDawarichRoute)
				r.Get("/dawarich/timeline", s.handleDawarichTimeline)
				r.Get("/photos", s.handleTripPhotos)
				r.Get("/photos/{id}/thumb", s.handlePhotoThumb)
				r.Post("/refresh", s.handleTripRefresh)

				r.Get("/export/calendar.ics", s.handleExportICS)
				r.Get("/export/trip.gpx", s.handleExportGPX)
				r.Get("/export/trip.json", s.handleExportJSON)

				r.Post("/ai/suggest", s.handleAISuggest)
				r.Post("/ai/plan", s.handleAIPlan)
				r.Post("/ai/describe", s.handleAIDescribe)
				r.Post("/ai/packing", s.handleAIPacking)
				r.Post("/ai/recap", s.handleAIRecap)
			})

			r.Get("/firefly/tags", s.handleFireflyTags)
			r.Post("/integrations/test", s.handleIntegrationTest)
			r.Post("/import/adventurelog/collections", s.handleALCollections)
			r.Post("/import/adventurelog", s.handleALImport)
			r.Post("/import/trip", s.handleImportJSON)

			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Put("/users/{id}", s.handleUpdateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/settings/system", s.handleSystemInfo)

			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)

			r.Get("/backup", s.handleBackup)
		})
	})

	// Calendar subscription: token in the query so phones can subscribe.
	r.Get("/cal/{tripID}.ics", s.handleCalendarFeed)

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryFloat(r *http.Request, name string) (float64, bool) {
	f, err := strconv.ParseFloat(r.URL.Query().Get(name), 64)
	return f, err == nil
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_travel_info Build info.\n# TYPE pocket_travel_info gauge\npocket_travel_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_travel_uptime_seconds Process uptime.\n# TYPE pocket_travel_uptime_seconds gauge\npocket_travel_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_travel_http_requests_total HTTP requests served.\n# TYPE pocket_travel_http_requests_total counter\npocket_travel_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_travel_http_errors_total HTTP 5xx responses.\n# TYPE pocket_travel_http_errors_total counter\npocket_travel_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_travel_rows Rows per table.\n# TYPE pocket_travel_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_travel_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}

// ---------- files ----------

// saveFile writes bytes under FILES_DIR/<trip>/ with a random name and returns the relative path.
func (s *Server) saveFile(tripID int64, ext string, data []byte) (string, error) {
	dir := filepath.Join(s.cfg.FilesDir, strconv.FormatInt(tripID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext = strings.ToLower(filepath.Ext("x" + ext))
	if len(ext) > 8 {
		ext = ""
	}
	name := store.RandomToken(12) + ext
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}
	return strconv.FormatInt(tripID, 10) + "/" + name, nil
}

func (s *Server) filePath(rel string) string {
	rel = filepath.Clean("/" + rel)
	return filepath.Join(s.cfg.FilesDir, rel)
}

func (s *Server) removeFile(rel string) {
	if rel == "" {
		return
	}
	_ = os.Remove(s.filePath(rel))
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, rel, mime, download string) {
	if rel == "" {
		writeErr(w, http.StatusNotFound, "no file")
		return
	}
	p := s.filePath(rel)
	if _, err := os.Stat(p); err != nil {
		writeErr(w, http.StatusNotFound, "file missing")
		return
	}
	if mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	if download != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(download, `"`, "")+`"`)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}

// readUpload reads one multipart file field (or the raw body) up to the limit.
func (s *Server) readUpload(r *http.Request, field string) ([]byte, string, string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, s.cfg.MaxUploadBytes)
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
			return nil, "", "", err
		}
		f, hdr, err := r.FormFile(field)
		if err != nil {
			return nil, "", "", err
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return nil, "", "", err
		}
		return b, hdr.Filename, hdr.Header.Get("Content-Type"), nil
	}
	b, err := io.ReadAll(r.Body)
	return b, r.URL.Query().Get("name"), ct, err
}
