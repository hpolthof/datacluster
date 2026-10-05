package pg

import (
	"strings"
	"testing"
)

func TestTableCountQueryIncludesAllSchemasAndPartitionedTables(t *testing.T) {
	query := strings.ToLower(tableCountQuery)
	if !strings.Contains(query, "from pg_class") {
		t.Fatalf("table count must query PostgreSQL's catalog: %q", tableCountQuery)
	}
	if !strings.Contains(query, "relkind in ('r','p')") {
		t.Fatalf("table count must include ordinary and partitioned tables: %q", tableCountQuery)
	}
	if strings.Contains(query, "schemaname") || strings.Contains(query, "nspname") || strings.Contains(query, "pg_namespace") {
		t.Fatalf("table count must not filter schemas: %q", tableCountQuery)
	}
}
