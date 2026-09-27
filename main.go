package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"datacluster/internal/config"
	"datacluster/internal/crypto"
	"datacluster/internal/db"
	"datacluster/internal/handlers"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	cfg := config.Load()

	if err := crypto.Init(cfg.DataDir); err != nil {
		log.Fatalf("crypto init: %v", err)
	}

	database, err := db.Init(cfg.DataDir)
	if err != nil {
		log.Fatalf("database init: %v", err)
	}
	defer database.Close()

	h := handlers.New(database, cfg)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public
	r.Post("/api/auth", h.Login)

	// Protected API
	r.Group(func(r chi.Router) {
		r.Use(h.AuthMiddleware)

		r.Get("/api/status", h.GetStatus)

		r.Get("/api/relays", h.ListRelays)
		r.Post("/api/relays", h.CreateRelay)
		r.Put("/api/relays/{id}", h.UpdateRelay)
		r.Delete("/api/relays/{id}", h.DeleteRelay)

		r.Get("/api/servers", h.ListServers)
		r.Post("/api/servers", h.CreateServer)
		r.Get("/api/servers/{id}", h.GetServer)
		r.Put("/api/servers/{id}", h.UpdateServer)
		r.Delete("/api/servers/{id}", h.DeleteServer)
		r.Post("/api/servers/{id}/test", h.TestServer)
		r.Get("/api/servers/{id}/databases", h.ListServerDatabases)
		r.Get("/api/servers/{id}/info", h.GetServerInfo)

		r.Get("/api/databases", h.ListManagedDatabases)
		r.Post("/api/databases", h.CreateManagedDatabase)
		r.Get("/api/databases/{id}", h.GetManagedDatabaseDetail)
		r.Delete("/api/databases/{id}", h.DeleteManagedDatabase)

		r.Get("/api/migrations", h.ListMigrations)
		r.Post("/api/migrations", h.CreateMigration)
		r.Get("/api/migrations/{id}", h.GetMigration)
		r.Delete("/api/migrations/{id}", h.DeleteMigration)

		r.Get("/api/clusters", h.ListClusters)
		r.Post("/api/clusters", h.CreateCluster)
		r.Get("/api/clusters/{id}", h.GetCluster)
		r.Put("/api/clusters/{id}", h.UpdateCluster)
		r.Delete("/api/clusters/{id}", h.DeleteCluster)
		r.Post("/api/clusters/{id}/members", h.AddClusterMember)
		r.Delete("/api/clusters/{id}/members/{serverId}", h.RemoveClusterMember)

		r.Get("/api/relay", h.Relay)
	})

	// Embedded static files — SPA fallback to index.html
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("static files: %v", err)
	}
	fileServer := http.FileServer(http.FS(staticFS))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fileServer.ServeHTTP(w, req)
	}))

	log.Printf("DataCluster listening on :%s (data: %s)", cfg.Port, cfg.DataDir)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}
