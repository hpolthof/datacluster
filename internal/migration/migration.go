package migration

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"datacluster/internal/pg"
)

type Logger interface {
	Log(msg string)
}

type funcLogger struct {
	fn func(string)
}

func (f *funcLogger) Log(msg string) { f.fn(msg) }

func FuncLogger(fn func(string)) Logger {
	return &funcLogger{fn: fn}
}

// Run performs a full database migration from source to target using binary COPY protocol.
// ownerPassword is the known password for the source database owner; pass empty string if unknown.
func Run(ctx context.Context, src, dst pg.ConnParams, srcDB, dstDB string, migrateUsers bool, ownerPassword string, log Logger) error {
	log.Log(fmt.Sprintf("[%s] Starting migration: %s/%s → %s/%s", ts(), src.Host, srcDB, dst.Host, dstDB))

	srcConn, err := pg.Connect(ctx, src, srcDB)
	if err != nil {
		return fmt.Errorf("connect source: %w", err)
	}
	defer srcConn.Close(ctx)

	// Ensure target database exists; create it if not.
	created, err := ensureTargetDatabase(ctx, dst, dstDB)
	if err != nil {
		return fmt.Errorf("ensure target database: %w", err)
	}
	if created {
		log.Log(fmt.Sprintf("[%s] Target database %q created", ts(), dstDB))
	}

	dstConn, err := pg.Connect(ctx, dst, dstDB)
	if err != nil {
		return fmt.Errorf("connect target: %w", err)
	}
	defer dstConn.Close(ctx)

	log.Log(fmt.Sprintf("[%s] Connected to source and target", ts()))

	// TimescaleDB hypertables are copied as ordinary PostgreSQL tables. The
	// migration does not recreate extension-specific dimensions or policies.
	if err := logHypertableFallback(ctx, srcConn, dstConn, log); err != nil {
		return fmt.Errorf("inspect TimescaleDB objects: %w", err)
	}

	// 1. Copy schema
	log.Log(fmt.Sprintf("[%s] Copying schema...", ts()))
	if err := copySchema(ctx, srcConn, dstConn, log); err != nil {
		return fmt.Errorf("copy schema: %w", err)
	}

	// 2. Disable FK checks on target temporarily
	_, _ = dstConn.Exec(ctx, `SET session_replication_role = 'replica'`)

	// 3. Copy each table
	tables, err := listTables(ctx, srcConn)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	log.Log(fmt.Sprintf("[%s] Found %d tables to migrate", ts(), len(tables)))

	total := int64(0)
	for _, table := range tables {
		n, err := copyTable(ctx, srcConn, dstConn, table)
		if err != nil {
			log.Log(fmt.Sprintf("[%s] ERROR copying table %s: %v", ts(), table, err))
			return fmt.Errorf("copy table %s: %w", table, err)
		}
		log.Log(fmt.Sprintf("[%s] Table %-40s %d rows", ts(), table, n))
		total += n
	}

	// 4. Re-enable FK
	_, _ = dstConn.Exec(ctx, `SET session_replication_role = 'origin'`)

	// 5. Sync sequences
	log.Log(fmt.Sprintf("[%s] Syncing sequences...", ts()))
	if err := syncSequences(ctx, srcConn, dstConn); err != nil {
		log.Log(fmt.Sprintf("[%s] WARNING: sequence sync failed: %v", ts(), err))
	}

	// 6. Migrate database owner role
	if migrateUsers {
		if err := migrateDBOwner(ctx, src, dst, srcDB, dstDB, ownerPassword, log); err != nil {
			log.Log(fmt.Sprintf("[%s] [WARN] User migration failed: %v", ts(), err))
		}
	}

	log.Log(fmt.Sprintf("[%s] Migration complete — %d total rows", ts(), total))
	return nil
}

