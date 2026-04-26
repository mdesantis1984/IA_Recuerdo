// Package server implements the HTTP REST API for IA_Recuerdo.
// Includes auth middleware, health, metrics, and import/export endpoints.
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mdesantis1984/ia-recuerdo/internal/cache"
	"github.com/mdesantis1984/ia-recuerdo/internal/mcp"
	"github.com/mdesantis1984/ia-recuerdo/internal/store"
	"github.com/mdesantis1984/ia-recuerdo/pkg/types"
)

// Server is the HTTP server for IA_Recuerdo.
type Server struct {
	store   *store.Store
	handler *mcp.Handler
	cache   *cache.Cache
	addr    string
	mux     *http.ServeMux
}

// New creates and configures a new Server. Pass nil for ca to disable caching.
func New(s *store.Store, h *mcp.Handler, ca *cache.Cache, addr string) *Server {
	if ca == nil {
		ca = cache.New("") // no-op cache
	}
	srv := &Server{store: s, handler: h, cache: ca, addr: addr, mux: http.NewServeMux()}
	srv.registerRoutes()
	return srv
}

func (s *Server) registerRoutes() {
	// MCP HTTP transport (no auth required for MCP — uses api key)
	s.handler.RegisterHTTPRoutes(s.mux)

	// Health
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Auth-protected REST API
	read := s.authMiddleware("read")
	write := s.authMiddleware("write")
	admin := s.authMiddleware("admin")

	// Observations
	s.mux.HandleFunc("GET /api/v1/observations", read(s.listObservations))
	s.mux.HandleFunc("POST /api/v1/observations", write(s.createObservation))
	s.mux.HandleFunc("GET /api/v1/observations/{id}", read(s.getObservation))
	s.mux.HandleFunc("DELETE /api/v1/observations/{id}", write(s.deleteObservation))

	// Context (recent session observations)
	s.mux.HandleFunc("GET /api/v1/context", read(s.getContext))

	// Search
	s.mux.HandleFunc("GET /api/v1/search", read(s.searchObservations))

	// Stats
	s.mux.HandleFunc("GET /api/v1/stats", read(s.getStats))

	// Metrics
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Import / Export
	s.mux.HandleFunc("GET /api/v1/export", read(s.exportAll))
	s.mux.HandleFunc("POST /api/v1/import", write(s.importAll))

	// API keys (admin) — POST is bootstrap-aware (allows first key without auth)
	s.mux.HandleFunc("GET /api/v1/keys", admin(s.listKeys))
	s.mux.HandleFunc("POST /api/v1/keys", s.createKey)        // bootstrap: no auth if 0 keys
	s.mux.HandleFunc("DELETE /api/v1/keys/{id}", admin(s.revokeKey))
}

// Start runs the HTTP server. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	log.Printf("[server] Listening on %s", s.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Auth middleware
// ─────────────────────────────────────────────────────────────────

func (s *Server) authMiddleware(requiredScopes ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !s.authorizeRequest(w, r, requiredScopes...) {
				return
			}
			next(w, r)
		}
	}
}

func (s *Server) authorizeRequest(w http.ResponseWriter, r *http.Request, requiredScopes ...string) bool {
	key := extractKey(r)
	if key == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing_credentials"})
		return false
	}
	record, err := s.store.LookupAPIKeyByHash(r.Context(), hashKey(key))
	if err != nil || record == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return false
	}
	if record.Revoked {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "revoked_credentials"})
		return false
	}
	if len(requiredScopes) > 0 && !store.HasScopes(record.Scopes, requiredScopes...) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient_scope"})
		return false
	}
	return true
}

// ─────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "ok",
		"observations": st.TotalObservations,
		"sessions":     st.TotalSessions,
	})
}

func (s *Server) listObservations(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	limit := 50
	obs, err := s.store.RecentContext(r.Context(), defaultStr(project, "default"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"observations": obs, "count": len(obs)})
}

