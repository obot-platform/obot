package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) GetWebhook(ctx context.Context, id string) (result *types.Webhook, _ error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/webhooks/%s", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Webhook{})
}

func (c *Client) ListWebhooks(ctx context.Context) (result types.WebhookList, _ error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Time.Before(result.Items[j].Metadata.Created.Time)
		})
	}()

	_, resp, err := c.doRequest(ctx, http.MethodGet, "/webhooks", nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	_, resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/webhooks/%s", id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