// logHypertableFallback surfaces the fidelity limitation without making
// TimescaleDB availability a requirement for ordinary PostgreSQL migrations.
func logHypertableFallback(ctx context.Context, src, dst *pgx.Conn, log Logger) error {
	var sourceTimescale, targetTimescale bool
	if err := src.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`).Scan(&sourceTimescale); err != nil {
		return err
	}
	if err := dst.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`).Scan(&targetTimescale); err != nil {
		return err
	}
	if !sourceTimescale && !targetTimescale {
		return nil
	}
	var hypertables int
	if sourceTimescale {
		if err := src.QueryRow(ctx, `SELECT count(*) FROM timescaledb_information.hypertables WHERE hypertable_schema = 'public'`).Scan(&hypertables); err != nil {
			log.Log(fmt.Sprintf("[%s] [WARN] Could not inspect hypertables; schema will be copied as regular PostgreSQL objects", ts()))
			return nil
		}
	}
	logTimescaleCopy(sourceTimescale, targetTimescale, hypertables, log)
	return nil
}

func logTimescaleCopy(sourceTimescale, targetTimescale bool, hypertables int, log Logger) {
	if hypertables > 0 {
		log.Log(fmt.Sprintf("[%s] [WARN] Found %d TimescaleDB hypertables; copying them as regular PostgreSQL tables. Hypertable dimensions, policies, and extension-specific behavior will not be preserved", ts(), hypertables))
	} else {
		log.Log(fmt.Sprintf("[%s] TimescaleDB detected (source=%t target=%t); copying regular schema objects", ts(), sourceTimescale, targetTimescale))
	}
}

func listTables(ctx context.Context, conn *pgx.Conn) ([]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname='public'
		ORDER BY tablename`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// copyTable streams a table from src to dst using binary COPY protocol.
// This is the most efficient method — no intermediate serialization.
func copyTable(ctx context.Context, src, dst *pgx.Conn, table string) (int64, error) {
	pr, pw := io.Pipe()

	copySQL := fmt.Sprintf(`COPY public.%s TO STDOUT (FORMAT BINARY)`, pgx.Identifier{table}.Sanitize())

	errCh := make(chan error, 1)
	go func() {
		_, err := src.PgConn().CopyTo(ctx, pw, copySQL)
		pw.CloseWithError(err)
		errCh <- err
	}()

	result, err := dst.PgConn().CopyFrom(ctx, pr,
		fmt.Sprintf(`COPY public.%s FROM STDIN (FORMAT BINARY)`, pgx.Identifier{table}.Sanitize()))
	pr.Close()

	srcErr := <-errCh
	if srcErr != nil {
		return 0, srcErr
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// copySchema recreates the public schema DDL on the target.
func copySchema(ctx context.Context, src, dst *pgx.Conn, log Logger) error {
	// Create enum / domain types first (tables may reference them)
	if err := copyEnumTypes(ctx, src, dst); err != nil {
		return fmt.Errorf("enum types: %w", err)
	}
	// Create sequences
	if err := copySequences(ctx, src, dst); err != nil {
		return fmt.Errorf("sequences: %w", err)
	}
	// Create tables (without FK constraints)
	if err := copyTables(ctx, src, dst); err != nil {
		return fmt.Errorf("tables: %w", err)
	}
	// Create indexes
	if err := copyIndexes(ctx, src, dst, log); err != nil {
		return fmt.Errorf("indexes: %w", err)
	}
	// Apply FK constraints
	if err := copyForeignKeys(ctx, src, dst, log); err != nil {
		return fmt.Errorf("foreign keys: %w", err)
	}
	return nil
}

func copyEnumTypes(ctx context.Context, src, dst *pgx.Conn) error {
	rows, err := src.Query(ctx, `
		SELECT t.typname, array_agg(e.enumlabel ORDER BY e.enumsortorder)
		FROM pg_type t
		JOIN pg_enum e ON e.enumtypid = t.oid
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE n.nspname = 'public'
		GROUP BY t.typname
		ORDER BY t.typname`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var typeName string
		var labels []string
		if err := rows.Scan(&typeName, &labels); err != nil {
			return err
		}
		quoted := make([]string, len(labels))
		for i, l := range labels {
			quoted[i] = "'" + strings.ReplaceAll(l, "'", "''") + "'"
		}
		ddl := fmt.Sprintf(`CREATE TYPE public.%s AS ENUM (%s)`, quoteIdent(typeName), strings.Join(quoted, ", "))
		if _, err := dst.Exec(ctx, ddl); err != nil && !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("create enum %s: %w", typeName, err)
		}
	}
	return rows.Err()
}

func copySequences(ctx context.Context, src, dst *pgx.Conn) error {
	rows, err := src.Query(ctx, `
		SELECT sequence_name, data_type, start_value, minimum_value, maximum_value, increment, cycle_option
		FROM information_schema.sequences WHERE sequence_schema='public'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, dataType, cycle string
		var start, min, max, inc int64
		if err := rows.Scan(&name, &dataType, &start, &min, &max, &inc, &cycle); err != nil {
			return err
		}
		cycleStr := "NO CYCLE"
		if cycle == "YES" {
			cycleStr = "CYCLE"
		}
		ddl := fmt.Sprintf(
			`CREATE SEQUENCE IF NOT EXISTS public.%s AS %s START %d MINVALUE %d MAXVALUE %d INCREMENT %d %s`,
			name, dataType, start, min, max, inc, cycleStr)
		if _, err := dst.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("create sequence %s: %w", name, err)
		}
	}
	return rows.Err()
}

