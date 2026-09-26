package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"datacluster/internal/config"
	"datacluster/internal/db"
)

type Handlers struct {
	db  *db.DB
	cfg *config.Config
}

func New(database *db.DB, cfg *config.Config) *Handlers {
	return &Handlers{db: database, cfg: cfg}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func (h *Handlers) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.AppToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != h.cfg.AppToken {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if h.cfg.AppToken == "" {
		writeJSON(w, http.StatusOK, map[string]string{"token": "no-auth"})
		return
	}
	if body.Password != h.cfg.AppPassword {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": h.cfg.AppToken})
}

func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.db.GetDashboardStats())
}
