package db

import "time"

type Server struct {
	ID               int        `json:"id"`
	Name             string     `json:"name"`
	Host             string     `json:"host"`
	Port             int        `json:"port"`
	AdminUser        string     `json:"admin_user"`
	SSLMode          string     `json:"ssl_mode"`
	Notes            string     `json:"notes"`
	CreatedAt        time.Time  `json:"created_at"`
	LastChecked      *time.Time `json:"last_checked"`
	Status           string     `json:"status"`
}

type ManagedDatabase struct {
	ID           int       `json:"id"`
	ServerID     int       `json:"server_id"`
	ServerName   string    `json:"server_name,omitempty"`
	DatabaseName string    `json:"database_name"`
	OwnerUser    string    `json:"owner_user"`
	CreatedAt    time.Time `json:"created_at"`
	Notes        string    `json:"notes"`
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
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	Log              string     `json:"log"`
}
