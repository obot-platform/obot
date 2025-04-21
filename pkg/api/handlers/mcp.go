package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MCPHandler struct {
}

func NewMCPHandler() *MCPHandler {
	return &MCPHandler{}
}

func (m *MCPHandler) GetCatalogEntry(req api.Context) error {
	var (
		entry v1.MCPServerCatalogEntry
		id    = req.PathValue("id")
	)

	if err := req.Get(&entry, id); err != nil {
		return err
	}

	result := convertMCPServerCatalogEntry(entry)
	return req.Write(result)
}

func (m *MCPHandler) ListCatalog(req api.Context) error {
	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list); err != nil {
		return err
	}

	var result types.MCPServerCatalogEntryList
	for _, entry := range list.Items {
		result.Items = append(result.Items, convertMCPServerCatalogEntry(entry))
	}

	return req.Write(result)
}

func convertMCPServerCatalogEntry(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
	return types.MCPServerCatalogEntry{
		Metadata:                      MetadataFrom(&entry),
		MCPServerCatalogEntryManifest: entry.Spec.Manifest,
	}
}

func (m *MCPHandler) ListServer(req api.Context) error {
	t, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var servers v1.MCPServerList
	if err := req.List(&servers, kclient.MatchingFields{
		"spec.threadName": t.Name,
	}); err != nil {
		return nil
	}

	var result types.MCPServerList
	for _, server := range servers.Items {
		result.Items = append(result.Items, convertMCPServer(server))
	}

	return req.Write(result)
}

func (m *MCPHandler) GetServer(req api.Context) error {
	var (
		server v1.MCPServer
		id     = req.PathValue("id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	result := convertMCPServer(server)
	return req.Write(result)
}

func (m *MCPHandler) DeleteServer(req api.Context) error {
	var (
		server v1.MCPServer
		id     = req.PathValue("id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	if err := req.Delete(&server); err != nil {
		return err
	}

	return req.Write(convertMCPServer(server))
}

func (m *MCPHandler) CreateServer(req api.Context) error {
	var (
		input types.MCPServer
	)

	t, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Read(&input); err != nil {
		return err
	}

	server := v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MCPServerPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.MCPServerSpec{
			Manifest:                  input.MCPServerManifest,
			MCPServerCatalogEntryName: input.CatalogID,
			ThreadName:                t.Name,
			UserID:                    req.User.GetUID(),
		},
	}

	if input.CatalogID != "" {
		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, input.CatalogID); err != nil {
			return err
		}
		server.Spec.Manifest = catalogEntry.Spec.Manifest.Server
		server.Spec.ToolReferenceName = catalogEntry.Spec.ToolReferenceName
	}

	if err := req.Create(&server); err != nil {
		return err
	}

	result := convertMCPServer(server)
	return req.WriteCreated(result)
}

func (m *MCPHandler) UpdateServer(req api.Context) error {
	var (
		id       = req.PathValue("id")
		updated  types.MCPServerManifest
		existing v1.MCPServer
	)

	if err := req.Get(&existing, id); err != nil {
		return err
	}

	if err := req.Read(&updated); err != nil {
		return err
	}

	existing.Spec.Manifest = updated
	if err := req.Update(&existing); err != nil {
		return err
	}

	return req.Write(convertMCPServer(existing))
}

func convertMCPServer(server v1.MCPServer) types.MCPServer {
	return types.MCPServer{
		Metadata:          MetadataFrom(&server),
		MCPServerManifest: server.Spec.Manifest,
		CatalogID:         server.Spec.MCPServerCatalogEntryName,
	}
}
