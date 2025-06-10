package handlers

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type CatalogHandler struct {
	// toolRefHandler is used to force refresh the catalogs.
	toolRefHandler *toolreference.Handler
}

func NewCatalogHandler(toolRefHandler *toolreference.Handler) *CatalogHandler {
	return &CatalogHandler{
		toolRefHandler: toolRefHandler,
	}
}

// List returns all catalogs.
func (*CatalogHandler) List(req api.Context) error {
	var list v1.CatalogList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list catalogs: %w", err)
	}

	catalogs := make([]types.Catalog, len(list.Items))
	for i, item := range list.Items {
		catalogs[i] = types.Catalog{
			ID:  item.Name,
			URL: item.Spec.URL,
		}
	}

	return req.Write(types.CatalogList{
		Items: catalogs,
	})
}

// Get returns a specific catalog by ID.
func (*CatalogHandler) Get(req api.Context) error {
	var catalog v1.Catalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}
	return req.Write(types.Catalog{
		ID:  catalog.Name,
		URL: catalog.Spec.URL,
	})
}

// Create creates a new catalog.
func (h *CatalogHandler) Create(req api.Context) error {
	urlBytes, err := io.ReadAll(req.Request.Body)
	if err != nil {
		return fmt.Errorf("failed to read catalog data: %w", err)
	}

	// Validate the URL.
	u, err := url.Parse(string(urlBytes))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are supported")
	}

	// Make sure another catalog with the same URL doesn't exist.
	var existing v1.CatalogList
	if err := req.List(&existing, kclient.MatchingFields{
		"spec.url": u.String(),
	}); err != nil {
		return fmt.Errorf("failed to list catalogs: %w", err)
	}
	if len(existing.Items) > 0 {
		return fmt.Errorf("a catalog with the same URL already exists")
	}

	catalog := &v1.Catalog{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.CatalogPrefix,
		},
		Spec: v1.CatalogSpec{
			URL: u.String(),
		},
	}

	if err := req.Create(catalog); err != nil {
		return fmt.Errorf("failed to create catalog: %w", err)
	}

	go func() {
		if err := h.toolRefHandler.ForceRefreshMCPCatalogs(context.Background(), req.Storage); err != nil {
			log.Errorf("Failed to force refresh MCP catalogs: %v", err)
		}
	}()

	return req.Write(types.Catalog{
		ID:  catalog.Name,
		URL: catalog.Spec.URL,
	})
}

// Delete deletes a catalog.
func (h *CatalogHandler) Delete(req api.Context) error {
	var catalog v1.Catalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	if err := req.Delete(&catalog); err != nil {
		return fmt.Errorf("failed to delete catalog: %w", err)
	}

	go func() {
		if err := h.toolRefHandler.ForceRefreshMCPCatalogs(context.Background(), req.Storage); err != nil {
			log.Errorf("Failed to force refresh MCP catalogs: %v", err)
		}
	}()

	return nil
}
