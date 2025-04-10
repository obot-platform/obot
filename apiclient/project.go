package apiclient

import (
	"context"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

type ListProjectsOptions struct {
	AgentID string
	All     bool
}

func (c *Client) GetProject(ctx context.Context, id string) (*types.Project, error) {
	// The API endpoint expects the ID to be passed directly without any transformation
	// The server code will handle the ID transformation from project ID to thread ID
	_, resp, err := c.doRequest(ctx, http.MethodGet, "/projects/"+id, nil)
	if err != nil {
		return nil, err
	}
	var result types.Project
	_, err = toObject(resp, &result)
	return &result, err
}

func (c *Client) ListProjects(ctx context.Context, opts ListProjectsOptions) (types.ProjectList, error) {
	url := "/projects"
	if opts.All {
		url += "?all=true"
	}
	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.ProjectList{}, err
	}
	var result types.ProjectList
	_, err = toObject(resp, &result)
	return result, err
}

func (c *Client) ListProjectShares(ctx context.Context) (types.ProjectShareList, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, "/shares", nil)
	if err != nil {
		return types.ProjectShareList{}, err
	}
	var result types.ProjectShareList
	_, err = toObject(resp, &result)
	return result, err
}

// CreateProject creates a new project (obot) for the specified assistant
func (c *Client) CreateProject(ctx context.Context, assistantID string, manifest types.ProjectManifest) (*types.Project, error) {
	url := "/assistants/" + assistantID + "/projects"
	_, resp, err := c.postJSON(ctx, url, manifest)
	if err != nil {
		return nil, err
	}
	var result types.Project
	_, err = toObject(resp, &result)
	return &result, err
}

// CreateProjectFromShare creates a new project from a shared project template
// This handles the POST /api/shares/{share_public_id} API endpoint
func (c *Client) CreateProjectFromShare(ctx context.Context, shareID string, create bool) (*types.Project, error) {
	url := "/shares/" + shareID
	if create {
		url += "?create"
	}
	_, resp, err := c.postJSON(ctx, url, struct{}{})
	if err != nil {
		return nil, err
	}
	var result types.Project
	_, err = toObject(resp, &result)
	return &result, err
}

// CopyProject creates a copy of an existing project
// This handles the POST /api/assistants/{assistant_id}/projects/{project_id}/copy API endpoint
func (c *Client) CopyProject(ctx context.Context, assistantID, projectID string) (*types.Project, error) {
	url := "/assistants/" + assistantID + "/projects/" + projectID + "/copy"
	_, resp, err := c.postJSON(ctx, url, struct{}{})
	if err != nil {
		return nil, err
	}
	var result types.Project
	_, err = toObject(resp, &result)
	return &result, err
}

// UpdateProject updates an existing project with new information
func (c *Client) UpdateProject(ctx context.Context, project *types.Project) (*types.Project, error) {
	url := "/assistants/" + project.AssistantID + "/projects/" + project.ID
	_, resp, err := c.putJSON(ctx, url, project)
	if err != nil {
		return nil, err
	}
	var result types.Project
	_, err = toObject(resp, &result)
	return &result, err
}

// DeleteProject deletes an existing project (obot)
func (c *Client) DeleteProject(ctx context.Context, assistantID, projectID string) error {
	url := "/assistants/" + assistantID + "/projects/" + projectID
	_, _, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	return err
}
