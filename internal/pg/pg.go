package pg

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"datacluster/internal/relay"
)

// passthroughLookup returns the hostname as-is so pgconn skips local DNS
// resolution. The relay DialFunc handles the actual network address.
func passthroughLookup(_ context.Context, host string) ([]string, error) {
	return []string{host}, nil
}

type ConnParams struct {
	Host       string
	Port       int
	User       string
	Password   string
	Database   string
	SSLMode    string
	RelayURL   string // optional: base URL of relay DataCluster instance
	RelayToken string // optional: Bearer token for the relay instance
}

func (p ConnParams) DSN(database string) string {
	if database == "" {
		database = "postgres"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, database, p.SSLMode)
}

func Connect(ctx context.Context, p ConnParams, database string) (*pgx.Conn, error) {
	config, err := pgx.ParseConfig(p.DSN(database))
	if err != nil {
		return nil, err
	}
	if p.RelayURL != "" && p.RelayToken != "" {
		config.Config.DialFunc = relay.DialFunc(p.RelayURL, p.RelayToken)
		config.Config.LookupFunc = passthroughLookup
	}
	return pgx.ConnectConfig(ctx, config)
}

// TestConnection checks if we can connect and returns the server version.
func TestConnection(ctx context.Context, p ConnParams) (string, error) {
	conn, err := Connect(ctx, p, "postgres")
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

// ListDatabases returns all non-template databases on the server.
func ListDatabases(ctx context.Context, p ConnParams) ([]string, error) {
	conn, err := Connect(ctx, p, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `SELECT datname FROM pg_database WHERE datistemplate=false ORDER BY datname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		dbs = append(dbs, name)
	}
	return dbs, rows.Err()
}

var safeIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func validateIdentifier(name string) error {
	if !safeIdentifier.MatchString(name) {
		return fmt.Errorf("invalid identifier %q: only letters, digits and underscores allowed", name)
	}
	return nil
}

// CreateIsolatedSet creates a database and a matching owner user with exclusive access.
func CreateIsolatedSet(ctx context.Context, p ConnParams, dbName, ownerUser, ownerPassword string) error {
	if err := validateIdentifier(dbName); err != nil {
		return err
	}
	if err := validateIdentifier(ownerUser); err != nil {
		return err
	}

	conn, err := Connect(ctx, p, "postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	// Create role if not exists
	var exists bool
	_ = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, ownerUser).Scan(&exists)
	if !exists {
		_, err = conn.Exec(ctx, fmt.Sprintf(`CREATE ROLE %s WITH LOGIN PASSWORD '%s'`, ownerUser, escapeSQLString(ownerPassword)))
		if err != nil {
			return fmt.Errorf("create role: %w", err)
		}
	}

	// Create database owned by the user
	_, err = conn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s OWNER %s`, dbName, ownerUser))
	if err != nil {
		return fmt.Errorf("create database: %w", err)
	}

	// Connect to new database and restrict public schema
	dbConn, err := Connect(ctx, p, dbName)
	if err != nil {
		return fmt.Errorf("connect to new db: %w", err)
	}
	defer dbConn.Close(ctx)

	sqls := []string{
		fmt.Sprintf(`REVOKE ALL ON DATABASE %s FROM PUBLIC`, dbName),
		fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE %s TO %s`, dbName, ownerUser),
		`REVOKE ALL ON SCHEMA public FROM PUBLIC`,
		fmt.Sprintf(`GRANT ALL ON SCHEMA public TO %s`, ownerUser),
	}
	for _, q := range sqls {
		if _, err := dbConn.Exec(ctx, q); err != nil {
			return fmt.Errorf("grant: %w", err)
		}
	}
	return nil
}

// DropIsolatedSet drops database and owner user.
func DropIsolatedSet(ctx context.Context, p ConnParams, dbName, ownerUser string) error {
	if err := validateIdentifier(dbName); err != nil {
		return err
	}
	if err := validateIdentifier(ownerUser); err != nil {
		return err
	}

	conn, err := Connect(ctx, p, "postgres")
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	// Terminate existing connections first
	_, _ = conn.Exec(ctx, fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='%s' AND pid<>pg_backend_pid()`,
		escapeSQLString(dbName)))

	if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, dbName)); err != nil {
		return fmt.Errorf("drop database: %w", err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, ownerUser)); err != nil {
		return fmt.Errorf("drop role: %w", err)
	}
	return nil
}

// escapeSQLString escapes single quotes for use in SQL string literals.
// Only used internally for identifiers/passwords in DDL where parameterized queries aren't available.
func escapeSQLString(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			result = append(result, '\'', '\'')
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}
