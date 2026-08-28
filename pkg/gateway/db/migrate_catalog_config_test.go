package db

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAutoMigrateFlattensServerConfigOnce(t *testing.T) {
	for _, test := range []struct {
		table     string
		migration string
	}{
		{
			table:     "mcpserver",
			migration: mcpServerConfigMigrationName,
		},
		{
			table:     "systemmcpserver",
			migration: systemMCPServerConfigMigrationName,
		},
	} {
		t.Run(test.table, func(t *testing.T) {
			db, sql := newCatalogConfigMigrationDB(t)
			if err := sql.Exec("CREATE TABLE " + test.table + " (id INTEGER PRIMARY KEY, name TEXT NOT NULL, namespace TEXT NOT NULL, deleted INTEGER NOT NULL, value TEXT NOT NULL)").Error; err != nil {
				t.Fatal(err)
			}
			legacy := catalogEntryValue(t, "https://catalog.example/source.yaml", map[string]any{
				"runtime": "remote",
				"env": []any{
					map[string]any{"key": "TOKEN", "value": "secret", "sensitive": true},
					map[string]any{"key": "FILE", "file": true, "dynamicFile": true},
					map[string]any{"key": "INPUT", "interpolated": true},
				},
				"remoteConfig": map[string]any{
					"url":     "https://example.com/mcp",
					"headers": []any{map[string]any{"key": "STATIC", "value": "fixed"}},
				},
				"multiUserConfig": map[string]any{
					"userDefinedHeaders": []any{map[string]any{"key": "USER", "required": true, "prefix": "Bearer "}},
				},
				"compositeConfig": map[string]any{
					"componentServers": []any{map[string]any{
						"manifest": map[string]any{"env": []any{map[string]any{"key": "NESTED"}}},
					}},
				},
			})
			for _, row := range []struct {
				id      int
				name    string
				deleted int
			}{
				{
					id:   1,
					name: "live",
				},
				{
					id:   2,
					name: "live",
				},
				{
					id:      3,
					name:    "deleted",
					deleted: 1,
				},
			} {
				if err := sql.Exec("INSERT INTO "+test.table+" (id, name, namespace, deleted, value) VALUES (?, ?, 'default', ?, ?)", row.id, row.name, row.deleted, legacy).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.AutoMigrate(); err != nil {
				t.Fatal(err)
			}
			var rows []struct {
				ID    int
				Value string
			}
			if err := sql.Table(test.table).Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if rows[0].Value != legacy || rows[2].Value != legacy {
				t.Fatal("historical or deleted server changed")
			}
			manifest := catalogEntryManifest(t, rows[1].Value)
			assertConfigKeys(t, manifest, map[string]types.Usage{
				"TOKEN":  types.Env,
				"FILE":   types.DynamicFile,
				"INPUT":  types.Interpolated,
				"STATIC": types.Header,
				"USER":   types.Header,
			})
			user := configByKey(t, manifest, "USER")
			if !user.UserAllowed || !user.Required || user.Prefix != "Bearer " {
				t.Fatalf("per-user configuration lost: %#v", user)
			}
			if configByKey(t, manifest, "TOKEN").Value != "secret" || configByKey(t, manifest, "STATIC").UserAllowed {
				t.Fatal("static configuration changed")
			}
			for _, field := range []string{"env", "multiUserConfig"} {
				if manifest[field] != nil {
					t.Fatalf("deprecated field %s retained", field)
				}
			}
			if rawObject(t, manifest["remoteConfig"])["headers"] != nil {
				t.Fatal("deprecated headers retained")
			}
			var composite struct {
				ComponentServers []struct{ Manifest types.MCPServerManifest } `json:"componentServers"`
			}
			if err := json.Unmarshal(manifest["compositeConfig"], &composite); err != nil {
				t.Fatal(err)
			}
			if len(composite.ComponentServers[0].Manifest.Config) != 1 {
				t.Fatal("nested server snapshot was not migrated")
			}
			assertNamedMigrationRecorded(t, sql, test.migration)
			if err := sql.Table(test.table).Where("id = 2").Update("value", legacy).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(); err != nil {
				t.Fatal(err)
			}
			var result string
			if err := sql.Table(test.table).Where("id = 2").Select("value").Scan(&result).Error; err != nil {
				t.Fatal(err)
			}
			if result != legacy {
				t.Fatal("completed server migration ran again")
			}
		})
	}
}

func TestAutoMigrateFlattensInstanceConfig(t *testing.T) {
	db, sql := newCatalogConfigMigrationDB(t)
	if err := sql.Exec("CREATE TABLE mcpserverinstance (id INTEGER PRIMARY KEY, name TEXT NOT NULL, namespace TEXT NOT NULL, deleted INTEGER NOT NULL, value TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	const legacy = `{"metadata":{"name":"instance"},"spec":{"userID":"user","mcpServerName":"server","multiUserConfig":{"userDefinedHeaders":[{"key":"TOKEN","required":true}]}}}`
	if err := sql.Exec("INSERT INTO mcpserverinstance VALUES (1, 'instance', 'default', 0, ?)", legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := sql.Table("mcpserverinstance").Select("value").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	spec := rawObject(t, rawObject(t, json.RawMessage(value))["spec"])
	config := configByKey(t, spec, "TOKEN")
	if !config.UserAllowed || config.Usage != types.Header || !config.Required {
		t.Fatalf("instance header changed: %#v", config)
	}
	if spec["multiUserConfig"] != nil || string(spec["userID"]) != `"user"` || string(spec["mcpServerName"]) != `"server"` {
		t.Fatalf("instance fields changed: %s", value)
	}
	assertNamedMigrationRecorded(t, sql, mcpServerConfigMigrationName)
}

func TestServerConfigMigrationRejectsConflictingKeys(t *testing.T) {
	const legacy = `{"spec":{"manifest":{"env":[{"key":"TOKEN"}],"remoteConfig":{"headers":[{"key":"TOKEN"}]}}}}`
	if converted, err := flattenStoredMCPConfig([]byte(legacy), true); err == nil || converted != nil {
		t.Fatalf("conflicting configuration must fail without a replacement: %s, %v", converted, err)
	}
}

func TestAutoMigrateFlattensUICatalogEntryConfigOnce(t *testing.T) {
	db, gormDB := newCatalogConfigMigrationDB(t)
	createCatalogEntryTable(t, gormDB)

	legacy := catalogEntryValue(t, "", map[string]any{
		"name":           "UI entry",
		"runtime":        "remote",
		"serverUserType": "multiUser",
		"config": []any{
			map[string]any{"key": "EXISTING", "usage": "env"},
		},
		"env": []any{
			map[string]any{"name": "Env", "description": "env description", "key": "ENV", "value": "value", "sensitive": true, "required": true, "prefix": "Token "},
			map[string]any{"key": "FILE", "file": true},
			map[string]any{"key": "DYNAMIC", "file": true, "dynamicFile": true},
			map[string]any{"key": "INTERPOLATED", "interpolated": true},
		},
		"remoteConfig": map[string]any{
			"fixedURL": "https://example.com/mcp",
			"headers": []any{
				map[string]any{"key": "REMOTE_HEADER", "required": true, "prefix": "Bearer "},
			},
		},
		"multiUserConfig": map[string]any{
			"userDefinedHeaders": []any{
				map[string]any{"key": "USER_HEADER", "sensitive": true},
			},
		},
	})
	prior := catalogEntryValue(t, "", map[string]any{"name": "prior", "runtime": "npx", "env": []any{map[string]any{"key": "PRIOR"}}})
	source := catalogEntryValue(t, "https://catalog.example/entry.yaml", map[string]any{"name": "source", "runtime": "npx", "env": []any{map[string]any{"key": "SOURCE"}}})
	deleted := catalogEntryValue(t, "", map[string]any{"name": "deleted", "runtime": "npx", "env": []any{map[string]any{"key": "DELETED"}}})

	insertCatalogEntry(t, gormDB, 1, "ui", 0, prior)
	insertCatalogEntry(t, gormDB, 2, "ui", 0, legacy)
	insertCatalogEntry(t, gormDB, 3, "source", 0, source)
	insertCatalogEntry(t, gormDB, 4, "deleted", 0, legacy)
	insertCatalogEntry(t, gormDB, 5, "deleted", 1, deleted)

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("startup migration failed: %v", err)
	}

	assertConfigKeys(t, catalogEntryManifest(t, catalogEntryValueForID(t, gormDB, 2)), map[string]types.Usage{
		"EXISTING":      types.Env,
		"ENV":           types.Env,
		"FILE":          types.File,
		"DYNAMIC":       types.DynamicFile,
		"INTERPOLATED":  types.Interpolated,
		"REMOTE_HEADER": types.Header,
		"USER_HEADER":   types.Header,
	})
	manifest := catalogEntryManifest(t, catalogEntryValueForID(t, gormDB, 2))
	config := configByKey(t, manifest, "ENV")
	if config.Name != "Env" || config.Description != "env description" || config.Value != "value" || !config.Sensitive || !config.Required || config.Prefix != "Token " {
		t.Fatalf("expected env metadata to be preserved, got %#v", config)
	}
	if _, ok := manifest["env"]; ok {
		t.Fatal("expected legacy env to be removed")
	}
	if _, ok := manifest["serverUserType"]; ok {
		t.Fatal("expected serverUserType to be removed")
	}
	if _, ok := manifest["multiUserConfig"]; ok {
		t.Fatal("expected empty legacy multiUserConfig to be removed")
	}
	remote := rawObject(t, manifest["remoteConfig"])
	if _, ok := remote["headers"]; ok {
		t.Fatal("expected legacy remote headers to be removed")
	}
	if got := remote["fixedURL"]; string(got) != `"https://example.com/mcp"` {
		t.Fatalf("expected remote configuration to be retained, got %s", got)
	}

	if got := catalogEntryValueForID(t, gormDB, 1); got != prior {
		t.Fatalf("expected historical row to remain unchanged\nwant: %s\n got: %s", prior, got)
	}
	if got := catalogEntryValueForID(t, gormDB, 3); got != source {
		t.Fatalf("expected source-backed entry to remain unchanged\nwant: %s\n got: %s", source, got)
	}
	if got := catalogEntryValueForID(t, gormDB, 5); got != deleted {
		t.Fatalf("expected deleted entry to remain unchanged\nwant: %s\n got: %s", deleted, got)
	}
	assertMigrationRecorded(t, gormDB)

	// The migration record makes a second startup a no-op, even if an old
	// value appears later in the table.
	if err := gormDB.Table("mcpservercatalogentry").Where("id = ?", 2).Update("value", legacy).Error; err != nil {
		t.Fatalf("failed to restore legacy value: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("second startup migration failed: %v", err)
	}
	if got := catalogEntryValueForID(t, gormDB, 2); got != legacy {
		t.Fatalf("expected completed migration to be a no-op\nwant: %s\n got: %s", legacy, got)
	}
}

func TestAutoMigrateFlattensUISystemCatalogEntryConfigOnce(t *testing.T) {
	db, gormDB := newCatalogConfigMigrationDB(t)
	createSystemCatalogEntryTable(t, gormDB)

	legacy := catalogEntryValue(t, "", map[string]any{
		"name":           "UI system entry",
		"runtime":        "remote",
		"serverUserType": "singleUser",
		"env":            []any{map[string]any{"key": "TOKEN", "required": true}},
		"remoteConfig": map[string]any{
			"fixedURL": "https://example.com/mcp",
			"headers":  []any{map[string]any{"key": "Authorization", "prefix": "Bearer "}},
		},
	})
	insertSystemCatalogEntry(t, gormDB, 1, "system", 0, legacy)

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("startup migration failed: %v", err)
	}
	manifest := catalogEntryManifest(t, systemCatalogEntryValueForID(t, gormDB, 1))
	assertConfigKeys(t, manifest, map[string]types.Usage{
		"TOKEN":         types.Env,
		"Authorization": types.Header,
	})
	if _, ok := manifest["env"]; ok {
		t.Fatal("expected legacy env to be removed")
	}
	if _, ok := manifest["serverUserType"]; ok {
		t.Fatal("expected serverUserType to be removed")
	}
	if _, ok := rawObject(t, manifest["remoteConfig"])["headers"]; ok {
		t.Fatal("expected legacy remote headers to be removed")
	}
	assertNamedMigrationRecorded(t, gormDB, uiSystemCatalogConfigMigrationName)

	if err := gormDB.Table("systemmcpservercatalogentry").Where("id = ?", 1).Update("value", legacy).Error; err != nil {
		t.Fatalf("failed to restore legacy value: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("second startup migration failed: %v", err)
	}
	if got := systemCatalogEntryValueForID(t, gormDB, 1); got != legacy {
		t.Fatalf("expected completed migration to be a no-op\nwant: %s\n got: %s", legacy, got)
	}
}

func TestAutoMigratePreservesCompositeComponentSnapshots(t *testing.T) {
	db, gormDB := newCatalogConfigMigrationDB(t)
	createCatalogEntryTable(t, gormDB)
	value := catalogEntryValue(t, "", map[string]any{
		"name":           "composite",
		"runtime":        "composite",
		"serverUserType": "multiUser",
		"env":            []any{map[string]any{"key": "OUTER_ENV"}},
		"compositeConfig": map[string]any{"componentServers": []any{
			map[string]any{"catalogEntryID": "component", "manifest": map[string]any{
				"name":           "component",
				"runtime":        "remote",
				"serverUserType": "singleUser",
				"env":            []any{map[string]any{"key": "COMPONENT_ENV"}},
				"remoteConfig": map[string]any{"headers": []any{
					map[string]any{"key": "COMPONENT_HEADER"},
				}},
			}},
		}},
	})
	insertCatalogEntry(t, gormDB, 1, "composite", 0, value)

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("startup migration failed: %v", err)
	}

	manifest := catalogEntryManifest(t, catalogEntryValueForID(t, gormDB, 1))
	assertConfigKeys(t, manifest, map[string]types.Usage{"OUTER_ENV": types.Env})
	if _, ok := manifest["env"]; ok {
		t.Fatal("expected outer legacy env to be removed")
	}
	if _, ok := manifest["serverUserType"]; ok {
		t.Fatal("expected outer serverUserType to be removed")
	}
	composite := rawObject(t, manifest["compositeConfig"])
	components := rawSlice(t, composite["componentServers"])
	component := rawObject(t, components[0])
	componentManifest := rawObject(t, component["manifest"])
	// Composite snapshots remain byte-compatible for compositemigration, which
	// owns their conversion while building the vMCP.
	if _, ok := componentManifest["env"]; !ok {
		t.Fatal("expected component legacy env to be preserved")
	}
	if _, ok := componentManifest["serverUserType"]; !ok {
		t.Fatal("expected component serverUserType to be preserved")
	}
	if _, ok := rawObject(t, componentManifest["remoteConfig"])["headers"]; !ok {
		t.Fatal("expected component remote headers to be preserved")
	}
}

func TestAutoMigrateCatalogConfigRollsBackAndRetries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{
			name:  "malformed JSON",
			value: `{"spec":`,
		},
		{
			name: "duplicate config key",
			value: catalogEntryValue(t, "", map[string]any{
				"name":    "duplicate",
				"runtime": "npx",
				"config":  []any{map[string]any{"key": "DUPLICATE", "usage": "env"}},
				"env":     []any{map[string]any{"key": "DUPLICATE"}},
			}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, gormDB := newCatalogConfigMigrationDB(t)
			createCatalogEntryTable(t, gormDB)
			first := catalogEntryValue(t, "", map[string]any{"name": "first", "runtime": "npx", "env": []any{map[string]any{"key": "FIRST"}}})
			insertCatalogEntry(t, gormDB, 1, "first", 0, first)
			insertCatalogEntry(t, gormDB, 2, "entry", 0, tc.value)

			if err := db.AutoMigrate(); err == nil {
				t.Fatal("expected startup migration to fail")
			}
			if got := catalogEntryValueForID(t, gormDB, 1); got != first {
				t.Fatalf("expected prior converted row to roll back\nwant: %s\n got: %s", first, got)
			}
			if got := catalogEntryValueForID(t, gormDB, 2); got != tc.value {
				t.Fatalf("expected failing row to remain unchanged\nwant: %s\n got: %s", tc.value, got)
			}
			assertMigrationNotRecorded(t, gormDB)

			valid := catalogEntryValue(t, "", map[string]any{"name": "valid", "runtime": "npx", "env": []any{map[string]any{"key": "VALID"}}})
			if err := gormDB.Table("mcpservercatalogentry").Where("id = ?", 2).Update("value", valid).Error; err != nil {
				t.Fatalf("failed to repair catalog entry: %v", err)
			}
			if err := db.AutoMigrate(); err != nil {
				t.Fatalf("expected repaired migration to succeed: %v", err)
			}
			assertConfigKeys(t, catalogEntryManifest(t, catalogEntryValueForID(t, gormDB, 1)), map[string]types.Usage{"FIRST": types.Env})
			assertConfigKeys(t, catalogEntryManifest(t, catalogEntryValueForID(t, gormDB, 2)), map[string]types.Usage{"VALID": types.Env})
			assertMigrationRecorded(t, gormDB)
		})
	}
}

