package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"uuid"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	staticConfigurationCredentialContextPrefix = "mcp-catalog-entry/"
	staticConfigurationCredentialNamePrefix    = "configuration-"
)

// StaticConfigurationRevealer reads the credentials holding catalog entry static configuration.
type StaticConfigurationRevealer interface {
	RevealCredential(ctx context.Context, contexts []string, name string) (gatewaytypes.Credential, error)
}

// StaticConfigurationStore reads and writes the credentials holding catalog entry static configuration.
type StaticConfigurationStore interface {
	StaticConfigurationRevealer
	UpsertCredential(ctx context.Context, credential gatewaytypes.Credential) error
}

// StaticConfigurationCredentialContext scopes static configuration to its catalog entry. Every
// revision of the entry's static configuration is stored in this context.
func StaticConfigurationCredentialContext(entryName string) string {
	return staticConfigurationCredentialContextPrefix + entryName
}

// StaticConfigurationCredentialName names the credential holding one revision of static configuration.
func StaticConfigurationCredentialName(revision string) string {
	return staticConfigurationCredentialNamePrefix + revision
}

// StaticConfigurationRevisionFromCredentialName reverses StaticConfigurationCredentialName.
func StaticConfigurationRevisionFromCredentialName(name string) (string, bool) {
	revision, ok := strings.CutPrefix(name, staticConfigurationCredentialNamePrefix)
	return revision, ok && revision != ""
}

// HasStaticConfiguration reports whether any field's value is stored in a static configuration credential.
func HasStaticConfiguration(config []types.MCPConfig) bool {
	return slices.ContainsFunc(config, func(field types.MCPConfig) bool { return field.Static })
}

// ExtractStaticConfiguration removes literal values from config, marks those fields Static, and
// returns the values keyed by configuration key. A Static field without a value keeps its value
// from previous when keepPrevious is set; otherwise it is an error, because nothing else can
// supply it.
func ExtractStaticConfiguration(config []types.MCPConfig, previous map[string]string, keepPrevious bool) (map[string]string, error) {
	values := make(map[string]string)
	for i := range config {
		field := &config[i]
		switch {
		case field.SecretBinding != nil:
			// Secret-bound values are resolved at launch and never stored here.
			field.Static = false
		case field.Value != "":
			values[field.Key] = field.Value
			field.Static = true
		case field.Static:
			value := previous[field.Key]
			if !keepPrevious || value == "" {
				return nil, fmt.Errorf("static configuration %q requires a value", field.Key)
			}
			values[field.Key] = value
		}
		field.Value = ""
	}
	return values, nil
}

// StoreStaticConfiguration moves the literal configuration values of an entry manifest into a
// credential and records the credential's revision on the manifest. Every Static field must have a
// value, because nothing else supplies one.
func StoreStaticConfiguration(ctx context.Context, store StaticConfigurationStore, entryName string, manifest *types.MCPServerCatalogEntryManifest, previousRevision string) error {
	previous, err := RevealStaticConfiguration(ctx, store, entryName, previousRevision)
	if err != nil {
		return err
	}

	values, err := ExtractStaticConfiguration(manifest.Config, nil, false)
	if err != nil {
		return err
	}

	manifest.StaticConfigurationRevision, err = StoreStaticConfigurationValues(ctx, store, entryName, values, previousRevision, previous)
	return err
}

// StoreStaticConfigurationValues stores static configuration values for an entry and returns the
// revision that holds them. The previous revision is kept when its values are unchanged, so
// repeated synchronization does not change the entry. Changed values are written to a new
// revision, so servers and vMCP snapshots of the old manifest keep the values they were created
// with until they are updated.
func StoreStaticConfigurationValues(ctx context.Context, store StaticConfigurationStore, entryName string, values map[string]string, previousRevision string, previous map[string]string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if previousRevision != "" && maps.Equal(values, previous) {
		return previousRevision, nil
	}

	revision := uuid.New().String()
	if err := store.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: StaticConfigurationCredentialContext(entryName),
		Name:    StaticConfigurationCredentialName(revision),
		Secrets: values,
	}); err != nil {
		return "", fmt.Errorf("failed to store static configuration: %w", err)
	}
	return revision, nil
}

// RevealStaticConfiguration returns the values of one revision of an entry's static configuration.
// A missing credential yields no values. Every revision Obot stores holds at least one value, so
// callers that need the values treat an empty result for a revision as unavailable.
func RevealStaticConfiguration(ctx context.Context, revealer StaticConfigurationRevealer, entryName, revision string) (map[string]string, error) {
	if revision == "" {
		return map[string]string{}, nil
	}

	credential, err := revealer.RevealCredential(ctx, []string{StaticConfigurationCredentialContext(entryName)}, StaticConfigurationCredentialName(revision))
	if errors.As(err, &gateway.CredentialNotFoundError{}) {
		return map[string]string{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read static configuration for catalog entry %q: %w", entryName, err)
	}
	return credential.Secrets, nil
}

// ResolveStaticConfiguration returns a copy of config with the values of Static fields restored
// from the entry's static configuration credential. The result is for building runtime
// configuration only and must never be persisted.
func ResolveStaticConfiguration(ctx context.Context, revealer StaticConfigurationRevealer, entryName, revision string, config []types.MCPConfig) ([]types.MCPConfig, error) {
	if !HasStaticConfiguration(config) {
		return config, nil
	}

	values, err := RevealStaticConfiguration(ctx, revealer, entryName, revision)
	if err != nil {
		return nil, err
	}

	// Required configuration checks treat Static fields as supplied, so a Static field without a
	// stored value must not resolve to an empty one.
	resolved := slices.Clone(config)
	for i := range resolved {
		if !resolved[i].Static || resolved[i].Value != "" {
			continue
		}
		if resolved[i].Value = values[resolved[i].Key]; resolved[i].Value == "" {
			return nil, fmt.Errorf("static configuration %q for catalog entry %q is not available", resolved[i].Key, entryName)
		}
	}
	return resolved, nil
}

// ResolveServerStaticConfiguration returns a copy of server whose manifest has the values of its
// Static configuration restored. The result is for building runtime configuration only and must
// never be persisted.
func ResolveServerStaticConfiguration(ctx context.Context, revealer StaticConfigurationRevealer, server v1.MCPServer) (v1.MCPServer, error) {
	config, err := ResolveStaticConfiguration(ctx, revealer, server.Spec.MCPServerCatalogEntryName, server.Spec.Manifest.StaticConfigurationRevision, server.Spec.Manifest.Config)
	if err != nil {
		return v1.MCPServer{}, err
	}
	server.Spec.Manifest.Config = config
	return server, nil
}
