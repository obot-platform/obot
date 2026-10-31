package adminmcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjectgroups"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/version"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// BuiltinName names the admin MCP server among the MCP servers built into Obot.
	BuiltinName = "obot-admin"
	// BuiltinEntryName is the catalog entry through which admins add the admin MCP server to vMCPs.
	BuiltinEntryName = system.DefaultCatalog + "-" + BuiltinName
)

// Builtin serves the admin MCP server as a vMCP component. The gateway only routes a vMCP's own
// loopback requests here, so req's user is whoever connected to the vMCP, and access to the vMCP
// has already been checked against its profiles. Profiles are configuration and can be too broad,
// so the user must still be an admin or owner now.
func (h *Handler) Builtin(req api.Context) http.Handler {
	if !subjectgroups.IsAdmin(req.User) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "only admins and owners can use the Obot Admin MCP server", http.StatusForbidden)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		h.streamable.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestKey{}, req)))
	})
}

// EnsureBuiltinEntry keeps the Obot Admin catalog entry in the default catalog current, or removes
// it when the server flag turns the admin MCP server off. The entry isn't synced from a source, so
// catalog sync leaves it alone, and it isn't editable.
func EnsureBuiltinEntry(ctx context.Context, c kclient.Client, allowed bool) error {
	var existing v1.MCPServerCatalogEntry
	err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: BuiltinEntryName}, &existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get the Obot Admin catalog entry: %w", err)
	}
	found := err == nil

	if !allowed {
		if found {
			if err := c.Delete(ctx, &existing); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete the Obot Admin catalog entry: %w", err)
			}
			slog.Info("Deleted the Obot Admin catalog entry because the admin MCP server is turned off")
		}
		return nil
	}

	manifest, err := builtinEntryManifest(ctx)
	if err != nil {
		return err
	}

	if !found {
		if err := c.Create(ctx, &v1.MCPServerCatalogEntry{
			Name:      BuiltinEntryName,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: system.DefaultCatalog,
				Manifest:       manifest,
			},
		}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create the Obot Admin catalog entry: %w", err)
		}
		slog.Info("Created the Obot Admin catalog entry", "entry", BuiltinEntryName)
		return nil
	}

	if reflect.DeepEqual(existing.Spec.Manifest, manifest) && existing.Spec.MCPCatalogName == system.DefaultCatalog && !existing.Spec.Editable {
		return nil
	}
	existing.Spec.MCPCatalogName = system.DefaultCatalog
	existing.Spec.Editable = false
	existing.Spec.Manifest = manifest
	if err := c.Update(ctx, &existing); err != nil {
		return fmt.Errorf("failed to update the Obot Admin catalog entry: %w", err)
	}
	slog.Info("Updated the Obot Admin catalog entry", "entry", BuiltinEntryName)
	return nil
}

func builtinEntryManifest(ctx context.Context) (types.MCPServerCatalogEntryManifest, error) {
	tools, err := builtinTools(ctx)
	if err != nil {
		return types.MCPServerCatalogEntryManifest{}, err
	}
	return types.MCPServerCatalogEntryManifest{
		Name:             "Obot Admin",
		ShortDescription: "Author Obot catalog files with a coding agent",
		Description: "Helps admins author catalog source files, the YAML that Obot catalogs sync vMCPs and catalog entries from, with a coding agent. " +
			"It looks up entry references, tool names, groups, users and secret bindings, and checks files the way catalog sync will. It never changes Obot.\n\n" +
			"It runs inside Obot. Only admins and owners can use it, whatever a vMCP's profiles grant. Its schema for catalog files is served at " + system.CatalogItemSchemaPath + ".",
		Icon:          "/user/images/obot-icon-blue.svg",
		Runtime:       types.RuntimeBuiltin,
		BuiltinConfig: &types.BuiltinRuntimeConfig{Name: BuiltinName},
		ToolPreview:   tools,
	}, nil
}

// builtinTools lists the admin MCP server's tools by connecting to it in memory.
func builtinTools(ctx context.Context) ([]types.MCPServerTool, error) {
	serverTransport, clientTransport := gomcp.NewInMemoryTransports()
	serverSession, err := (&Handler{}).newServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start the admin MCP server: %w", err)
	}
	defer serverSession.Close()

	client := gomcp.NewClient(&gomcp.Implementation{Name: "obot", Version: version.Tag}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the admin MCP server: %w", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list the admin MCP server's tools: %w", err)
	}
	tools := make([]types.MCPServerTool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		tools = append(tools, types.MCPServerTool{
			ID:          tool.Name,
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return tools, nil
}
