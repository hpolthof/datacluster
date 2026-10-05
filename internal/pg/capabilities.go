package pg

import (
	"context"
	"strings"
)

// Engine identifies the database server implementation.
type Engine string

const (
	EnginePostgreSQL Engine = "postgresql"
	EngineTimescaleDB Engine = "timescaledb"
)

// Capabilities describes server identity and TimescaleDB availability in the
// database used for detection. An installed extension does not by itself imply
// that the server is TimescaleDB.
type Capabilities struct {
	Engine             Engine `json:"engine"`
	TimescaleDB        bool   `json:"timescaledb"`
	TimescaleExtension bool   `json:"timescale_extension"`
}

// DetectCapabilities inspects server identity and whether the TimescaleDB
// extension is installed in the connected database. It performs read-only
// queries and does not require the extension to be installed.
func DetectCapabilities(ctx context.Context, p ConnParams) (Capabilities, error) {
	conn, err := Connect(ctx, p, "postgres")
	if err != nil {
		return Capabilities{}, err
	}
	defer conn.Close(ctx)

	var version string
	var extensionInstalled bool
	if err := conn.QueryRow(ctx, `SELECT version(), EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`).Scan(&version, &extensionInstalled); err != nil {
		return Capabilities{}, err
	}
	return capabilitiesFor(version, extensionInstalled), nil
}

func capabilitiesFor(version string, extensionInstalled bool) Capabilities {
	engine := EnginePostgreSQL
	if strings.Contains(strings.ToLower(version), "timescaledb") {
		engine = EngineTimescaleDB
	}
	return Capabilities{
		Engine:             engine,
		TimescaleDB:        engine == EngineTimescaleDB,
		TimescaleExtension: extensionInstalled,
	}
}
