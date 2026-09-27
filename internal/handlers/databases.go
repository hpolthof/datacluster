package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"datacluster/internal/crypto"
	"datacluster/internal/pg"
)

func (h *Handlers) ListManagedDatabases(w http.ResponseWriter, r *http.Request) {
	var serverID *int
	if sid := r.URL.Query().Get("server_id"); sid != "" {
		id, err := strconv.Atoi(sid)
		if err == nil {
			serverID = &id
		}
	}
	dbs, err := h.db.ListManagedDatabases(serverID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dbs)
}

func (h *Handlers) CreateManagedDatabase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServerID      int    `json:"server_id"`
		DatabaseName  string `json:"database_name"`
		OwnerUser     string `json:"owner_user"`
		OwnerPassword string `json:"owner_password"`
		Notes         string `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.ServerID == 0 || body.DatabaseName == "" || body.OwnerUser == "" || body.OwnerPassword == "" {
		writeError(w, http.StatusBadRequest, "server_id, database_name, owner_user and owner_password are required")
		return
	}

	params, err := h.serverConnParams(body.ServerID)
	if err != nil || params == nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := pg.CreateIsolatedSet(ctx, *params, body.DatabaseName, body.OwnerUser, body.OwnerPassword); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	enc, err := crypto.Encrypt(body.OwnerPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	id, err := h.db.CreateManagedDatabase(body.ServerID, body.DatabaseName, body.OwnerUser, enc, body.Notes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	mdb, _, _ := h.db.GetManagedDatabase(int(id))
	writeJSON(w, http.StatusCreated, mdb)
}

func (h *Handlers) GetManagedDatabaseDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	detail, enc, err := h.db.GetManagedDatabaseDetail(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	pass, err := crypto.Decrypt(enc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decryption failed")
		return
	}
	detail.OwnerPassword = pass
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handlers) DeleteManagedDatabase(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	mdb, _, err := h.db.GetManagedDatabase(id)
	if err != nil || mdb == nil {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}

	params, err := h.serverConnParams(mdb.ServerID)
	if err == nil && params != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		_ = pg.DropIsolatedSet(ctx, *params, mdb.DatabaseName, mdb.OwnerUser)
	}

	if err := h.db.DeleteManagedDatabase(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