func copyTables(ctx context.Context, src, dst *pgx.Conn) error {
	tables, err := listTables(ctx, src)
	if err != nil {
		return err
	}
	for _, table := range tables {
		ddl, err := buildCreateTable(ctx, src, table)
		if err != nil {
			return fmt.Errorf("build DDL for %s: %w", table, err)
		}
		if _, err := dst.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("create table %s: %w", table, err)
		}
	}
	return nil
}

func buildCreateTable(ctx context.Context, conn *pgx.Conn, table string) (string, error) {
	rows, err := conn.Query(ctx, `
		SELECT column_name, data_type, character_maximum_length, is_nullable,
		       column_default, numeric_precision, numeric_scale, udt_name, udt_schema
		FROM information_schema.columns
		WHERE table_schema='public' AND table_name=$1
		ORDER BY ordinal_position`, table)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var colName, dataType, isNullable, udtName, udtSchema string
		var charLen, numPrec, numScale *int
		var colDefault *string
		if err := rows.Scan(&colName, &dataType, &charLen, &isNullable, &colDefault, &numPrec, &numScale, &udtName, &udtSchema); err != nil {
			return "", err
		}

		typeDef := dataType
		switch dataType {
		case "USER-DEFINED":
			// Enum or custom type — use the actual type name
			if udtSchema == "public" || udtSchema == "" {
				typeDef = quoteIdent(udtName)
			} else {
				typeDef = quoteIdent(udtSchema) + "." + quoteIdent(udtName)
			}
		case "ARRAY":
			// Array of a base or user-defined type; udt_name is "_typename"
			base := strings.TrimPrefix(udtName, "_")
			typeDef = base + "[]"
		case "character varying":
			if charLen != nil {
				typeDef = fmt.Sprintf("varchar(%d)", *charLen)
			} else {
				typeDef = "text"
			}
		case "character":
			if charLen != nil {
				typeDef = fmt.Sprintf("char(%d)", *charLen)
			}
		case "numeric", "decimal":
			if numPrec != nil && numScale != nil {
				typeDef = fmt.Sprintf("numeric(%d,%d)", *numPrec, *numScale)
			}
		}

		col := fmt.Sprintf("%s %s", quoteIdent(colName), typeDef)
		if isNullable == "NO" {
			col += " NOT NULL"
		}
		if colDefault != nil {
			col += " DEFAULT " + *colDefault
		}
		cols = append(cols, col)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	// Add primary key
	pkCols, err := getPrimaryKey(ctx, conn, table)
	if err == nil && len(pkCols) > 0 {
		quoted := make([]string, len(pkCols))
		for i, c := range pkCols {
			quoted[i] = quoteIdent(c)
		}
		cols = append(cols, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(quoted, ", ")))
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS public.%s (\n  %s\n)",
		quoteIdent(table), strings.Join(cols, ",\n  ")), nil
}