func newCatalogConfigMigrationDB(t *testing.T) (*DB, *gorm.DB) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("failed to get SQL database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := New(gormDB, sqlDB, true)
	if err != nil {
		t.Fatalf("failed to construct gateway database: %v", err)
	}
	return db, gormDB
}

func createCatalogEntryTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("CREATE TABLE mcpservercatalogentry (id INTEGER PRIMARY KEY, name TEXT NOT NULL, namespace TEXT NOT NULL, deleted INTEGER NOT NULL, value TEXT NOT NULL)").Error; err != nil {
		t.Fatalf("failed to create catalog entry table: %v", err)
	}
}

func createSystemCatalogEntryTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("CREATE TABLE systemmcpservercatalogentry (id INTEGER PRIMARY KEY, name TEXT NOT NULL, namespace TEXT NOT NULL, deleted INTEGER NOT NULL, value TEXT NOT NULL)").Error; err != nil {
		t.Fatalf("failed to create system catalog entry table: %v", err)
	}
}

func insertCatalogEntry(t *testing.T, db *gorm.DB, id int, name string, deleted int, value string) {
	t.Helper()
	if err := db.Exec("INSERT INTO mcpservercatalogentry (id, name, namespace, deleted, value) VALUES (?, ?, ?, ?, ?)", id, name, "default", deleted, value).Error; err != nil {
		t.Fatalf("failed to insert catalog entry: %v", err)
	}
}

