package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
)

// ListOrphanedVMCPs returns catalog YAML for orphaned vMCPs: vMCPs migrated
// from composite catalog entries synced from sourceURL that catalog sync has
// not adopted yet.
func (c *Client) ListOrphanedVMCPs(ctx context.Context, catalogID, sourceURL string) (types.OrphanedVMCPCatalogItemList, error) {
	path := fmt.Sprintf("/mcp-catalogs/%s/orphaned-vmcps?%s", url.PathEscape(catalogID), url.Values{"sourceURL": {sourceURL}}.Encode())
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return types.OrphanedVMCPCatalogItemList{}, err
	}

	var result types.OrphanedVMCPCatalogItemList
	_, err = toObject(resp, &result)
	return result, err
}
