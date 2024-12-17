package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) getGeneric(ctx context.Context, typeName, ref string) (string, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, "/"+typeName+"s/"+ref, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var outputMap = map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&outputMap); err != nil {
		return "", err
	}
	ref, _ = outputMap["id"].(string)
	return ref, nil
}

func (c *Client) createGeneric(ctx context.Context, typeName string, inputMap any) (string, error) {
	_, resp, err := c.postJSON(ctx, "/"+typeName+"s", inputMap)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var outputMap = map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&outputMap); err != nil {
		return "", err
	}
	id, _ := outputMap["id"].(string)
	return id, nil
}

func (c *Client) Create(ctx context.Context, typeName string, body []byte) (string, error) {
	var (
		inputMap = map[string]any{}
	)

	if err := json.Unmarshal(body, &inputMap); err != nil {
		return "", err
	}

	alias, _ := inputMap["alias"].(string)
	if alias == "" {
		return c.createGeneric(ctx, typeName, inputMap)
	}

	id, err := c.getGeneric(ctx, typeName, alias)
	if types.IsNotFound(err) {
		return c.createGeneric(ctx, typeName, inputMap)
	} else if err != nil {
		return "", err
	}

	_, resp, err := c.putJSON(ctx, "/"+typeName+"s/"+id, inputMap)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return id, nil
}
