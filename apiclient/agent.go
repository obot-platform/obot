package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/acorn-io/acorn/apiclient/types"
)

func (c *Client) UpdateAgent(ctx context.Context, id string, manifest types.AgentManifest) (*types.Agent, error) {
	_, resp, err := c.putJSON(ctx, fmt.Sprintf("/agents/%s", id), manifest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Agent{})
}

func (c *Client) GetAgent(ctx context.Context, id string) (*types.Agent, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/agents/"+id), nil)
	if err != nil {
		return nil, err
	}

	return toObject(resp, &types.Agent{})
}

func (c *Client) CreateAgent(ctx context.Context, agent types.AgentManifest) (*types.Agent, error) {
	_, resp, err := c.postJSON(ctx, fmt.Sprintf("/agents"), agent)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Agent{})
}

type ListAgentsOptions struct {
	Alias string
}

func (c *Client) ListAgents(ctx context.Context, opts ListAgentsOptions) (result types.AgentList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Time.Before(result.Items[j].Metadata.Created.Time)
		})
	}()

	_, resp, err := c.doRequest(ctx, http.MethodGet, "/agents", nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	if err != nil {
		return result, err
	}

	if opts.Alias != "" {
		var filtered types.AgentList
		for _, agent := range result.Items {
			if agent.Alias == opts.Alias && agent.AliasAssigned != nil && *agent.AliasAssigned {
				filtered.Items = append(filtered.Items, agent)
			}
		}
		result = filtered
	}

	return
}

func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	_, resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/agents/"+id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
