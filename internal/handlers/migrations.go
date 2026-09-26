package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"datacluster/internal/crypto"
	"datacluster/internal/migration"
	"datacluster/internal/pg"
)

func (h *Handlers) ListMigrations(w http.ResponseWriter, r *http.Request) {
	migs, err := h.db.ListMigrations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, migs)
}

func (h *Handlers) GetMigration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	m, err := h.db.GetMigration(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, "migration not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) CreateMigration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name           string `json:"name"`
		SourceServerID int    `json:"source_server_id"`
		SourceDatabase string `json:"source_database"`
		TargetServerID int    `json:"target_server_id"`
		TargetDatabase string `json:"target_database"`
		MigrateUsers   bool   `json:"migrate_users"`
		CleanupSource  bool   `json:"cleanup_source"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.SourceServerID == 0 || body.SourceDatabase == "" || body.TargetServerID == 0 || body.TargetDatabase == "" {
		writeError(w, http.StatusBadRequest, "source_server_id, source_database, target_server_id and target_database are required")
		return
	}

	id, err := h.db.CreateMigration(body.Name, body.SourceServerID, body.TargetServerID,
		body.SourceDatabase, body.TargetDatabase, body.MigrateUsers, body.CleanupSource)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Run migration in background
	go h.runMigration(int(id), body.SourceServerID, body.TargetServerID, body.SourceDatabase, body.TargetDatabase, body.MigrateUsers, body.CleanupSource)

	m, _ := h.db.GetMigration(int(id))
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) runMigration(migID, srcServerID, dstServerID int, srcDB, dstDB string, migrateUsers, cleanupSource bool) {
	now := time.Now()
	_ = h.db.UpdateMigrationStatus(migID, "running", &now, nil)

	logFn := func(msg string) {
		_ = h.db.AppendMigrationLog(migID, msg)
	}

	srcParams, err := h.serverConnParams(srcServerID)
	if err != nil || srcParams == nil {
		logFn(fmt.Sprintf("[ERROR] Source server not found: %v", err))
		t := time.Now()
		_ = h.db.UpdateMigrationStatus(migID, "failed", &now, &t)
		return
	}

	dstParams, err := h.serverConnParams(dstServerID)
	if err != nil || dstParams == nil {
		logFn(fmt.Sprintf("[ERROR] Target server not found: %v", err))
		t := time.Now()
		_ = h.db.UpdateMigrationStatus(migID, "failed", &now, &t)
		return
	}

	// Look up the owner password from managed database records (if known)
	ownerPassword := ""
	if migrateUsers {
		mdb, mdbEnc, err := h.db.GetManagedDatabaseByServerAndName(srcServerID, srcDB)
		if err == nil && mdb != nil && mdbEnc != "" {
			if pw, err := crypto.Decrypt(mdbEnc); err == nil {
				ownerPassword = pw
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	logger := migration.FuncLogger(logFn)
	runErr := migration.Run(ctx, *srcParams, *dstParams, srcDB, dstDB, migrateUsers, ownerPassword, logger)

	completed := time.Now()
	if runErr != nil {
		logFn(fmt.Sprintf("[ERROR] Migration failed: %v", runErr))
		_ = h.db.UpdateMigrationStatus(migID, "failed", &now, &completed)
		return
	}

	_ = h.db.UpdateMigrationStatus(migID, "completed", &now, &completed)

	if cleanupSource {
		logFn("[INFO] Cleaning up source database and user...")
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cleanCancel()
		if err := pg.DropIsolatedSet(cleanCtx, *srcParams, srcDB, srcDB); err != nil {
			logFn(fmt.Sprintf("[WARN] Cleanup failed (migration itself succeeded): %v", err))
		} else {
			logFn("[INFO] Source database and user removed.")
		}
	}
}

func (h *Handlers) DeleteMigration(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	m, err := h.db.GetMigration(id)
	if err != nil || m == nil {
		writeError(w, http.StatusNotFound, "migration not found")
		return
	}
	if m.Status == "running" || m.Status == "pending" {
		writeError(w, http.StatusConflict, "cannot delete a running migration")
		return
	}

	if err := h.db.DeleteMigration(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
