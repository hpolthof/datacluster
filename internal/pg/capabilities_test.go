package pg

import "testing"

func TestCapabilitiesForTimescaleDB(t *testing.T) {
	got := capabilitiesFor("PostgreSQL 16.4 on x86_64 (TimescaleDB 2.16.1)", true)
	if got.Engine != EngineTimescaleDB || !got.TimescaleDB || !got.TimescaleExtension {
		t.Fatalf("unexpected capabilities: %+v", got)
	}
}

func TestCapabilitiesForPostgreSQL(t *testing.T) {
	got := capabilitiesFor("PostgreSQL 16.4 on x86_64-pc-linux-gnu", false)
	if got.Engine != EnginePostgreSQL || got.TimescaleDB || got.TimescaleExtension {
		t.Fatalf("unexpected capabilities: %+v", got)
	}
}

func TestCapabilitiesDistinguishExtensionFromServer(t *testing.T) {
	got := capabilitiesFor("PostgreSQL 16.4", true)
	if got.Engine != EnginePostgreSQL || got.TimescaleDB || !got.TimescaleExtension {
		t.Fatalf("unexpected capabilities: %+v", got)
	}
}
