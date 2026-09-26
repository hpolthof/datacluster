package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Init(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "datacluster.db")
	sqldb, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1) // sqlite is single-writer

	d := &DB{sqldb}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.Exec(`
CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    host TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 5432,
    admin_user TEXT NOT NULL,
    admin_password_enc TEXT NOT NULL,
    ssl_mode TEXT NOT NULL DEFAULT 'prefer',
    notes TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_checked DATETIME,
    status TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS managed_databases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    database_name TEXT NOT NULL,
    owner_user TEXT NOT NULL,
    owner_password_enc TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notes TEXT NOT NULL DEFAULT '',
    UNIQUE(server_id, database_name)
);

CREATE TABLE IF NOT EXISTS clusters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    replication_type TEXT NOT NULL DEFAULT 'logical',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cluster_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cluster_id INTEGER NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'replica',
    joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(cluster_id, server_id)
);

CREATE TABLE IF NOT EXISTS relays (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS migrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL DEFAULT '',
    source_server_id INTEGER REFERENCES servers(id) ON DELETE SET NULL,
    source_database TEXT NOT NULL,
    target_server_id INTEGER REFERENCES servers(id) ON DELETE SET NULL,
    target_database TEXT NOT NULL,
    migrate_users INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME,
    log TEXT NOT NULL DEFAULT ''
);
`)
	if err != nil {
		return err
	}
	// Additive column migrations — SQLite has no IF NOT EXISTS for columns, ignore duplicate errors.
	d.Exec(`ALTER TABLE servers ADD COLUMN relay_url TEXT NOT NULL DEFAULT ''`)
	d.Exec(`ALTER TABLE servers ADD COLUMN relay_password_enc TEXT NOT NULL DEFAULT ''`)
	d.Exec(`ALTER TABLE servers ADD COLUMN relay_id INTEGER REFERENCES relays(id) ON DELETE SET NULL`)
	d.Exec(`ALTER TABLE migrations ADD COLUMN cleanup_source INTEGER NOT NULL DEFAULT 0`)
	return nil
}
