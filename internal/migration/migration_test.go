package migration

import (
	"strings"
	"testing"
)

type testLogger struct{ messages []string }

func (l *testLogger) Log(msg string) { l.messages = append(l.messages, msg) }

func TestLogTimescaleCopyForOrdinaryObjects(t *testing.T) {
	log := &testLogger{}
	logTimescaleCopy(false, true, 0, log)
	if len(log.messages) != 1 || !strings.Contains(log.messages[0], "copying regular schema objects") {
		t.Fatalf("unexpected migration log: %v", log.messages)
	}
}

func TestLogTimescaleCopyFallbackForHypertables(t *testing.T) {
	log := &testLogger{}
	logTimescaleCopy(true, true, 2, log)
	message := strings.Join(log.messages, " ")
	for _, expected := range []string{"Found 2 TimescaleDB hypertables", "regular PostgreSQL tables", "dimensions, policies", "will not be preserved"} {
		if !strings.Contains(message, expected) {
			t.Errorf("migration log %q missing %q", message, expected)
		}
	}
}
