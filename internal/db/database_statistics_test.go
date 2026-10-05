package db

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestManagedDatabaseStatisticsAndMissingValues(t *testing.T) {
	database, err := Init(filepath.Join(t.TempDir(), "data"))
	if err != nil { t.Fatal(err) }
	defer database.Close()
	if _, err := database.Exec(`INSERT INTO servers(name,host,admin_user,admin_password_enc) VALUES('server','localhost','admin','')`); err != nil { t.Fatal(err) }
	if _, err := database.Exec(`INSERT INTO managed_databases(server_id,database_name,owner_user,owner_password_enc) VALUES(1,'measured','owner',''),(1,'missing','owner','')`); err != nil { t.Fatal(err) }
	count, size := int64(5), int64(4096)
	if err := database.SaveDatabaseStatistics(1, "measured", &count, &size); err != nil { t.Fatal(err) }
	list, err := database.ListManagedDatabases(nil)
	if err != nil { t.Fatal(err) }
	if len(list) != 2 { t.Fatalf("got %d databases", len(list)) }
	measured, missing := list[1], list[0]
	if measured.TableCount == nil || *measured.TableCount != count || measured.SizeBytes == nil || *measured.SizeBytes != size { t.Fatalf("unexpected stats: %+v", measured) }
	if missing.TableCount != nil || missing.SizeBytes != nil { t.Fatalf("missing statistics should be null: %+v", missing) }
	encoded, err := json.Marshal(missing)
	if err != nil { t.Fatal(err) }
	if string(encoded) == "" { t.Fatal("empty JSON") }
}
