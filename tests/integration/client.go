package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	apiclient "github.com/obot-platform/obot/apiclient/types"
)

type Client struct {
	ServerURL  string
	HTTPClient *http.Client
}

func NewClient(serverURL string) *Client {
	return &Client{
		ServerURL:  serverURL,
		HTTPClient: &http.Client{},
	}
}

func (c *Client) CreateProject() (*apiclient.Project, error) {
	payload := []byte(`{}`)

	resp, err := c.HTTPClient.Post(c.ServerURL+"/api/assistants/a1-obot/projects", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create project: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var project apiclient.Project
	err = json.Unmarshal(body, &project)
	if err != nil {
		return nil, err
	}

	return &project, nil
}

func (c *Client) GetProject(id string) (*apiclient.Project, error) {
	resp, err := c.HTTPClient.Get(c.ServerURL + "/api/assistants/a1-obot/projects/" + id)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get project: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var project apiclient.Project
	err = json.Unmarshal(body, &project)
	if err != nil {
		return nil, err
	}

	return &project, nil
}

func (c *Client) GetProjectTask(projectID, taskID string) (*apiclient.Task, error) {
	resp, err := c.HTTPClient.Get(c.ServerURL + "/api/assistants/a1-obot/projects/" + projectID + "/tasks/" + taskID)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get project task: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var task apiclient.Task
	err = json.Unmarshal(body, &task)
	if err != nil {
		return nil, err
	}

	return &task, nil
}

func (c *Client) ConfigureProjectSlack(projectID string, payload map[string]interface{}) (*apiclient.SlackReceiver, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Post(c.ServerURL+"/api/assistants/a1-obot/projects/"+projectID+"/slack", "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to configure project slack: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	slackReceiver := apiclient.SlackReceiver{}
	err = json.Unmarshal(body, &slackReceiver)
	if err != nil {
		return nil, err
	}

	return &slackReceiver, nil
}
