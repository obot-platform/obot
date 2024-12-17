package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) GetEmailReceiver(ctx context.Context, id string) (*types.EmailReceiver, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/email-receivers/"+id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.EmailReceiver{})
}

func (c *Client) ListEmailReceivers(ctx context.Context) (result types.EmailReceiverList, _ error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Time.Before(result.Items[j].Metadata.Created.Time)
		})
	}()

	_, resp, err := c.doRequest(ctx, http.MethodGet, "/email-receivers", nil)
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

func (c *Client) DeleteEmailReceiver(ctx context.Context, id string) error {
	_, resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/email-receivers/"+id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
