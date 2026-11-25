package apiclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
)

// Agent identity IDs are decimal strings and API key IDs are numbers, both of which are path-escaped anyway so that
// a malformed value cannot change the route. Failed requests return *types.ErrHTTP, which keeps the status code.

func agentPath(agentID string, rest ...string) string {
	path := "/agents/" + url.PathEscape(agentID)
	for _, r := range rest {
		path += "/" + url.PathEscape(r)
	}
	return path
}

// doJSON sends body (if non-nil) as JSON and decodes the response into out (if non-nil). Responses with no content,
// such as the 204 of a delete, decode nothing.
func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	var (
		reader  io.Reader
		headers []string
	)
	if body != nil {
		data, err := marshalJSON(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
		headers = []string{"Content-Type", "application/json"}
	}

	resp, err := c.doRequest(ctx, method, path, reader, headers...)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent || out == nil {
		_ = resp.Body.Close()
		return nil
	}
	_, err = toObject(resp, out)
	return err
}

// doJSONResult is doJSON for a response that decodes into a new T.
func doJSONResult[T any](ctx context.Context, c *Client, method, path string, body any) (*T, error) {
	var result T
	if err := c.doJSON(ctx, method, path, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListAgentIdentities(ctx context.Context, includeDeleted bool) (types.AgentIdentityList, error) {
	path := "/agents"
	if includeDeleted {
		path += "?" + url.Values{"includeDeleted": {"true"}}.Encode()
	}
	var result types.AgentIdentityList
	err := c.doJSON(ctx, http.MethodGet, path, nil, &result)
	return result, err
}

func (c *Client) CreateAgentIdentity(ctx context.Context, manifest types.AgentIdentityManifest) (*types.AgentIdentity, error) {
	return doJSONResult[types.AgentIdentity](ctx, c, http.MethodPost, "/agents", manifest)
}

func (c *Client) GetAgentIdentity(ctx context.Context, agentID string) (*types.AgentIdentity, error) {
	return doJSONResult[types.AgentIdentity](ctx, c, http.MethodGet, agentPath(agentID), nil)
}

// UpdateAgentIdentity replaces the agent's metadata with manifest; fields left out are cleared, not kept.
func (c *Client) UpdateAgentIdentity(ctx context.Context, agentID string, manifest types.AgentIdentityManifest) (*types.AgentIdentity, error) {
	return doJSONResult[types.AgentIdentity](ctx, c, http.MethodPut, agentPath(agentID), manifest)
}

func (c *Client) DisableAgentIdentity(ctx context.Context, agentID, reason string) (*types.AgentIdentity, error) {
	return doJSONResult[types.AgentIdentity](ctx, c, http.MethodPost, agentPath(agentID, "disable"), types.AgentIdentityDisableRequest{Reason: reason})
}

func (c *Client) EnableAgentIdentity(ctx context.Context, agentID string) (*types.AgentIdentity, error) {
	return doJSONResult[types.AgentIdentity](ctx, c, http.MethodPost, agentPath(agentID, "enable"), nil)
}

// DeleteAgentIdentity deletes the agent and revokes all of its keys.
func (c *Client) DeleteAgentIdentity(ctx context.Context, agentID string) error {
	return c.doJSON(ctx, http.MethodDelete, agentPath(agentID), nil, nil)
}

func (c *Client) ListAgentAPIKeys(ctx context.Context, agentID string, showRevoked bool) (types.AgentAPIKeyList, error) {
	path := agentPath(agentID, "api-keys")
	if showRevoked {
		path += "?" + url.Values{"show_revoked": {"true"}}.Encode()
	}
	var result types.AgentAPIKeyList
	err := c.doJSON(ctx, http.MethodGet, path, nil, &result)
	return result, err
}

func (c *Client) CreateAgentAPIKey(ctx context.Context, agentID string, req types.AgentAPIKeyCreateRequest) (*types.AgentAPIKeyCreateResponse, error) {
	return doJSONResult[types.AgentAPIKeyCreateResponse](ctx, c, http.MethodPost, agentPath(agentID, "api-keys"), req)
}

func (c *Client) GetAgentAPIKey(ctx context.Context, agentID, keyID string) (*types.AgentAPIKey, error) {
	return doJSONResult[types.AgentAPIKey](ctx, c, http.MethodGet, agentPath(agentID, "api-keys", keyID), nil)
}

func (c *Client) RevokeAgentAPIKey(ctx context.Context, agentID, keyID string) error {
	return c.doJSON(ctx, http.MethodDelete, agentPath(agentID, "api-keys", keyID), nil, nil)
}
