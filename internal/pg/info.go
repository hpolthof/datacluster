package pg

import (
	"context"
	"time"
)

type DatabaseInfo struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	Owner       string `json:"owner"`
	Connections int    `json:"connections"`
	Collation   string `json:"collation"`
}

type RoleInfo struct {
	Name        string  `json:"name"`
	Superuser   bool    `json:"superuser"`
	CanLogin    bool    `json:"can_login"`
	CreateDB    bool    `json:"create_db"`
	CreateRole  bool    `json:"create_role"`
	Replication bool    `json:"replication"`
	ConnLimit   int     `json:"conn_limit"`
	ValidUntil  *string `json:"valid_until,omitempty"`
}

type SettingInfo struct {
	Name    string `json:"name"`
	Setting string `json:"setting"`
	Unit    string `json:"unit"`
	Desc    string `json:"desc"`
}

type ServerInfo struct {
	Version           string         `json:"version"`
	UptimeSeconds     float64        `json:"uptime_seconds"`
	ConnectionsActive int            `json:"connections_active"`
	ConnectionsMax    int            `json:"connections_max"`
	Databases         []DatabaseInfo `json:"databases"`
	Roles             []RoleInfo     `json:"roles"`
	Settings          []SettingInfo  `json:"settings"`
}

func GetServerInfo(ctx context.Context, p ConnParams) (*ServerInfo, error) {
	conn, err := Connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	info := &ServerInfo{
		Databases: []DatabaseInfo{},
		Roles:     []RoleInfo{},
		Settings:  []SettingInfo{},
	}

	// Version, uptime, connections
	var startTime time.Time
	_ = conn.QueryRow(ctx, `SELECT version(), pg_postmaster_start_time(),
		(SELECT count(*) FROM pg_stat_activity WHERE state IS NOT NULL),
		(SELECT setting::int FROM pg_settings WHERE name='max_connections')`).
		Scan(&info.Version, &startTime, &info.ConnectionsActive, &info.ConnectionsMax)
	info.UptimeSeconds = time.Since(startTime).Seconds()

	// Databases with sizes
	rows, err := conn.Query(ctx, `
		SELECT d.datname,
		       pg_database_size(d.datname),
		       r.rolname,
		       d.datcollate,
		       (SELECT count(*) FROM pg_stat_activity WHERE datname=d.datname)
		FROM pg_database d
		JOIN pg_roles r ON r.oid=d.datdba
		WHERE d.datistemplate=false
		ORDER BY pg_database_size(d.datname) DESC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var db DatabaseInfo
			if err := rows.Scan(&db.Name, &db.SizeBytes, &db.Owner, &db.Collation, &db.Connections); err == nil {
				info.Databases = append(info.Databases, db)
			}
		}
	}

	// Roles (skip pg_* internal roles)
	rows2, err := conn.Query(ctx, `
		SELECT rolname, rolsuper, rolcanlogin, rolcreatedb, rolcreaterole, rolreplication,
		       rolconnlimit,
		       CASE WHEN rolvaliduntil IS NOT NULL THEN rolvaliduntil::text ELSE NULL END
		FROM pg_roles
		WHERE rolname NOT LIKE 'pg_%'
		ORDER BY rolcanlogin DESC, rolname`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var r RoleInfo
			if err := rows2.Scan(&r.Name, &r.Superuser, &r.CanLogin, &r.CreateDB, &r.CreateRole,
				&r.Replication, &r.ConnLimit, &r.ValidUntil); err == nil {
				info.Roles = append(info.Roles, r)
			}
		}
	}

	// Key settings
	rows3, err := conn.Query(ctx, `
		SELECT name, setting, COALESCE(unit,''), short_desc
		FROM pg_settings
		WHERE name IN (
			'max_connections','shared_buffers','effective_cache_size','work_mem',
			'maintenance_work_mem','wal_level','max_wal_senders','log_min_duration_statement',
			'autovacuum','track_counts','default_transaction_isolation','TimeZone'
		)
		ORDER BY name`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var s SettingInfo
			if err := rows3.Scan(&s.Name, &s.Setting, &s.Unit, &s.Desc); err == nil {
				info.Settings = append(info.Settings, s)
			}
		}
	}

	return info, nil
}
