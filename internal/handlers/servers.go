package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"datacluster/internal/config"
	"datacluster/internal/crypto"
	"datacluster/internal/pg"
)

func (h *Handlers) ListServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.db.ListServers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *Handlers) GetServer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	s, err := h.db.GetServer(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) CreateServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string `json:"name"`
		Host          string `json:"host"`
		Port          int    `json:"port"`
		AdminUser     string `json:"admin_user"`
		AdminPassword string `json:"admin_password"`
		SSLMode       string `json:"ssl_mode"`
		Notes         string `json:"notes"`
		RelayID       *int   `json:"relay_id"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name == "" || body.Host == "" || body.AdminUser == "" || body.AdminPassword == "" {
		writeError(w, http.StatusBadRequest, "name, host, admin_user and admin_password are required")
		return
	}
	if body.Port == 0 {
		body.Port = 5432
	}
	if body.SSLMode == "" {
		body.SSLMode = "prefer"
	}

	enc, err := crypto.Encrypt(body.AdminPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	id, err := h.db.CreateServer(body.Name, body.Host, body.Port, body.AdminUser, enc, body.SSLMode, body.Notes, body.RelayID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	s, _ := h.db.GetServer(int(id))
	writeJSON(w, http.StatusCreated, s)
}

func (h *Handlers) UpdateServer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, existingAdminEnc, _, err := h.db.GetServerWithPassword(id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}

	var body struct {
		Name          string `json:"name"`
		Host          string `json:"host"`
		Port          int    `json:"port"`
		AdminUser     string `json:"admin_user"`
		AdminPassword string `json:"admin_password"`
		SSLMode       string `json:"ssl_mode"`
		Notes         string `json:"notes"`
		RelayID       *int   `json:"relay_id"` // null clears relay
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name == "" {
		body.Name = existing.Name
	}
	if body.Host == "" {
		body.Host = existing.Host
	}
	if body.Port == 0 {
		body.Port = existing.Port
	}
	if body.AdminUser == "" {
		body.AdminUser = existing.AdminUser
	}
	if body.SSLMode == "" {
		body.SSLMode = existing.SSLMode
	}

	adminEnc := existingAdminEnc
	if body.AdminPassword != "" {
		adminEnc, err = crypto.Encrypt(body.AdminPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
	}

	if err := h.db.UpdateServer(id, body.Name, body.Host, body.Port, body.AdminUser, adminEnc, body.SSLMode, body.Notes, body.RelayID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s, _ := h.db.GetServer(id)
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) DeleteServer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.db.DeleteServer(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) TestServer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	params, err := h.serverConnParams(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	version, err := pg.TestConnection(ctx, *params)
	if err != nil {
		_ = h.db.UpdateServerStatus(id, "offline")
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	_ = h.db.UpdateServerStatus(id, "online")
	writeJSON(w, http.StatusOK, map[string]string{"version": version})
}

func (h *Handlers) ListServerDatabases(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	params, err := h.serverConnParams(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	dbs, err := pg.ListDatabases(ctx, *params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dbs)
}

func (h *Handlers) GetServerInfo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	params, err := h.serverConnParams(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	info, err := pg.GetServerInfo(ctx, *params)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	for _, database := range info.Databases {
		_ = h.db.SaveDatabaseStatistics(id, database.Name, database.TableCount, database.SizeBytes)
	}
	writeJSON(w, http.StatusOK, info)
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func (h *Handlers) serverConnParams(serverID int) (*pg.ConnParams, error) {
	s, adminEnc, relayEnc, err := h.db.GetServerWithPassword(serverID)
	if err != nil || s == nil {
		return nil, err
	}
	pass, err := crypto.Decrypt(adminEnc)
	if err != nil {
		return nil, err
	}
	params := &pg.ConnParams{
		Host:     s.Host,
		Port:     s.Port,
		User:     s.AdminUser,
		Password: pass,
		SSLMode:  s.SSLMode,
	}

	if s.RelayID != nil {
		// Named relay takes priority
		relay, relayPwEnc, err := h.db.GetRelay(*s.RelayID)
		if err != nil || relay == nil {
			return nil, fmt.Errorf("relay not found")
		}
		relayPass, err := crypto.Decrypt(relayPwEnc)
		if err != nil {
			return nil, err
		}
		params.RelayURL = relay.URL
		params.RelayToken = config.TokenFromPassword(relayPass)
	} else if s.RelayURL != "" && relayEnc != "" {
		// Legacy inline relay config
		relayPass, err := crypto.Decrypt(relayEnc)
		if err != nil {
			return nil, err
		}
		params.RelayURL = s.RelayURL
		params.RelayToken = config.TokenFromPassword(relayPass)
	}
	return params, nil
}