func (s *Server) getObservation(w http.ResponseWriter, r *http.Request) {
	id := int64(0)
	fmt.Sscanf(r.PathValue("id"), "%d", &id)
	o, err := s.store.GetObservation(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	if o == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) deleteObservation(w http.ResponseWriter, r *http.Request) {
	id := int64(0)
	fmt.Sscanf(r.PathValue("id"), "%d", &id)
	hard := r.URL.Query().Get("hard") == "true"
	if err := s.store.DeleteObservation(r.Context(), id, hard); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) searchObservations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}
	project := r.URL.Query().Get("project")

	// Cache lookup
	fp := fmt.Sprintf("%s|%s", q, project)
	var cachedResp map[string]interface{}
	if s.cache.GetSearch(r.Context(), fp, &cachedResp) {
		writeJSON(w, http.StatusOK, cachedResp)
		return
	}

	results, err := s.store.Search(r.Context(), q, project, 20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	resp := map[string]interface{}{"results": results, "count": len(results)}
	s.cache.SetSearch(r.Context(), fp, resp)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createObservation(w http.ResponseWriter, r *http.Request) {
	var o types.Observation
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if o.Title == "" || o.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title and content are required"})
		return
	}
	if o.Type == "" {
		o.Type = types.TypeDiscovery
	}
	if o.Scope == "" {
		o.Scope = types.ScopeProject
	}
	saved, err := s.store.SaveObservation(r.Context(), &o)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) getContext(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	obs, err := s.store.RecentContext(r.Context(), defaultStr(project, "default"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"context": obs, "count": len(obs)})
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "ia_recuerdo_up 1\n")
	fmt.Fprintf(w, "ia_recuerdo_total_observations %d\n", st.TotalObservations)
	fmt.Fprintf(w, "ia_recuerdo_total_sessions %d\n", st.TotalSessions)
	fmt.Fprintf(w, "ia_recuerdo_total_prompts %d\n", st.TotalPrompts)
	fmt.Fprintf(w, "ia_recuerdo_total_projects %d\n", st.TotalProjects)
}

// exportAll dumps all observations as JSON.
func (s *Server) exportAll(w http.ResponseWriter, r *http.Request) {
	obs, err := s.store.ListAll(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="ia-recuerdo-export.json"`)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"version":      "1.0",
		"exported_at":  time.Now().UTC(),
		"observations": obs,
		"count":        len(obs),
	})
}

// importAll ingests observations from a JSON export.
func (s *Server) importAll(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Observations []types.Observation `json:"observations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.store.BulkInsert(r.Context(), payload.Observations); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "imported",
		"imported": len(payload.Observations),
	})
}

// ─────────────────────────────────────────────────────────────────
// API keys management
// ─────────────────────────────────────────────────────────────────

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.store.ListAPIKeys(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	// Bootstrap: if no keys exist yet, allow creation without auth.
	// Once the first key is created, all subsequent requests require a valid key.
	keys, err := s.store.ListAPIKeys(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	if len(keys) > 0 {
		if !s.authorizeRequest(w, r, "admin") {
			return
		}
	}

	var body struct {
		Name   string `json:"name"`
		Scopes string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if body.Scopes == "" {
		body.Scopes = "read,write"
	}
	raw, hash, err := generateAPIKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	id := uuid.New().String()
	if err := s.store.CreateAPIKey(r.Context(), id, body.Name, hash, body.Scopes); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      id,
		"name":    body.Name,
		"key":     raw, // shown only once
		"message": "Store this key securely — it will not be shown again",
	})
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.RevokeAPIKey(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ─────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────

func generateAPIKey() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = "ir_" + base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = fmt.Sprintf("%x", h)
	return
}

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}

func extractKey(r *http.Request) string {
	key := r.Header.Get("X-Api-Key")
	if key != "" {
		return key
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(err error) map[string]string {
	return map[string]string{"error": err.Error()}
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
