package db

import "time"

type Server struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	AdminUser   string     `json:"admin_user"`
	SSLMode     string     `json:"ssl_mode"`
	Notes       string     `json:"notes"`
	RelayURL    string     `json:"relay_url"`
	RelayID     *int       `json:"relay_id,omitempty"`
	RelayName   string     `json:"relay_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastChecked *time.Time `json:"last_checked"`
	Status      string     `json:"status"`
}

type Relay struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type ManagedDatabase struct {
	ID           int       `json:"id"`
	ServerID     int       `json:"server_id"`
	ServerName   string    `json:"server_name,omitempty"`
	DatabaseName string    `json:"database_name"`
	OwnerUser    string    `json:"owner_user"`
	CreatedAt    time.Time `json:"created_at"`
	Notes        string    `json:"notes"`
	TableCount   *int64    `json:"table_count"`
	SizeBytes    *int64    `json:"size_bytes"`
}

type ManagedDatabaseDetail struct {
	ID            int       `json:"id"`
	ServerID      int       `json:"server_id"`
	ServerName    string    `json:"server_name"`
	ServerHost    string    `json:"server_host"`
	ServerPort    int       `json:"server_port"`
	ServerSSLMode string    `json:"server_ssl_mode"`
	DatabaseName  string    `json:"database_name"`
	OwnerUser     string    `json:"owner_user"`
	OwnerPassword string    `json:"owner_password"`
	CreatedAt     time.Time `json:"created_at"`
	Notes         string    `json:"notes"`
}

type Cluster struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	ReplicationType string    `json:"replication_type"`
	CreatedAt       time.Time `json:"created_at"`
	Members         []ClusterMember `json:"members,omitempty"`
}

type ClusterMember struct {
	ID         int       `json:"id"`
	ClusterID  int       `json:"cluster_id"`
	ServerID   int       `json:"server_id"`
	ServerName string    `json:"server_name,omitempty"`
	ServerHost string    `json:"server_host,omitempty"`
	Role       string    `json:"role"`
	JoinedAt   time.Time `json:"joined_at"`
}

type Migration struct {
	ID               int        `json:"id"`
	Name             string     `json:"name"`
	SourceServerID   *int       `json:"source_server_id"`
	SourceServerName string     `json:"source_server_name,omitempty"`
	SourceDatabase   string     `json:"source_database"`
	TargetServerID   *int       `json:"target_server_id"`
	TargetServerName string     `json:"target_server_name,omitempty"`
	TargetDatabase   string     `json:"target_database"`
	MigrateUsers     bool       `json:"migrate_users"`
	CleanupSource    bool       `json:"cleanup_source"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	Log              string     `json:"log"`
}
