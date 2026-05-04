package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func addCatalogIDToAccessControlRules(ctx context.Context, client kclient.Client) error {
	var acRules v1.AccessControlRuleList
	if err := client.List(ctx, &acRules); err != nil {
		return err
	}

	// Iterate over each AccessControlRule and add CatalogID
	for _, acRule := range acRules.Items {
		if acRule.Spec.MCPCatalogID == "" && acRule.Spec.PowerUserWorkspaceID == "" {
			acRule.Spec.MCPCatalogID = system.DefaultCatalog
			if err := client.Update(ctx, &acRule); err != nil {
				return err
			}
		}
	}

	return nil
}

func migratePublishedArtifactVisibility(ctx context.Context, client kclient.Client) error {
	var artifacts v1.PublishedArtifactList
	if err := client.List(ctx, &artifacts); err != nil {
		return err
	}

	for i := range artifacts.Items {
		artifact := &artifacts.Items[i]
		if artifact.Spec.LegacyVisibility == "" {
			continue
		}

		var subjects []types.Subject
		switch artifact.Spec.LegacyVisibility {
		case "public":
			subjects = []types.Subject{{
				Type: types.SubjectTypeSelector,
				ID:   "*",
			}}
		case "private":
			subjects = nil
		default:
			log.Errorf("invalid legacy visibility %q for published artifact %s", artifact.Spec.LegacyVisibility, artifact.Name)
			// Make it private to be safe
			subjects = nil
		}

		for j := range artifact.Status.Versions {
			artifact.Status.Versions[j].Subjects = subjects
		}

		artifact.Spec.LegacyVisibility = ""
		if err := client.Update(ctx, artifact); err != nil {
			return err
		}
	}

	return nil
}

func migrateMultiUserMCPServerManifestValuesToCredentials(ctx context.Context, client kclient.Client, gptClient *gptscript.GPTScript) error {
	var servers v1.MCPServerList
	if err := client.List(ctx, &servers); err != nil {
		return err
	}

	for i := range servers.Items {
		server := &servers.Items[i]
		credCtx := mcpServerCredentialContext(*server)
		if credCtx == "" {
			continue
		}

		configValues, changed := extractAndClearMCPServerConfigValues(&server.Spec.Manifest)
		if !changed {
			continue
		}

		if len(configValues) > 0 {
			if existingCred, err := gptClient.RevealCredential(ctx, []string{credCtx}, server.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to find credential for MCP server %s: %w", server.Name, err)
			} else if err == nil {
				// Copy the new config values into the existing credential values so we don't lose any existing values that aren't in the manifest.
				maps.Copy(existingCred.Env, configValues)
				configValues = existingCred.Env
			}

			if err := gptClient.CreateCredential(ctx, gptscript.Credential{
				Context:  credCtx,
				ToolName: server.Name,
				Type:     gptscript.CredentialTypeTool,
				Env:      configValues,
			}); err != nil {
				return fmt.Errorf("failed to create credential for MCP server %s: %w", server.Name, err)
			}
		}

		if err := client.Update(ctx, server); err != nil {
			return fmt.Errorf("failed to clear manifest config values for MCP server %s: %w", server.Name, err)
		}
	}

	return nil
}

func mcpServerCredentialContext(server v1.MCPServer) string {
	switch {
	case server.Spec.MCPCatalogID != "":
		return fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
	case server.Spec.PowerUserWorkspaceID != "":
		return fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)
	default:
		return ""
	}
}

func extractAndClearMCPServerConfigValues(manifest *types.MCPServerManifest) (map[string]string, bool) {
	configValues := make(map[string]string)
	var changed bool

	for i := range manifest.Env {
		if manifest.Env[i].Value != "" {
			if manifest.Env[i].Key != "" {
				configValues[manifest.Env[i].Key] = manifest.Env[i].Value
			}
			manifest.Env[i].Value = ""
			changed = true
		}
	}

	if manifest.RemoteConfig != nil {
		for i := range manifest.RemoteConfig.Headers {
			if manifest.RemoteConfig.Headers[i].Value != "" {
				if manifest.RemoteConfig.Headers[i].Key != "" {
					configValues[manifest.RemoteConfig.Headers[i].Key] = manifest.RemoteConfig.Headers[i].Value
				}
				manifest.RemoteConfig.Headers[i].Value = ""
				changed = true
			}
		}
	}

	return configValues, changed
}
