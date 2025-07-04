package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

func (c *Client) CreateThread(projectID string) (*apiclient.Thread, error) {
	resp, err := c.HTTPClient.Post(c.ServerURL+"/api/assistants/a1-obot/projects/"+projectID+"/threads", "application/json", bytes.NewBuffer([]byte{}))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create thread: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var thread apiclient.Thread
	err = json.Unmarshal(body, &thread)
	if err != nil {
		return nil, err
	}

	return &thread, nil
}

func (c *Client) GetProjectThread(projectID, threadID string) (*apiclient.Thread, error) {
	resp, err := c.HTTPClient.Get(c.ServerURL + "/api/assistants/a1-obot/projects/" + projectID + "/threads/" + threadID)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get project thread: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var thread apiclient.Thread
	err = json.Unmarshal(body, &thread)
	if err != nil {
		return nil, err
	}

	return &thread, nil
}

func (c *Client) InvokeProjectThread(projectID, threadID, message string) error {
	resp, err := c.HTTPClient.Post(c.ServerURL+"/api/assistants/a1-obot/projects/"+projectID+"/threads/"+threadID+"/invoke", "application/json", bytes.NewBuffer([]byte(message)))
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to invoke project thread: %s, %s", resp.Status, string(result))
	}

	return nil
}

type Event struct {
	ID   string
	Data apiclient.Progress
}

func (c *Client) GetProjectThreadEventsStream(projectID, threadID string) (<-chan Event, <-chan error) {
	eventCh := make(chan Event)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		url := fmt.Sprintf("%s/api/assistants/a1-obot/projects/%s/threads/%s/events?follow=true&history=true",
			c.ServerURL, projectID, threadID)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			errCh <- err
			return
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("failed to get project thread events: %s", resp.Status)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		var currentEvent, currentID, currentData strings.Builder

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				if currentEvent.Len() > 0 || currentID.Len() > 0 || currentData.Len() > 0 {
					var data apiclient.Progress
					err := json.Unmarshal([]byte(currentData.String()), &data)
					if err != nil {
						errCh <- fmt.Errorf("failed to unmarshal event data: %v", err)
						return
					}

					eventCh <- Event{
						ID:   currentID.String(),
						Data: data,
					}
				}
				currentEvent.Reset()
				currentID.Reset()
				currentData.Reset()
				continue
			}

			switch {
			case strings.HasPrefix(line, "id:"):
				currentID.WriteString(strings.TrimSpace(line[len("id:"):]))
			case strings.HasPrefix(line, "data:"):
				if currentData.Len() > 0 {
					currentData.WriteString("\n")
				}
				currentData.WriteString(strings.TrimSpace(line[len("data:"):]))
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("error reading SSE: %v", err)
		}
	}()

	return eventCh, errCh
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

func (c *Client) GetModels() ([]apiclient.Model, error) {
	resp, err := c.HTTPClient.Get(c.ServerURL + "/api/models")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get models: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var modelList apiclient.ModelList
	err = json.Unmarshal(body, &modelList)
	if err != nil {
		return nil, err
	}

	return modelList.Items, nil
}

func (c *Client) SetUpDefaultModelAlias(modelID, usage string) error {
	payload := map[string]interface{}{
		"model_id": modelID,
		"usage":    usage,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Post(c.ServerURL+"/api/default-model-aliases", "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to set up default model alias: %s", resp.Status)
	}

	return nil
}

func (c *Client) GetAgent(id string) (*apiclient.Agent, error) {
	resp, err := c.HTTPClient.Get(c.ServerURL + "/api/agents/" + id)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get assistant: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var agent apiclient.Agent
	err = json.Unmarshal(body, &agent)
	if err != nil {
		return nil, err
	}

	return &agent, nil
}

func (c *Client) UpdateAgent(id string, agent apiclient.Agent) (*apiclient.Agent, error) {
	payloadBytes, err := json.Marshal(agent)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PUT", c.ServerURL+"/api/agents/"+id, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update assistant: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var updatedAgent apiclient.Agent
	err = json.Unmarshal(body, &updatedAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	return &updatedAgent, nil
}
