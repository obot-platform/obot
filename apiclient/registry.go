package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

type ListRegistryServersOptions struct {
	Search string
	Cursor string
	Limit  int
}

func (c *Client) ListRegistryServers(ctx context.Context, opts ListRegistryServersOptions) (types.RegistryServerList, error) {
	values := url.Values{}
	if opts.Search != "" {
		values.Set("search", opts.Search)
	}
	if opts.Cursor != "" {
		values.Set("cursor", opts.Cursor)
	}
	if opts.Limit > 0 {
		values.Set("limit", strconv.Itoa(opts.Limit))
	}

	path := "/v0.1/servers"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}

	_, resp, err := c.doRequestWithBaseURL(ctx, http.MethodGet, appBaseURLForAPIBaseURL(c.BaseURL), path, nil)
	if err != nil {
		return types.RegistryServerList{}, err
	}

	var result types.RegistryServerList
	_, err = toObject(resp, &result)
	return result, err
}

func appBaseURLForAPIBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return strings.TrimSuffix(baseURL, "/api")
}
