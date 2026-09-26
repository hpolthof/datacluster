package db

import (
	"database/sql"
	"time"
)

// ─── Servers ─────────────────────────────────────────────────────────────────

func (d *DB) ListServers() ([]Server, error) {
	rows, err := d.Query(`SELECT id,name,host,port,admin_user,ssl_mode,notes,relay_url,created_at,last_checked,status FROM servers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.AdminUser, &s.SSLMode, &s.Notes, &s.RelayURL, &s.CreatedAt, &s.LastChecked, &s.Status); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	if servers == nil {
		servers = []Server{}
	}
	return servers, rows.Err()
}

func (d *DB) GetServer(id int) (*Server, error) {
	var s Server
	err := d.QueryRow(`SELECT id,name,host,port,admin_user,ssl_mode,notes,relay_url,created_at,last_checked,status FROM servers WHERE id=?`, id).
		Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.AdminUser, &s.SSLMode, &s.Notes, &s.RelayURL, &s.CreatedAt, &s.LastChecked, &s.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

// GetServerWithPassword returns the server plus both encrypted password fields.
// Returns: server, adminPasswordEnc, relayPasswordEnc, error.
func (d *DB) GetServerWithPassword(id int) (*Server, string, string, error) {
	var s Server
	var adminEnc, relayEnc string
	err := d.QueryRow(`SELECT id,name,host,port,admin_user,admin_password_enc,ssl_mode,notes,relay_url,relay_password_enc,created_at,last_checked,status FROM servers WHERE id=?`, id).
		Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.AdminUser, &adminEnc, &s.SSLMode, &s.Notes, &s.RelayURL, &relayEnc, &s.CreatedAt, &s.LastChecked, &s.Status)
	if err == sql.ErrNoRows {
		return nil, "", "", nil
	}
	return &s, adminEnc, relayEnc, err
}

func (d *DB) CreateServer(name, host string, port int, adminUser, adminPasswordEnc, sslMode, notes, relayURL, relayPasswordEnc string) (int64, error) {
	res, err := d.Exec(`INSERT INTO servers(name,host,port,admin_user,admin_password_enc,ssl_mode,notes,relay_url,relay_password_enc) VALUES(?,?,?,?,?,?,?,?,?)`,
		name, host, port, adminUser, adminPasswordEnc, sslMode, notes, relayURL, relayPasswordEnc)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateServer(id int, name, host string, port int, adminUser, adminPasswordEnc, sslMode, notes, relayURL, relayPasswordEnc string) error {
	_, err := d.Exec(`UPDATE servers SET name=?,host=?,port=?,admin_user=?,admin_password_enc=?,ssl_mode=?,notes=?,relay_url=?,relay_password_enc=? WHERE id=?`,
		name, host, port, adminUser, adminPasswordEnc, sslMode, notes, relayURL, relayPasswordEnc, id)
	return err
}

func (d *DB) UpdateServerStatus(id int, status string) error {
	_, err := d.Exec(`UPDATE servers SET status=?,last_checked=CURRENT_TIMESTAMP WHERE id=?`, status, id)
	return err
}

func (d *DB) DeleteServer(id int) error {
	_, err := d.Exec(`DELETE FROM servers WHERE id=?`, id)
	return err
}

// ─── Managed Databases ────────────────────────────────────────────────────────

func (d *DB) ListManagedDatabases(serverID *int) ([]ManagedDatabase, error) {
	query := `SELECT md.id,md.server_id,s.name,md.database_name,md.owner_user,md.created_at,md.notes
		FROM managed_databases md JOIN servers s ON s.id=md.server_id`
	args := []any{}
	if serverID != nil {
		query += " WHERE md.server_id=?"
		args = append(args, *serverID)
	}
	query += " ORDER BY s.name,md.database_name"
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ManagedDatabase
	for rows.Next() {
		var m ManagedDatabase
		if err := rows.Scan(&m.ID, &m.ServerID, &m.ServerName, &m.DatabaseName, &m.OwnerUser, &m.CreatedAt, &m.Notes); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	if list == nil {
		list = []ManagedDatabase{}
	}
	return list, rows.Err()
}

func (d *DB) GetManagedDatabase(id int) (*ManagedDatabase, string, error) {
	var m ManagedDatabase
	var enc string
	err := d.QueryRow(`SELECT md.id,md.server_id,s.name,md.database_name,md.owner_user,md.owner_password_enc,md.created_at,md.notes
		FROM managed_databases md JOIN servers s ON s.id=md.server_id WHERE md.id=?`, id).
		Scan(&m.ID, &m.ServerID, &m.ServerName, &m.DatabaseName, &m.OwnerUser, &enc, &m.CreatedAt, &m.Notes)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	return &m, enc, err
}

func (d *DB) CreateManagedDatabase(serverID int, dbName, ownerUser, ownerPasswordEnc, notes string) (int64, error) {
	res, err := d.Exec(`INSERT INTO managed_databases(server_id,database_name,owner_user,owner_password_enc,notes) VALUES(?,?,?,?,?)`,
		serverID, dbName, ownerUser, ownerPasswordEnc, notes)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) DeleteManagedDatabase(id int) error {
	_, err := d.Exec(`DELETE FROM managed_databases WHERE id=?`, id)
	return err
}

// ─── Clusters ────────────────────────────────────────────────────────────────

func (d *DB) ListClusters() ([]Cluster, error) {
	rows, err := d.Query(`SELECT id,name,description,replication_type,created_at FROM clusters ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Cluster
	for rows.Next() {
		var c Cluster
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.ReplicationType, &c.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	if list == nil {
		list = []Cluster{}
	}
	for i := range list {
		list[i].Members, _ = d.getClusterMembers(list[i].ID)
	}
	return list, rows.Err()
}

func (d *DB) GetCluster(id int) (*Cluster, error) {
	var c Cluster
	err := d.QueryRow(`SELECT id,name,description,replication_type,created_at FROM clusters WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Description, &c.ReplicationType, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Members, _ = d.getClusterMembers(c.ID)
	return &c, nil
}

func (d *DB) getClusterMembers(clusterID int) ([]ClusterMember, error) {
	rows, err := d.Query(`SELECT cm.id,cm.cluster_id,cm.server_id,s.name,s.host,cm.role,cm.joined_at
		FROM cluster_members cm JOIN servers s ON s.id=cm.server_id WHERE cm.cluster_id=? ORDER BY cm.role,s.name`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ClusterMember
	for rows.Next() {
		var m ClusterMember
		if err := rows.Scan(&m.ID, &m.ClusterID, &m.ServerID, &m.ServerName, &m.ServerHost, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	if list == nil {
		list = []ClusterMember{}
	}
	return list, rows.Err()
}

func (d *DB) CreateCluster(name, description, replicationType string) (int64, error) {
	res, err := d.Exec(`INSERT INTO clusters(name,description,replication_type) VALUES(?,?,?)`, name, description, replicationType)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateCluster(id int, name, description, replicationType string) error {
	_, err := d.Exec(`UPDATE clusters SET name=?,description=?,replication_type=? WHERE id=?`, name, description, replicationType, id)
	return err
}

func (d *DB) DeleteCluster(id int) error {
	_, err := d.Exec(`DELETE FROM clusters WHERE id=?`, id)
	return err
}

func (d *DB) AddClusterMember(clusterID, serverID int, role string) error {
	_, err := d.Exec(`INSERT INTO cluster_members(cluster_id,server_id,role) VALUES(?,?,?)`, clusterID, serverID, role)
	return err
}

func (d *DB) RemoveClusterMember(clusterID, serverID int) error {
	_, err := d.Exec(`DELETE FROM cluster_members WHERE cluster_id=? AND server_id=?`, clusterID, serverID)
	return err
}

// ─── Migrations ──────────────────────────────────────────────────────────────

func (d *DB) ListMigrations() ([]Migration, error) {
	rows, err := d.Query(`
		SELECT m.id,m.name,m.source_server_id,ss.name,m.source_database,
		       m.target_server_id,ts.name,m.target_database,
		       m.migrate_users,m.cleanup_source,m.status,m.created_at,m.started_at,m.completed_at,m.log
		FROM migrations m
		LEFT JOIN servers ss ON ss.id=m.source_server_id
		LEFT JOIN servers ts ON ts.id=m.target_server_id
		ORDER BY m.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Migration
	for rows.Next() {
		var m Migration
		var mu, cs int
		if err := rows.Scan(&m.ID, &m.Name, &m.SourceServerID, &m.SourceServerName, &m.SourceDatabase,
			&m.TargetServerID, &m.TargetServerName, &m.TargetDatabase,
			&mu, &cs, &m.Status, &m.CreatedAt, &m.StartedAt, &m.CompletedAt, &m.Log); err != nil {
			return nil, err
		}
		m.MigrateUsers = mu == 1
		m.CleanupSource = cs == 1
		list = append(list, m)
	}
	if list == nil {
		list = []Migration{}
	}
	return list, rows.Err()
}

func (d *DB) GetMigration(id int) (*Migration, error) {
	var m Migration
	var mu, cs int
	err := d.QueryRow(`
		SELECT m.id,m.name,m.source_server_id,ss.name,m.source_database,
		       m.target_server_id,ts.name,m.target_database,
		       m.migrate_users,m.cleanup_source,m.status,m.created_at,m.started_at,m.completed_at,m.log
		FROM migrations m
		LEFT JOIN servers ss ON ss.id=m.source_server_id
		LEFT JOIN servers ts ON ts.id=m.target_server_id
		WHERE m.id=?`, id).
		Scan(&m.ID, &m.Name, &m.SourceServerID, &m.SourceServerName, &m.SourceDatabase,
			&m.TargetServerID, &m.TargetServerName, &m.TargetDatabase,
			&mu, &cs, &m.Status, &m.CreatedAt, &m.StartedAt, &m.CompletedAt, &m.Log)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.MigrateUsers = mu == 1
	m.CleanupSource = cs == 1
	return &m, nil
}

func (d *DB) CreateMigration(name string, srcServerID, dstServerID int, srcDB, dstDB string, migrateUsers, cleanupSource bool) (int64, error) {
	mu, cs := 0, 0
	if migrateUsers {
		mu = 1
	}
	if cleanupSource {
		cs = 1
	}
	res, err := d.Exec(`INSERT INTO migrations(name,source_server_id,source_database,target_server_id,target_database,migrate_users,cleanup_source) VALUES(?,?,?,?,?,?,?)`,
		name, srcServerID, srcDB, dstServerID, dstDB, mu, cs)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateMigrationStatus(id int, status string, startedAt, completedAt *time.Time) error {
	_, err := d.Exec(`UPDATE migrations SET status=?,started_at=?,completed_at=? WHERE id=?`, status, startedAt, completedAt, id)
	return err
}

func (d *DB) AppendMigrationLog(id int, line string) error {
	_, err := d.Exec(`UPDATE migrations SET log=log||? WHERE id=?`, line+"\n", id)
	return err
}

func (d *DB) DeleteMigration(id int) error {
	_, err := d.Exec(`DELETE FROM migrations WHERE id=?`, id)
	return err
}

// ─── Dashboard ───────────────────────────────────────────────────────────────

type Stats struct {
	Servers    int `json:"servers"`
	Databases  int `json:"databases"`
	Clusters   int `json:"clusters"`
	Migrations int `json:"migrations_active"`
}

func (d *DB) GetDashboardStats() Stats {
	var s Stats
	_ = d.QueryRow(`SELECT COUNT(*) FROM servers`).Scan(&s.Servers)
	_ = d.QueryRow(`SELECT COUNT(*) FROM managed_databases`).Scan(&s.Databases)
	_ = d.QueryRow(`SELECT COUNT(*) FROM clusters`).Scan(&s.Clusters)
	_ = d.QueryRow(`SELECT COUNT(*) FROM migrations WHERE status IN ('pending','running')`).Scan(&s.Migrations)
	return s
}