func insertSystemCatalogEntry(t *testing.T, db *gorm.DB, id int, name string, deleted int, value string) {
	t.Helper()
	if err := db.Exec("INSERT INTO systemmcpservercatalogentry (id, name, namespace, deleted, value) VALUES (?, ?, ?, ?, ?)", id, name, "default", deleted, value).Error; err != nil {
		t.Fatalf("failed to insert system catalog entry: %v", err)
	}
}

func catalogEntryValue(t *testing.T, sourceURL string, manifest map[string]any) string {
	t.Helper()
	spec := map[string]any{"manifest": manifest}
	if sourceURL != "" {
		spec["sourceURL"] = sourceURL
	}
	value, err := json.Marshal(map[string]any{"spec": spec})
	if err != nil {
		t.Fatalf("failed to marshal catalog entry: %v", err)
	}
	return string(value)
}

func catalogEntryValueForID(t *testing.T, db *gorm.DB, id int) string {
	t.Helper()
	var value string
	if err := db.Table("mcpservercatalogentry").Select("value").Where("id = ?", id).Scan(&value).Error; err != nil {
		t.Fatalf("failed to load catalog entry: %v", err)
	}
	return value
}

func systemCatalogEntryValueForID(t *testing.T, db *gorm.DB, id int) string {
	t.Helper()
	var value string
	if err := db.Table("systemmcpservercatalogentry").Select("value").Where("id = ?", id).Scan(&value).Error; err != nil {
		t.Fatalf("failed to load system catalog entry: %v", err)
	}
	return value
}

