package db

import (
	"encoding/json"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"gorm.io/gorm"
)

const (
	uiCatalogConfigMigrationName       = "ui_catalog_entries_flatten_config"
	uiSystemCatalogConfigMigrationName = "ui_system_catalog_entries_flatten_config"
	mcpServerConfigMigrationName       = "mcp_servers_flatten_config"
	systemMCPServerConfigMigrationName = "system_mcp_servers_flatten_config"
)

// Migrate raw storage JSON before controllers start. Decoding through the current
// catalog type would discard legacy configuration before it could be converted.
func migrateUICatalogEntryConfig(tx *gorm.DB) error {
	return migrateUICatalogConfigTable(tx, "mcpservercatalogentry")
}

func migrateUICatalogConfigTable(tx *gorm.DB, table string) error {
	if !tx.Migrator().HasTable(table) {
		return nil
	}
	var entries []struct {
		ID    int64
		Value string
	}
	// Kinm keeps historical versions in the same table. Only change the latest
	// live version, never a historical row or a deleted resource.
	latest := tx.Table(table).Select("MAX(id)").Group("namespace, name")
	if err := tx.Table(table).Select("id, value").Where("id IN (?) AND deleted = 0", latest).Order("id").Find(&entries).Error; err != nil {
		return err
	}
	for _, entry := range entries {
		var value []byte
		var err error
		if table == "mcpserverinstance" {
			value, err = flattenMCPInstanceConfig([]byte(entry.Value))
		} else {
			value, err = flattenStoredMCPConfig([]byte(entry.Value), table == "mcpserver" || table == "systemmcpserver")
		}
		if err != nil {
			return fmt.Errorf("%s row %d: %w", table, entry.ID, err)
		}
		if value == nil {
			continue
		}
		if err := tx.Table(table).Where("id = ?", entry.ID).Update("value", string(value)).Error; err != nil {
			return err
		}
	}
	return nil
}

func flattenMCPInstanceConfig(data []byte) ([]byte, error) {
	var instance map[string]json.RawMessage
	if err := json.Unmarshal(data, &instance); err != nil {
		return nil, err
	}
	wrapped, err := json.Marshal(map[string]any{"spec": map[string]any{"manifest": instance["spec"]}})
	if err != nil {
		return nil, err
	}
	converted, err := flattenStoredMCPConfig(wrapped, true)
	if err != nil || converted == nil {
		return nil, err
	}
	var result struct {
		Spec struct {
			Manifest json.RawMessage `json:"manifest"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(converted, &result); err != nil {
		return nil, err
	}
	instance["spec"] = result.Spec.Manifest
	return json.Marshal(instance)
}

func flattenStoredMCPConfig(data []byte, server bool) ([]byte, error) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(entry["spec"], &spec); err != nil {
		return nil, err
	}
	var sourceURL string
	if raw := spec["sourceURL"]; raw != nil {
		if err := json.Unmarshal(raw, &sourceURL); err != nil {
			return nil, err
		}
	}
	if !server && sourceURL != "" {
		return nil, nil
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(spec["manifest"], &manifest); err != nil {
		return nil, err
	}
	var config []types.MCPConfig
	if raw := manifest["config"]; raw != nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return nil, err
		}
	}
	var changed bool
	if raw, ok := manifest["env"]; ok {
		var env []types.MCPEnv
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, err
		}
		for _, item := range env {
			config = append(config, types.ConfigFromEnv(item))
		}
		delete(manifest, "env")
		changed = true
	}
	for _, source := range []struct {
		parent string
		key    string
	}{
		{parent: "remoteConfig", key: "headers"},
		{parent: "multiUserConfig", key: "userDefinedHeaders"},
	} {
		if manifest[source.parent] == nil {
			continue
		}
		var parent map[string]json.RawMessage
		if err := json.Unmarshal(manifest[source.parent], &parent); err != nil {
			return nil, err
		}
		if raw, ok := parent[source.key]; ok {
			var headers []types.MCPHeader
			if err := json.Unmarshal(raw, &headers); err != nil {
				return nil, err
			}
			for _, item := range headers {
				field := types.ConfigFromHeader(item)
				field.UserAllowed = server && source.parent == "multiUserConfig"
				config = append(config, field)
			}
			delete(parent, source.key)
			changed = true
		}
		if source.parent == "multiUserConfig" && len(parent) == 0 {
			delete(manifest, source.parent)
			changed = true
		} else {
			value, err := json.Marshal(parent)
			if err != nil {
				return nil, err
			}
			manifest[source.parent] = value
		}
	}
	if _, ok := manifest["serverUserType"]; ok {
		delete(manifest, "serverUserType")
		changed = true
	}
	// Composite server snapshots are server manifests too. Convert them before
	// the composite migration reads them through the current runtime types.
	if server && manifest["compositeConfig"] != nil {
		var composite map[string]json.RawMessage
		if err := json.Unmarshal(manifest["compositeConfig"], &composite); err != nil {
			return nil, err
		}
		var components []map[string]json.RawMessage
		if err := json.Unmarshal(composite["componentServers"], &components); err != nil {
			return nil, err
		}
		for _, component := range components {
			if component["manifest"] == nil {
				continue
			}
			wrapped, err := json.Marshal(map[string]any{"spec": map[string]any{"manifest": component["manifest"]}})
			if err != nil {
				return nil, err
			}
			converted, err := flattenStoredMCPConfig(wrapped, true)
			if err != nil {
				return nil, err
			}
			if converted != nil {
				var result struct {
					Spec struct {
						Manifest json.RawMessage `json:"manifest"`
					} `json:"spec"`
				}
				if err := json.Unmarshal(converted, &result); err != nil {
					return nil, err
				}
				component["manifest"] = result.Spec.Manifest
				changed = true
			}
		}
		value, err := json.Marshal(components)
		if err != nil {
			return nil, err
		}
		composite["componentServers"] = value
		value, err = json.Marshal(composite)
		if err != nil {
			return nil, err
		}
		manifest["compositeConfig"] = value
	}
	if !changed {
		return nil, nil
	}
	if err := (types.MCPServerManifest{Config: config}).ValidateConfig(); err != nil {
		return nil, err
	}
	if len(config) > 0 {
		value, err := json.Marshal(config)
		if err != nil {
			return nil, err
		}
		manifest["config"] = value
	}
	value, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	spec["manifest"] = value
	value, err = json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	entry["spec"] = value
	return json.Marshal(entry)
}
