package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type databasePinger interface {
	PingContext(context.Context) error
}

type healthHandler struct {
	db databasePinger
}

func NewHealthHandler(db databasePinger) http.Handler {
	h := healthHandler{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.live)
	mux.HandleFunc("GET /readyz", h.ready)
	return mux
}

func (h healthHandler) live(w http.ResponseWriter, _ *http.Request) {
	writeHealth(w, http.StatusOK, "ok")
}

func (h healthHandler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.db == nil || h.db.PingContext(ctx) != nil {
		writeHealth(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	writeHealth(w, http.StatusOK, "ok")
}

func writeHealth(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}
