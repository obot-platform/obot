package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) ListSkills(ctx context.Context, query string, limit int) (types.SkillList, error) {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}

	path := "/skills"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}

	_, resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return types.SkillList{}, err
	}

	var result types.SkillList
	_, err = toObject(resp, &result)
	return result, err
}

func (c *Client) GetSkill(ctx context.Context, id string) (types.Skill, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/skills/%s", url.PathEscape(id)), nil)
	if err != nil {
		return types.Skill{}, err
	}

	obj := types.Skill{}
	_, err = toObject(resp, &obj)
	return obj, err
}

func (c *Client) DownloadSkill(ctx context.Context, id string) ([]byte, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/skills/%s/download", url.PathEscape(id)), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