func getPrimaryKey(ctx context.Context, conn *pgx.Conn, table string) ([]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu ON tc.constraint_name=kcu.constraint_name
			AND tc.table_schema=kcu.table_schema
		WHERE tc.constraint_type='PRIMARY KEY' AND tc.table_schema='public' AND tc.table_name=$1
		ORDER BY kcu.ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func copyIndexes(ctx context.Context, src, dst *pgx.Conn, log Logger) error {
	rows, err := src.Query(ctx, `
		SELECT indexname, indexdef FROM pg_indexes
		WHERE schemaname='public' AND indexname NOT IN (
			SELECT constraint_name FROM information_schema.table_constraints
			WHERE table_schema='public'
		)
		ORDER BY indexname`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return err
		}
		if _, err := dst.Exec(ctx, def); err != nil {
			log.Log(fmt.Sprintf("[%s] WARNING: index %s: %v", ts(), name, err))
		}
	}
	return rows.Err()
}

func copyForeignKeys(ctx context.Context, src, dst *pgx.Conn, log Logger) error {
	rows, err := src.Query(ctx, `
		SELECT tc.constraint_name,
		       kcu.table_name, kcu.column_name,
		       ccu.table_name AS ref_table, ccu.column_name AS ref_column,
		       rc.update_rule, rc.delete_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu ON tc.constraint_name=kcu.constraint_name AND tc.table_schema=kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu ON ccu.constraint_name=tc.constraint_name AND ccu.table_schema=tc.table_schema
		JOIN information_schema.referential_constraints rc ON rc.constraint_name=tc.constraint_name AND rc.constraint_schema=tc.table_schema
		WHERE tc.constraint_type='FOREIGN KEY' AND tc.table_schema='public'
		ORDER BY tc.constraint_name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type fk struct {
		name, table, col, refTable, refCol, onUpdate, onDelete string
	}
	fks := make(map[string]*fk)
	var order []string
	for rows.Next() {
		var f fk
		if err := rows.Scan(&f.name, &f.table, &f.col, &f.refTable, &f.refCol, &f.onUpdate, &f.onDelete); err != nil {
			return err
		}
		if _, ok := fks[f.name]; !ok {
			fks[f.name] = &f
			order = append(order, f.name)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range order {
		f := fks[name]
		ddl := fmt.Sprintf(
			`ALTER TABLE public.%s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES public.%s(%s) ON UPDATE %s ON DELETE %s`,
			quoteIdent(f.table), quoteIdent(name), quoteIdent(f.col),
			quoteIdent(f.refTable), quoteIdent(f.refCol), f.onUpdate, f.onDelete)
		if _, err := dst.Exec(ctx, ddl); err != nil {
			log.Log(fmt.Sprintf("[%s] WARNING: FK %s: %v", ts(), name, err))
		}
	}
	return nil
}

func syncSequences(ctx context.Context, src, dst *pgx.Conn) error {
	rows, err := src.Query(ctx, `SELECT sequence_name FROM information_schema.sequences WHERE sequence_schema='public'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var seqs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		seqs = append(seqs, s)
	}
	rows.Close()

	for _, seq := range seqs {
		var lastVal int64
		var isCalled bool
		if err := src.QueryRow(ctx, fmt.Sprintf(`SELECT last_value, is_called FROM public.%s`, seq)).
			Scan(&lastVal, &isCalled); err != nil {
			continue
		}
		if isCalled {
			_, _ = dst.Exec(ctx, fmt.Sprintf(`SELECT setval('public.%s', %d, true)`, seq, lastVal))
		}
	}
	return nil
}

