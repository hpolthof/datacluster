package handlers

import (
	"context"
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
		RelayURL      string `json:"relay_url"`
		RelayPassword string `json:"relay_password"`
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
	relayEnc := ""
	if body.RelayPassword != "" {
		relayEnc, err = crypto.Encrypt(body.RelayPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
	}

	id, err := h.db.CreateServer(body.Name, body.Host, body.Port, body.AdminUser, enc, body.SSLMode, body.Notes, body.RelayURL, relayEnc)
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

	existing, existingAdminEnc, existingRelayEnc, err := h.db.GetServerWithPassword(id)
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
		RelayURL      string `json:"relay_url"`
		RelayPassword string `json:"relay_password"`
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
	if body.RelayURL == "" {
		body.RelayURL = existing.RelayURL
	}

	adminEnc := existingAdminEnc
	if body.AdminPassword != "" {
		adminEnc, err = crypto.Encrypt(body.AdminPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
	}
	relayEnc := existingRelayEnc
	if body.RelayPassword != "" {
		relayEnc, err = crypto.Encrypt(body.RelayPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
	}

	if err := h.db.UpdateServer(id, body.Name, body.Host, body.Port, body.AdminUser, adminEnc, body.SSLMode, body.Notes, body.RelayURL, relayEnc); err != nil {
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
		RelayURL: s.RelayURL,
	}
	if s.RelayURL != "" && relayEnc != "" {
		relayPass, err := crypto.Decrypt(relayEnc)
		if err != nil {
			return nil, err
		}
		params.RelayToken = config.TokenFromPassword(relayPass)
	}
	return params, nil
}