func catalogEntryManifest(t *testing.T, value string) map[string]json.RawMessage {
	t.Helper()
	var entry struct {
		Spec struct {
			Manifest json.RawMessage `json:"manifest"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(value), &entry); err != nil {
		t.Fatalf("failed to decode catalog entry: %v", err)
	}
	return rawObject(t, entry.Spec.Manifest)
}

func rawObject(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	return result
}

func rawSlice(t *testing.T, raw json.RawMessage) []json.RawMessage {
	t.Helper()
	var result []json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("failed to decode array: %v", err)
	}
	return result
}

func assertConfigKeys(t *testing.T, manifest map[string]json.RawMessage, want map[string]types.Usage) {
	t.Helper()
	var config []types.MCPConfig
	if err := json.Unmarshal(manifest["config"], &config); err != nil {
		t.Fatalf("failed to decode config: %v", err)
	}
	got := make(map[string]types.Usage, len(config))
	for _, item := range config {
		got[item.Key] = item.Usage
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d config items, got %d: %#v", len(want), len(got), got)
	}
	for key, usage := range want {
		if got[key] != usage {
			t.Errorf("config %q usage: want %q, got %q", key, usage, got[key])
		}
	}
}

func configByKey(t *testing.T, manifest map[string]json.RawMessage, key string) types.MCPConfig {
	t.Helper()
	var config []types.MCPConfig
	if err := json.Unmarshal(manifest["config"], &config); err != nil {
		t.Fatalf("failed to decode config: %v", err)
	}
	for _, item := range config {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("config %q not found", key)
	return types.MCPConfig{}
}

func assertMigrationRecorded(t *testing.T, db *gorm.DB) {
	t.Helper()
	assertNamedMigrationRecorded(t, db, uiCatalogConfigMigrationName)
}

func assertNamedMigrationRecorded(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	var migration gatewaytypes.Migration
	if err := db.Where("name = ?", name).First(&migration).Error; err != nil {
		t.Fatalf("expected migration record: %v", err)
	}
}

func assertMigrationNotRecorded(t *testing.T, db *gorm.DB) {
	t.Helper()
	if !db.Migrator().HasTable(&gatewaytypes.Migration{}) {
		return
	}
	var count int64
	if err := db.Model(&gatewaytypes.Migration{}).Where("name = ?", uiCatalogConfigMigrationName).Count(&count).Error; err != nil {
		t.Fatalf("failed to count migration records: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no migration record after rollback, got %d", count)
	}
}
