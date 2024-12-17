package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
)

type ListCredentialsOptions struct {
	AgentID    string
	WorkflowID string
	ThreadID   string
}

func (c *Client) ListCredentials(ctx context.Context, opts ListCredentialsOptions) (result types.CredentialList, err error) {
	path := "/credentials"
	if opts.AgentID != "" {
		path = "/agents/" + opts.AgentID + path
	} else if opts.WorkflowID != "" {
		path = "/workflows/" + opts.WorkflowID + path
	} else if opts.ThreadID != "" {
		path = "/threads/" + opts.ThreadID + path
	}

	_, resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	return
}

type DeleteCredentialsOptions struct {
	AgentID    string
	WorkflowID string
	ThreadID   string
}

func (c *Client) DeleteCredential(ctx context.Context, name string, opts DeleteCredentialsOptions) (err error) {
	path := "/credentials/" + url.PathEscape(name)
	if opts.AgentID != "" {
		path = "/agents/" + opts.AgentID + path
	} else if opts.WorkflowID != "" {
		path = "/workflows/" + opts.WorkflowID + path
	} else if opts.ThreadID != "" {
		path = "/threads/" + opts.ThreadID + path
	}
	_, resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	return
}
