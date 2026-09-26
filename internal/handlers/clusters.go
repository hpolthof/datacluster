package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (h *Handlers) ListClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := h.db.ListClusters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, clusters)
}

func (h *Handlers) GetCluster(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.db.GetCluster(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) CreateCluster(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		ReplicationType string `json:"replication_type"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.ReplicationType == "" {
		body.ReplicationType = "logical"
	}

	id, err := h.db.CreateCluster(body.Name, body.Description, body.ReplicationType)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	c, _ := h.db.GetCluster(int(id))
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handlers) UpdateCluster(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		ReplicationType string `json:"replication_type"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.db.UpdateCluster(id, body.Name, body.Description, body.ReplicationType); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	c, _ := h.db.GetCluster(id)
	writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) DeleteCluster(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.db.DeleteCluster(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) AddClusterMember(w http.ResponseWriter, r *http.Request) {
	clusterID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cluster id")
		return
	}
	var body struct {
		ServerID int    `json:"server_id"`
		Role     string `json:"role"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.ServerID == 0 {
		writeError(w, http.StatusBadRequest, "server_id is required")
		return
	}
	if body.Role == "" {
		body.Role = "replica"
	}
	if err := h.db.AddClusterMember(clusterID, body.ServerID, body.Role); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	c, _ := h.db.GetCluster(clusterID)
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handlers) RemoveClusterMember(w http.ResponseWriter, r *http.Request) {
	clusterID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cluster id")
		return
	}
	serverID, err := strconv.Atoi(chi.URLParam(r, "serverId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")
		return
	}
	if err := h.db.RemoveClusterMember(clusterID, serverID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
