package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"datacluster/internal/crypto"
)

func (h *Handlers) ListRelays(w http.ResponseWriter, r *http.Request) {
	relays, err := h.db.ListRelays()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, relays)
}

func (h *Handlers) CreateRelay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name == "" || body.URL == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "name, url and password are required")
		return
	}
	enc, err := crypto.Encrypt(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encryption failed")
		return
	}
	id, err := h.db.CreateRelay(body.Name, body.URL, enc)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	relay, _, _ := h.db.GetRelay(int(id))
	writeJSON(w, http.StatusCreated, relay)
}

func (h *Handlers) UpdateRelay(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, existingEnc, err := h.db.GetRelay(id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "relay not found")
		return
	}
	var body struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name == "" {
		body.Name = existing.Name
	}
	if body.URL == "" {
		body.URL = existing.URL
	}
	enc := existingEnc
	if body.Password != "" {
		enc, err = crypto.Encrypt(body.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
	}
	if err := h.db.UpdateRelay(id, body.Name, body.URL, enc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	relay, _, _ := h.db.GetRelay(id)
	writeJSON(w, http.StatusOK, relay)
}

func (h *Handlers) DeleteRelay(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.db.DeleteRelay(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
