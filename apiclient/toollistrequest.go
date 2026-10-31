package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
)

// CreateToolListRequest starts a request to list the tools of the catalog entry with entryID. The
// user confirms it in the Obot UI at the returned request's URL.
func (c *Client) CreateToolListRequest(ctx context.Context, entryID string) (types.ToolListRequest, error) {
	resp, err := c.postJSON(ctx, "/tool-list-requests", types.ToolListRequestManifest{EntryID: entryID})
	if err != nil {
		return types.ToolListRequest{}, err
	}
	var result types.ToolListRequest
	_, err = toObject(resp, &result)
	return result, err
}

// GetToolListRequest returns the tool list request with id.
func (c *Client) GetToolListRequest(ctx context.Context, id string) (types.ToolListRequest, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/tool-list-requests/"+url.PathEscape(id), nil)
	if err != nil {
		return types.ToolListRequest{}, err
	}
	var result types.ToolListRequest
	_, err = toObject(resp, &result)
	return result, err
}