// migrateDBOwner copies the owner role of srcDB to the target server.
// If knownPassword is empty a random one is generated and logged.
func migrateDBOwner(ctx context.Context, src, dst pg.ConnParams, srcDB, dstDB, knownPassword string, log Logger) error {
	// Get the owner role name from the source
	srcConn, err := pg.Connect(ctx, src, "postgres")
	if err != nil {
		return err
	}
	defer srcConn.Close(ctx)

	var ownerName string
	if err := srcConn.QueryRow(ctx,
		`SELECT pg_catalog.pg_get_userbyid(datdba) FROM pg_database WHERE datname=$1`, srcDB,
	).Scan(&ownerName); err != nil || ownerName == "" || ownerName == "postgres" {
		log.Log(fmt.Sprintf("[%s] No dedicated owner role on source — skipping user migration", ts()))
		return nil
	}

	password := knownPassword
	generated := password == ""
	if generated {
		password = generatePassword(24)
	}

	// Create or update the role on the target
	dstConn, err := pg.Connect(ctx, dst, "postgres")
	if err != nil {
		return err
	}
	defer dstConn.Close(ctx)

	var exists bool
	_ = dstConn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, ownerName).Scan(&exists)

	escapedPw := strings.ReplaceAll(password, "'", "''")
	if exists {
		if generated {
			if _, err := dstConn.Exec(ctx, fmt.Sprintf(`ALTER ROLE %s WITH LOGIN PASSWORD '%s'`, quoteIdent(ownerName), escapedPw)); err != nil {
				return fmt.Errorf("alter role: %w", err)
			}
		}
		log.Log(fmt.Sprintf("[%s] Role %q already exists on target", ts(), ownerName))
	} else {
		if _, err := dstConn.Exec(ctx, fmt.Sprintf(`CREATE ROLE %s WITH LOGIN PASSWORD '%s'`, quoteIdent(ownerName), escapedPw)); err != nil {
			return fmt.Errorf("create role: %w", err)
		}
		log.Log(fmt.Sprintf("[%s] Role %q created on target", ts(), ownerName))
	}

	// Grant role access to the target database and schema
	if _, err := dstConn.Exec(ctx, fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE %s TO %s`, quoteIdent(dstDB), quoteIdent(ownerName))); err != nil {
		log.Log(fmt.Sprintf("[%s] [WARN] grant database: %v", ts(), err))
	}

	dstDbConn, err := pg.Connect(ctx, dst, dstDB)
	if err != nil {
		return err
	}
	defer dstDbConn.Close(ctx)

	for _, q := range []string{
		fmt.Sprintf(`GRANT ALL ON SCHEMA public TO %s`, quoteIdent(ownerName)),
		fmt.Sprintf(`GRANT ALL ON ALL TABLES IN SCHEMA public TO %s`, quoteIdent(ownerName)),
		fmt.Sprintf(`GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO %s`, quoteIdent(ownerName)),
	} {
		if _, err := dstDbConn.Exec(ctx, q); err != nil {
			log.Log(fmt.Sprintf("[%s] [WARN] grant: %v", ts(), err))
		}
	}

	if generated {
		log.Log(fmt.Sprintf("[%s] ╔══════════════════════════════════════════════════════╗", ts()))
		log.Log(fmt.Sprintf("[%s] ║  GENERATED PASSWORD for role %q", ts(), ownerName))
		log.Log(fmt.Sprintf("[%s] ║  Password: %s", ts(), password))
		log.Log(fmt.Sprintf("[%s] ║  Save this — it will not be shown again.", ts()))
		log.Log(fmt.Sprintf("[%s] ╚══════════════════════════════════════════════════════╝", ts()))
	} else {
		log.Log(fmt.Sprintf("[%s] Role %q migrated with original password", ts(), ownerName))
	}
	return nil
}

func generatePassword(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

// ensureTargetDatabase connects to the target's postgres database and creates
// dstDB if it does not exist. Returns true if the database was created.
func ensureTargetDatabase(ctx context.Context, dst pg.ConnParams, dstDB string) (bool, error) {
	conn, err := pg.Connect(ctx, dst, "postgres")
	if err != nil {
		return false, err
	}
	defer conn.Close(ctx)

	var exists bool
	err = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, dstDB).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s`, quoteIdent(dstDB))); err != nil {
		return false, fmt.Errorf("create database %q: %w", dstDB, err)
	}
	return true, nil
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func ts() string {
	return time.Now().Format("15:04:05")
}
