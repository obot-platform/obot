//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getBaseURL returns the base URL from environment variable or defaults to localhost
func getBaseURL() string {
	if url := os.Getenv("OBOT_BASE_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}

// validateRequiredEnvVars validates that all required environment variables are set
func validateRequiredEnvVars(t *testing.T) {
	t.Helper()

	var missing []string

	if os.Getenv("AUTH_TOKEN") == "" {
		missing = append(missing, "AUTH_TOKEN")
	}

	if os.Getenv("ADMIN_AUTH_TOKEN") == "" {
		missing = append(missing, "ADMIN_AUTH_TOKEN")
	}

	if os.Getenv("EVAL_MODEL_API_KEY") == "" {
		missing = append(missing, "EVAL_MODEL_API_KEY")
	}

	if len(missing) > 0 {
		t.Fatalf("Required environment variables not set: %v", missing)
	}
}

// getAuthToken returns the auth token for user from environment variable
func getAuthToken(t *testing.T) string {
	t.Helper()
	token := os.Getenv("AUTH_TOKEN")
	if token == "" {
		t.Fatal("AUTH_TOKEN environment variable is required")
	}
	return token
}

// getAdminAuthToken returns the admin auth token from environment variable
func getAdminAuthToken(t *testing.T) string {
	t.Helper()
	token := os.Getenv("ADMIN_AUTH_TOKEN")
	if token == "" {
		t.Fatal("ADMIN_AUTH_TOKEN environment variable is required")
	}
	return token
}

// getHTTPClient returns an HTTP client with a reasonable timeout for integration tests
func getHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

// redactSensitiveData redacts sensitive fields from a map to prevent credential leakage in logs
func redactSensitiveData(data map[string]interface{}) map[string]interface{} {
	sensitiveKeys := []string{
		"token", "authorization", "AUTHORIZATION", "password", "secret",
		"api_key", "apiKey", "access_token", "accessToken", "refresh_token",
		"GITHUB_TOKEN", "githubToken", "AUTH_TOKEN", "authToken",
	}

	redacted := make(map[string]interface{})
	for k, v := range data {
		// Check if key is sensitive
		isSensitive := false
		keyLower := strings.ToLower(k)
		for _, sensitiveKey := range sensitiveKeys {
			if strings.Contains(keyLower, strings.ToLower(sensitiveKey)) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			redacted[k] = "[REDACTED]"
		} else if nestedMap, ok := v.(map[string]interface{}); ok {
			// Recursively redact nested maps
			redacted[k] = redactSensitiveData(nestedMap)
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// redactJSONString attempts to parse a JSON string and redact sensitive fields
// If parsing fails, it returns the original string truncated for safety
func redactJSONString(jsonStr string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// Not valid JSON or not an object, truncate for safety
		if len(jsonStr) > 200 {
			return jsonStr[:200] + "... [truncated]"
		}
		return jsonStr
	}

	redacted := redactSensitiveData(data)
	redactedJSON, err := json.Marshal(redacted)
	if err != nil {
		return "[failed to redact JSON]"
	}
	return string(redactedJSON)
}

// PostRequestOptions configures optional parameters for makePostRequest
type PostRequestOptions struct {
	Client           *http.Client
	SessionID        string
	ExpectedStatuses []int
	StepName         string
}

// PostResponse contains the response from a POST request
type PostResponse struct {
	Body       map[string]interface{}
	StatusCode int
	Headers    http.Header
}

// makePostRequest makes a POST request with JSON payload and returns body, status, and headers
// By default, it checks for 200 or 201 status codes and fails the test if not matched
func makePostRequest(t *testing.T, url string, payload map[string]interface{}, authToken string, opts *PostRequestOptions) *PostResponse {
	t.Helper()

	// Set default options
	if opts == nil {
		opts = &PostRequestOptions{}
	}
	if opts.Client == nil {
		opts.Client = getHTTPClient()
	}
	if len(opts.ExpectedStatuses) == 0 {
		opts.ExpectedStatuses = []int{http.StatusOK, http.StatusCreated}
	}

	// Marshal payload
	jsonData, err := json.Marshal(payload)
	require.NoError(t, err, "Failed to marshal JSON payload")

	// Log request
	t.Logf("POST %s", url)
	redactedPayload := redactSensitiveData(payload)
	redactedJSON, _ := json.Marshal(redactedPayload)
	t.Logf("Payload: %s", string(redactedJSON))

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	require.NoError(t, err, "Failed to create POST request")

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

	// Add session ID header if provided
	if opts.SessionID != "" {
		req.Header.Set("mcp-session-id", opts.SessionID)
		t.Logf("Using session ID: %s", opts.SessionID)
	}

	// Execute request
	resp, err := opts.Client.Do(req)
	require.NoError(t, err, "Failed to execute POST request to %s", url)
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")

	// Log response
	redactedBody := redactJSONString(string(body))
	t.Logf("Response status: %d, Body: %s", resp.StatusCode, redactedBody)

	// Check status code
	statusOK := false
	for _, expectedStatus := range opts.ExpectedStatuses {
		if resp.StatusCode == expectedStatus {
			statusOK = true
			break
		}
	}
	if !statusOK {
		stepInfo := ""
		if opts.StepName != "" {
			stepInfo = fmt.Sprintf(" (%s)", opts.StepName)
		}
		t.Fatalf("❌ POST request%s FAILED: Expected status %v, got %d", stepInfo, opts.ExpectedStatuses, resp.StatusCode)
	}
	t.Logf("✓ Status code assertion passed: %d", resp.StatusCode)

	// Parse response body
	var result map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			t.Logf("Warning: Failed to parse response as JSON: %v", err)
			result = map[string]interface{}{"raw": string(body)}
		}
	} else {
		result = make(map[string]interface{})
	}

	return &PostResponse{
		Body:       result,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
	}
}

// GetRequestOptions configures optional parameters for makeGetRequest
type GetRequestOptions struct {
	Client           *http.Client
	ExpectedStatuses []int
	StepName         string
}

// GetResponse contains the response from a GET request
type GetResponse struct {
	Body       map[string]interface{}
	StatusCode int
	Headers    http.Header
}

// makeGetRequest makes a GET request and returns body, status, and headers
// By default, it checks for 200 status code and fails the test if not matched
func makeGetRequest(t *testing.T, url string, authToken string, opts *GetRequestOptions) *GetResponse {
	t.Helper()

	// Set default options
	if opts == nil {
		opts = &GetRequestOptions{}
	}
	if opts.Client == nil {
		opts.Client = getHTTPClient()
	}
	if len(opts.ExpectedStatuses) == 0 {
		opts.ExpectedStatuses = []int{http.StatusOK}
	}

	// Log request
	t.Logf("GET %s", url)

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err, "Failed to create GET request")

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

	// Execute request
	resp, err := opts.Client.Do(req)
	require.NoError(t, err, "Failed to execute GET request to %s", url)
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")

	// Log response
	redactedBody := redactJSONString(string(body))
	t.Logf("Response status: %d, Body: %s", resp.StatusCode, redactedBody)

	// Check status code
	statusOK := false
	for _, expectedStatus := range opts.ExpectedStatuses {
		if resp.StatusCode == expectedStatus {
			statusOK = true
			break
		}
	}
	if !statusOK {
		stepInfo := ""
		if opts.StepName != "" {
			stepInfo = fmt.Sprintf(" (%s)", opts.StepName)
		}
		t.Fatalf("❌ GET request%s FAILED: Expected status %v, got %d", stepInfo, opts.ExpectedStatuses, resp.StatusCode)
	}
	t.Logf("✓ Status code assertion passed: %d", resp.StatusCode)

	// Parse response body
	var result map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			t.Logf("Warning: Failed to parse response as JSON: %v", err)
			result = map[string]interface{}{"raw": string(body)}
		}
	} else {
		result = make(map[string]interface{})
	}

	return &GetResponse{
		Body:       result,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
	}
}

// buildURLWithQuery builds a URL with query parameters
func buildURLWithQuery(baseURL string, params ...string) string {
	if len(params)%2 != 0 {
		panic("params must be in key-value pairs")
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("Invalid base URL: %v", err))
	}

	// The params are expected to already contain URL-encoded keys and values
	// (e.g., "resources%2Flist"), so we construct RawQuery manually to avoid
	// double-encoding via url.Values.Encode.
	var b strings.Builder
	if u.RawQuery != "" {
		b.WriteString(u.RawQuery)
	}
	for i := 0; i < len(params); i += 2 {
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(params[i])
		b.WriteByte('=')
		b.WriteString(params[i+1])
	}
	u.RawQuery = b.String()

	return u.String()
}

// buildEventsURL constructs the events API URL from the connectURL and sessionID
// connectURL format: http://localhost:8080/mcp-connect/{agentPath}
// events URL format: http://localhost:8080/mcp-connect/{agentPath}/api/events/{sessionID}
func buildEventsURL(connectURL string, sessionID string) (string, error) {
	u, err := url.Parse(connectURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse connectURL: %w", err)
	}

	// Construct the events path by appending /api/events/{sessionID} to the path
	eventsPath := fmt.Sprintf("%s/api/events/%s", u.Path, sessionID)
	u.Path = eventsPath
	u.RawQuery = "" // Clear any query parameters

	return u.String(), nil
}

// makeStreamingGetRequest makes a GET request and reads the entire streaming response
// It reads Server-Sent Events (SSE) or streaming data until completion or timeout
func makeStreamingGetRequest(t *testing.T, url string, authToken string, sessionID string) (string, int) {
	t.Helper()
	// Create a custom client with no timeout for streaming
	streamClient := &http.Client{
		Timeout: 0, // No timeout for streaming connections
	}

	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err, "Failed to create GET request")

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	// NOTE: Do NOT send mcp-session-id header for event stream - session ID is in the URL path
	t.Logf("Event stream URL contains session ID: %s", sessionID)

	resp, err := streamClient.Do(req)
	require.NoError(t, err, "Failed to execute GET request to %s", url)
	defer resp.Body.Close()

	t.Logf("Response status: %d", resp.StatusCode)
	t.Logf("Content-Type: %s", resp.Header.Get("Content-Type"))

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Event stream connection failed with status %d", resp.StatusCode)
		return "", resp.StatusCode
	}

	// Parse Content-Type to handle parameters like charset
	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Errorf("Failed to parse Content-Type header: %s", contentType)
	} else if mediaType != "text/event-stream" {
		t.Errorf("Unexpected Content-Type: %s (expected text/event-stream)", contentType)
	}

	t.Log("Event stream connection successful, ready to receive data")

	// Read the streaming response with timeout detection using raw byte reading
	var fullResponse bytes.Buffer
	reader := bufio.NewReader(resp.Body)

	// Channel to signal when reading is done
	done := make(chan bool)
	lastDataTime := time.Now()
	idleTimeout := 60 * time.Second  // Timeout if no data for 60 seconds
	readTimeout := 600 * time.Second // Max total time to wait

	startTime := time.Now()
	t.Log("Starting to read stream...")

	go func() {
		totalLinesRead := 0
		for {
			// Read line by line (SSE is line-based)
			line, err := reader.ReadString('\n')

			if len(line) > 0 {
				fullResponse.WriteString(line)
				totalLinesRead++
				lastDataTime = time.Now()

				// Log the received line (trim newline for cleaner output)
				trimmedLine := strings.TrimRight(line, "\r\n")
				if trimmedLine != "" { // Only log non-empty lines
					t.Logf("Received line %d: %s", totalLinesRead, trimmedLine)
				}

				// Check for completion signals
				if strings.Contains(line, "\"done\":true") ||
					strings.Contains(line, "event: done") ||
					strings.Contains(line, "event: history-end") ||
					strings.Contains(line, "event: chat-done") ||
					strings.Contains(line, "event: complete") ||
					strings.Contains(line, "[DONE]") {
					t.Log("Detected completion signal in stream")
					done <- true
					return
				}
			}

			if err != nil {
				if err == io.EOF {
					t.Log("Reached end of stream (EOF)")
				} else if strings.Contains(err.Error(), "use of closed network connection") {
					t.Log("Connection closed (possibly by server or due to idle)")
				} else {
					t.Logf("Read error: %v", err)
				}
				done <- true
				return
			}
		}
	}()

	// Wait for either completion or timeout
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			t.Log("Stream reading completed")
			return fullResponse.String(), resp.StatusCode
		case <-ticker.C:
			elapsed := time.Since(startTime)
			idleTime := time.Since(lastDataTime)

			t.Logf("Status check - elapsed: %v, idle: %v", elapsed, idleTime)

			if elapsed > readTimeout {
				t.Logf("Maximum read timeout reached (%v), stopping", readTimeout)
				return fullResponse.String(), resp.StatusCode
			}
			if idleTime > idleTimeout {
				t.Logf("No data received for %v (idle timeout), assuming stream is complete", idleTimeout)
				return fullResponse.String(), resp.StatusCode
			}
		}
	}
}

// makeDeleteRequestWithStatus is a helper function to make HTTP DELETE requests and return status code
func makeDeleteRequestWithStatus(t *testing.T, url string, token string) (map[string]interface{}, int) {
	t.Helper()

	t.Logf("DELETE %s", url)

	// Create HTTP DELETE request
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Make the HTTP call
	client := getHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	t.Logf("Response Status: %d", resp.StatusCode)

	// Parse response (handle empty body)
	var result map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			t.Logf("Warning: Failed to parse response as JSON: %v", err)
			result = map[string]interface{}{"raw": string(body)}
		}
	} else {
		// Empty response body, return empty map
		result = make(map[string]interface{})
	}

	return result, resp.StatusCode
}

// CreateWorkflowAndSendPrompt tests the complete workflow creation process
// by making sequential API calls and verifying responses
func CreateWorkflowAndSendPrompt(t *testing.T, chatMessage string) {
	t.Helper()
	baseURL := getBaseURL()
	authToken := getAuthToken(t)
	client := getHTTPClient()

	// Step 1: Get projects list and extract ID
	t.Log("Step 1: Getting projects list")
	projectsURL := fmt.Sprintf("%s/api/projectsv2", baseURL)

	projectsResp := makeGetRequest(t, projectsURL, authToken, &GetRequestOptions{
		Client:   client,
		StepName: "Step 1: Get Projects List",
	})

	// Extract items array
	items, ok := projectsResp.Body["items"].([]interface{})
	require.True(t, ok, "Failed to extract 'items' array from projects response")

	var projectID string

	// If no projects found, create a new one
	if len(items) == 0 {
		t.Log("No projects found. Creating a new project...")

		createProjectURL := fmt.Sprintf("%s/api/projectsv2", baseURL)
		createProjectPayload := map[string]interface{}{
			"displayName": "New Project",
		}

		createResp := makePostRequest(t, createProjectURL, createProjectPayload, authToken, &PostRequestOptions{
			Client:   client,
			StepName: "Create New Project",
		})

		projectID, ok = createResp.Body["id"].(string)
		require.True(t, ok, "Failed to extract 'id' from created project")
		require.NotEmpty(t, projectID, "Created project ID is empty")
		t.Logf("Created new project with ID: %s", projectID)
	} else {
		// Get first project
		firstProject, ok := items[0].(map[string]interface{})
		require.True(t, ok, "Failed to parse first project")

		projectID, ok = firstProject["id"].(string)
		require.True(t, ok, "Failed to extract 'id' from first project")
		require.NotEmpty(t, projectID, "Project ID is empty")
		t.Logf("Using existing project ID: %s", projectID)
	}

	// Step 2: Get agents list and extract connect_url
	t.Log("Step 2: Getting agents list")
	agentsURL := fmt.Sprintf("%s/api/projectsv2/%s/agents", baseURL, projectID)

	agentsResp := makeGetRequest(t, agentsURL, authToken, &GetRequestOptions{
		Client:   client,
		StepName: "Step 2: Get Agents List",
	})

	// Extract items array
	agentItems, ok := agentsResp.Body["items"].([]interface{})
	require.True(t, ok, "Failed to extract 'items' array from agents response")

	var connectURL string

	// If no agents found, create a new one
	if len(agentItems) == 0 {
		t.Log("No agents found. Creating a new agent...")

		createAgentURL := fmt.Sprintf("%s/api/projectsv2/%s/agents", baseURL, projectID)
		createAgentPayload := map[string]interface{}{
			"displayName": "New Agent",
		}

		createAgentResp := makePostRequest(t, createAgentURL, createAgentPayload, authToken, &PostRequestOptions{
			Client:   client,
			StepName: "Create New Agent",
		})

		connectURL, ok = createAgentResp.Body["connectURL"].(string)
		require.True(t, ok, "Failed to extract 'connectURL' from created agent")
		require.NotEmpty(t, connectURL, "Created agent connectURL is empty")
		t.Logf("Created new agent with connectURL: %s", connectURL)
	} else {
		// Get first agent
		firstAgent, ok := agentItems[0].(map[string]interface{})
		require.True(t, ok, "Failed to parse first agent")

		t.Logf("First agent details: %+v", firstAgent)

		connectURL, ok = firstAgent["connectURL"].(string)
		require.True(t, ok, "Failed to extract 'connectURL' from first agent")
		require.NotEmpty(t, connectURL, "connectURL is empty")
		t.Logf("Using existing agent connectURL: %s", connectURL)
	}

	// Step 3: POST initialize request
	t.Log("Step 3: Sending initialize request")
	initializePayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "nanobot-ui",
				"version": "0.0.1",
			},
		},
	}
	resp := makePostRequest(t, connectURL, initializePayload, authToken, &PostRequestOptions{
		Client:    client,
		SessionID: "",
		StepName:  "Initialize",
	})

	// Extract mcp-session-id from response headers
	sessionID := resp.Headers.Get("mcp-session-id")
	if sessionID == "" {
		sessionID = resp.Headers.Get("Mcp-Session-Id") // Try with different capitalization
	}
	t.Logf("Extracted session ID: %s", sessionID)
	t.Logf("All initialize response headers: %v", resp.Headers)
	require.NotEmpty(t, sessionID, "Session ID must not be empty")

	// Step 4: POST initialized notification
	t.Log("Step 4: Sending initialized notification")
	initializedPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]interface{}{},
	}
	makePostRequest(t, connectURL, initializedPayload, authToken, &PostRequestOptions{
		Client:           client,
		SessionID:        sessionID,
		ExpectedStatuses: []int{http.StatusOK, http.StatusAccepted},
		StepName:         "Initialized",
	})

	// Step 5: POST resources/list
	t.Log("Step 5: Sending resources/list request")
	resourcesListURL := buildURLWithQuery(connectURL, "method", "resources%2Flist")
	resourcesListPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "resources/list",
		"params":  map[string]interface{}{},
	}
	makePostRequest(t, resourcesListURL, resourcesListPayload, authToken, &PostRequestOptions{
		Client:    client,
		SessionID: sessionID,
		StepName:  "Resources/List",
	})

	// Step 6: POST prompts/list

	t.Log("Step 6: Sending prompts/list request")
	promptsListURL := buildURLWithQuery(connectURL, "method", "prompts%2Flist")
	promptsListPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "prompts/list",
		"params":  map[string]interface{}{},
	}
	makePostRequest(t, promptsListURL, promptsListPayload, authToken, &PostRequestOptions{
		Client:    client,
		SessionID: sessionID,
		StepName:  "Prompts/List",
	})

	// Step 7: POST tools/call - list_agents
	t.Log("Step 7: Sending tools/call - list_agents request")
	listAgentsURL := buildURLWithQuery(connectURL, "method", "tools%2Fcall", "toolcallname", "list_agents")
	listAgentsPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "list_agents",
			"arguments": map[string]interface{}{},
		},
	}
	makePostRequest(t, listAgentsURL, listAgentsPayload, authToken, &PostRequestOptions{
		Client:    client,
		SessionID: sessionID,
		StepName:  "Tools/Call - List Agents",
	})

	// Step 8: POST tools/call - list_chats (first call)
	t.Log("Step 8: Sending tools/call - list_chats request (first)")
	listChatsURL := buildURLWithQuery(connectURL, "method", "tools%2Fcall", "toolcallname", "list_chats")
	listChatsPayload1 := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "list_chats",
			"arguments": map[string]interface{}{},
		},
	}
	makePostRequest(t, listChatsURL, listChatsPayload1, authToken, &PostRequestOptions{
		Client:    client,
		SessionID: sessionID,
		StepName:  "Tools/Call - List Chats (First)",
	})

	// Step 9: Connect to event streaming endpoint BEFORE making any async calls
	t.Log("Step 9: Connecting to event stream in background")
	eventsURL, err := buildEventsURL(connectURL, sessionID)
	require.NoError(t, err, "Failed to build events URL")
	t.Logf("Events URL: %s", eventsURL)

	// Channel to receive the streaming response
	streamResult := make(chan struct {
		response   string
		statusCode int
	})

	// Start streaming in a goroutine
	go func() {
		response, statusCode := makeStreamingGetRequest(t, eventsURL, authToken, sessionID)
		streamResult <- struct {
			response   string
			statusCode int
		}{response: response, statusCode: statusCode}
	}()

	// Give the stream connection time to fully establish before making async calls
	// This ensures the server has the connection ready to receive events
	time.Sleep(2 * time.Second)
	t.Log("Event stream connection established, ready to receive events")

	// Step 10: POST tools/call - chat-with-nanobot (first chat)
	t.Log("Step 10: Sending tools/call - chat-with-nanobot request (first)")

	// Generate a unique progress token for this async operation (NOT the session ID)
	progressToken := uuid.New().String()
	t.Logf("Using progress token: %s", progressToken)

	chatWithNanobotURL := buildURLWithQuery(connectURL, "method", "tools%2Fcall", "toolcallname", "chat-with-nanobot")
	chatWithNanobotPayload1 := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "chat-with-nanobot",
			"arguments": map[string]interface{}{
				"prompt":      "I want to design an AI workflow. Help me get started.",
				"attachments": []interface{}{},
			},
			"_meta": map[string]interface{}{
				"ai.nanobot.async": true,
				"progressToken":    progressToken, // Use unique progress token
			},
		},
	}
	chatResp := makePostRequest(t, chatWithNanobotURL, chatWithNanobotPayload1, authToken, &PostRequestOptions{
		Client:    client,
		SessionID: sessionID,
		StepName:  "Tools/Call - Chat with Nanobot (First)",
	})
	t.Logf("Chat response headers: %v", chatResp.Headers)
	t.Log("Async chat call sent, events should start flowing...")

	//Step 12: POST tools/call - chat-with-nanobot (second chat with chatMessage parameter)
	t.Log("Step 12: Sending tools/call - chat-with-nanobot request (second)")

	// Generate a unique progress token for the second async operation
	progressToken2 := uuid.New().String()
	t.Logf("Using progress token for second chat: %s", progressToken2)

	chatWithNanobotPayload2 := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "chat-with-nanobot",
			"arguments": map[string]interface{}{
				"prompt":      chatMessage,
				"attachments": []interface{}{},
			},
			"_meta": map[string]interface{}{
				"ai.nanobot.async": true,
				"progressToken":    progressToken2,
			},
		},
	}
	makePostRequest(t, chatWithNanobotURL, chatWithNanobotPayload2, authToken, &PostRequestOptions{
		Client:    client,
		SessionID: sessionID,
		StepName:  "Tools/Call - Chat with Nanobot (Second)",
	})

	// Step 13: Wait for and display the complete event stream
	t.Log("Step 13: Waiting for event stream to complete (capturing events from both chats)")
	result := <-streamResult
	if !assert.Equal(t, http.StatusOK, result.statusCode,
		"Expected 200 OK from events endpoint") {
		t.FailNow()
	}
	// Truncate long event stream responses to prevent exposing sensitive data in logs
	responsePreview := result.response
	if len(responsePreview) > 500 {
		responsePreview = responsePreview[:500] + "... [truncated for security]"
	}
	t.Logf("Complete event stream response:\n%s", responsePreview)

	t.Log("Workflow creation test completed successfully!")
}

// ToolCallArgument represents a name-value pair for a tool call argument
type ToolCallArgument struct {
	Name  string
	Value interface{}
}

// ToolCall represents a tool call made during a chat session
type ToolCall struct {
	Method    string
	Arguments []ToolCallArgument
}

// normalizeValue normalizes different value types for consistent comparison
// Converts float64 numbers to int if they're whole numbers, keeps other types as-is
func normalizeValue(value interface{}) interface{} {
	switch v := value.(type) {
	case float64:
		// If it's a whole number, convert to int for cleaner representation
		if v == float64(int(v)) {
			return int(v)
		}
		return v
	case string:
		return v
	case int:
		return v
	case bool:
		return v
	case nil:
		return nil
	default:
		// For complex types (arrays, objects), return as-is
		return v
	}
}

// GetToolCalls retrieves all tool calls made for a given MCP server ID
func GetToolCalls(t *testing.T, mcpServerID string) []ToolCall {
	t.Helper()
	baseURL := getBaseURL()
	token := getAdminAuthToken(t)
	client := getHTTPClient()

	// Step 1: Get audit logs for the MCP server
	t.Logf("Fetching audit logs for MCP server ID: %s", mcpServerID)
	auditLogsURL := fmt.Sprintf("%s/api/mcp-audit-logs?call_type=tools%%2Fcall&mcp_id=%s", baseURL, mcpServerID)

	auditResp := makeGetRequest(t, auditLogsURL, token, &GetRequestOptions{
		Client:   client,
		StepName: "Get Audit Logs",
	})

	// Extract items array
	items, ok := auditResp.Body["items"].([]interface{})
	if !ok {
		t.Logf("No items found in audit logs response")
		return []ToolCall{}
	}

	t.Logf("Found %d audit log entries", len(items))

	// Step 2: Get details for each audit log entry
	var toolCalls []ToolCall

	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			t.Logf("Warning: Skipping non-map item in audit logs")
			continue
		}

		// Extract the ID (numeric)
		var id string
		if idValue, exists := itemMap["id"]; exists {
			// ID is numeric, convert to string
			switch v := idValue.(type) {
			case float64:
				id = fmt.Sprintf("%.0f", v)
			case int:
				id = fmt.Sprintf("%d", v)
			default:
				t.Logf("Warning: Skipping item with unexpected ID type %T: %+v", idValue, itemMap)
				continue
			}
		} else {
			t.Logf("Warning: Skipping item without ID field: %+v", itemMap)
			continue
		}

		// Get detail for this audit log entry
		detailURL := fmt.Sprintf("%s/api/mcp-audit-logs/detail/%s", baseURL, id)

		// Use a separate client for detail requests to avoid test failures
		detailClient := getHTTPClient()
		req, err := http.NewRequest("GET", detailURL, nil)
		if err != nil {
			t.Logf("Warning: Failed to create request for detail %s: %v", id, err)
			continue
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := detailClient.Do(req)
		if err != nil {
			t.Logf("Warning: Failed to get detail for %s: %v", id, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Logf("Warning: Detail endpoint returned status %d for ID %s", resp.StatusCode, id)
			continue
		}

		// Parse detail response
		detailBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Logf("Warning: Failed to read detail response for %s: %v", id, err)
			continue
		}

		t.Logf("Detail response for %s: %s", id, redactJSONString(string(detailBody)))

		var detailResponse map[string]interface{}
		err = json.Unmarshal(detailBody, &detailResponse)
		if err != nil {
			t.Logf("Warning: Failed to parse detail response for %s: %v", id, err)
			continue
		}

		// Extract requestBody
		requestBody, ok := detailResponse["requestBody"].(map[string]interface{})
		if !ok {
			t.Logf("Warning: No requestBody found in detail for %s", id)
			continue
		}

		// Extract params object
		params, ok := requestBody["params"].(map[string]interface{})
		if !ok {
			t.Logf("Warning: No params found in requestBody for %s", id)
			continue
		}

		// Extract tool name from params.name
		toolName, ok := params["name"].(string)
		if !ok {
			t.Logf("Warning: No name found in params for %s", id)
			continue
		}

		// Parse arguments from params.arguments (map of name -> value)
		var arguments []ToolCallArgument
		if argsInterface, exists := params["arguments"]; exists {
			if argsMap, ok := argsInterface.(map[string]interface{}); ok {
				for name, value := range argsMap {
					arguments = append(arguments, ToolCallArgument{
						Name:  name,
						Value: normalizeValue(value),
					})
				}
			} else {
				t.Logf("Warning: arguments is not a map for %s: %T", id, argsInterface)
			}
		}

		toolCall := ToolCall{
			Method:    toolName,
			Arguments: arguments,
		}
		toolCalls = append(toolCalls, toolCall)

		// Log arguments in readable format
		argStrings := make([]string, len(arguments))
		for i, arg := range arguments {
			argStrings[i] = fmt.Sprintf("%s:%v", arg.Name, arg.Value)
		}
		t.Logf("Found tool call: name=%s, arguments=[%s]", toolName, strings.Join(argStrings, ", "))
	}

	t.Logf("Total tool calls found: %d", len(toolCalls))
	return toolCalls
}

// GetCatalogIDByName fetches the catalog entry ID for an MCP entry by its manifest.name
// It queries the /api/all-mcps/entries endpoint and searches for a matching manifest.name
// Returns the "id" field of the matching entry (e.g., "default-asana-877addce")
func GetCatalogIDByName(t *testing.T, name string, authToken string) string {
	t.Helper()

	baseURL := getBaseURL()
	apiURL := fmt.Sprintf("%s/api/all-mcps/entries", baseURL)

	// Make GET request to fetch all MCP entries
	resp := makeGetRequest(t, apiURL, authToken, &GetRequestOptions{
		Client:   getHTTPClient(),
		StepName: "Fetch All MCP Entries",
	})

	// Extract the items array from the response
	items, ok := resp.Body["items"].([]interface{})
	if !ok {
		t.Fatalf("Failed to extract 'items' array from MCP entries response")
	}

	t.Logf("Searching for MCP entry with manifest.name='%s' among %d entries", name, len(items))

	// Search for entry with matching manifest.name
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if manifest exists
		manifest, ok := entry["manifest"].(map[string]interface{})
		if !ok {
			continue
		}

		// Check if manifest.name matches
		manifestName, ok := manifest["name"].(string)
		if !ok {
			continue
		}
		t.Logf("Checking manifest.name: %s", manifestName)

		if manifestName == name {
			// Found the matching entry, extract the id field
			entryID, ok := entry["id"].(string)
			if !ok {
				t.Fatalf("Found entry with manifest.name='%s' but 'id' field is not a string", name)
			}
			t.Logf("Found catalog entry ID '%s' for manifest.name='%s'", entryID, name)
			return entryID
		}
	}

	t.Fatalf("No MCP entry found with manifest.name='%s'", name)
	return ""
}

// AssertToolCalls compares actual tool calls with expected tool calls
// It checks that all expected tool calls are present in the actual list
func AssertToolCalls(t *testing.T, actual []ToolCall, expected []ToolCall) {
	t.Helper()

	// Check if counts match
	if !assert.Equal(t, len(expected), len(actual),
		"Expected %d tool calls, but got %d", len(expected), len(actual)) {
		t.Logf("Actual tool calls: %+v", actual)
		t.Logf("Expected tool calls: %+v", expected)
		return
	}

	// Create a map to track which actual calls have been matched
	matched := make([]bool, len(actual))

	// For each expected call, try to find a matching actual call
	for i, expectedCall := range expected {
		found := false
		for j, actualCall := range actual {
			if matched[j] {
				continue // Already matched this actual call
			}

			// Check if method matches
			if actualCall.Method == expectedCall.Method {
				// Check if arguments match (if expected arguments are specified)
				if expectedCall.Arguments == nil || compareArguments(expectedCall.Arguments, actualCall.Arguments) {
					matched[j] = true
					found = true
					t.Logf("✓ Matched expected call #%d: method=%s", i+1, expectedCall.Method)
					break
				}
			}
		}

		if !found {
			argStr := formatArguments(expectedCall.Arguments)
			t.Errorf("❌ Expected tool call not found: method=%s, arguments=[%s]", expectedCall.Method, argStr)
		}
	}

	// Report any unmatched actual calls
	for j, isMatched := range matched {
		if !isMatched {
			argStr := formatArguments(actual[j].Arguments)
			t.Logf("⚠️  Unexpected tool call: method=%s, arguments=[%s]", actual[j].Method, argStr)
		}
	}
}

// AssertToolCallsContainSemantic checks that all expected tool calls are semantically present
// in the actual list using an LLM evaluation model for conceptual matching.
// This is useful when exact string matching is too strict and you want to verify
// that the intent/concept of the tool call is present, even if naming differs.
//
// Requires EVAL_MODEL_API_KEY environment variable (OpenAI API key by default).
// Optionally set EVAL_MODEL_PROVIDER (openai or anthropic) and EVAL_MODEL_NAME.
func AssertToolCallsContainSemantic(t *testing.T, actual []ToolCall, expected []ToolCall) {
	t.Helper()

	// Get evaluation model configuration
	apiKey := os.Getenv("EVAL_MODEL_API_KEY")
	if apiKey == "" {
		t.Skip("EVAL_MODEL_API_KEY not set, skipping semantic evaluation")
		return
	}

	provider := os.Getenv("EVAL_MODEL_PROVIDER")
	if provider == "" {
		provider = "openai" // default to OpenAI
	}

	modelName := os.Getenv("EVAL_MODEL_NAME")
	if modelName == "" {
		if provider == "openai" {
			modelName = "gpt-4o"
		} else if provider == "anthropic" {
			modelName = "claude-3-5-sonnet-20241022"
		}
	}

	// Format tool calls for evaluation
	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err, "Failed to marshal actual tool calls")

	expectedJSON, err := json.MarshalIndent(expected, "", "  ")
	require.NoError(t, err, "Failed to marshal expected tool calls")

	// Construct evaluation prompt
	prompt := fmt.Sprintf(`You are evaluating whether a set of actual tool calls semantically matches a set of expected tool calls.

IMPORTANT: This is SEMANTIC matching, not exact string matching. Focus on the INTENT and MEANING, not exact names or values.

Expected Tool Calls:
%s

Actual Tool Calls:
%s

For each expected tool call, determine if there is a semantically equivalent call in the actual list.

Two tool calls are SEMANTICALLY EQUIVALENT if they accomplish the same goal, even if:
- Method names are different but mean the same thing (e.g., "get_weather" vs "fetch_weather_data" - both retrieve weather)
- Argument names differ but represent the same concept (e.g., "location" vs "city", "units" vs "temperature_unit")
- Argument values are expressed differently but mean the same (e.g., "San Francisco" vs "SF", "celsius" vs "C")
- Extra arguments are present that don't change the core intent
- Minor variations in data structure that preserve meaning

SPECIAL CASE - CHAINED/REPEATED OPERATIONS:
When the same method is called multiple times (e.g., chained calculations), be EXTRA LENIENT:
- For mathematical operations (add, sum, multiply, etc.), the INTERMEDIATE values may differ based on execution order
- Example: Computing "299+499+9786+3454363" might chain as (299+499)+(result+9786)+(result+3454363)
  OR as (798+3464149)+(9786+3454363)+(299+499) - both are valid!
- What matters: The method is called the expected number of times with values from the problem domain
- The ORDER of operations may differ - this is OK if the overall computation is equivalent
- Focus on: Does the actual call use the same operation type? Are the argument values plausible for the computation?

Examples of SEMANTIC MATCHES:
- "get_weather(location='NYC')" matches "fetch_weather_data(city='New York')" - same intent
- "send_email(to='user@x.com', subject='Hi')" matches "compose_and_send_message(recipient='user@x.com', title='Hi', body='...')" - same core action
- "search_db(query='users')" matches "query_database(search_term='users')" - same operation
- "get-sum(a=299, b=499)" matches "get-sum(a=798, b=3464149)" - both are sum operations, values differ due to chaining order

Examples of NON-MATCHES:
- "get_weather(...)" does NOT match "send_email(...)" - completely different actions
- "delete_user(...)" does NOT match "create_user(...)" - opposite operations
- "get-sum(...)" does NOT match "get-product(...)" - different mathematical operations

MATCHING STRATEGY:
1. If methods are clearly the same operation type (same or synonymous names), mark as matched
2. For chained operations with same method, be lenient about argument values - focus on operation pattern
3. Only reject if the operations are fundamentally different in function/purpose

Your task: For EACH expected tool call, find if ANY actual tool call achieves the same conceptual goal.
Be VERY LENIENT with naming differences and computed values in chained operations.
Be STRICT only with fundamentally different operations.

Respond with a JSON object with this structure:
{
  "matches": [
    {
      "expectedIndex": 0,
      "matched": true/false,
      "actualIndex": 0,
      "reason": "explanation of why they match or don't match"
    }
  ],
  "summary": "overall evaluation summary"
}

Respond ONLY with valid JSON, no other text.`, string(expectedJSON), string(actualJSON))

	// Call evaluation model
	evaluation := callEvaluationModel(t, provider, modelName, apiKey, prompt)

	// Strip markdown code fences if present
	evaluation = strings.TrimSpace(evaluation)
	if strings.HasPrefix(evaluation, "```json") {
		evaluation = strings.TrimPrefix(evaluation, "```json")
		evaluation = strings.TrimSuffix(evaluation, "```")
		evaluation = strings.TrimSpace(evaluation)
	} else if strings.HasPrefix(evaluation, "```") {
		evaluation = strings.TrimPrefix(evaluation, "```")
		evaluation = strings.TrimSuffix(evaluation, "```")
		evaluation = strings.TrimSpace(evaluation)
	}

	// Parse evaluation result
	var result struct {
		Matches []struct {
			ExpectedIndex int    `json:"expectedIndex"`
			Matched       bool   `json:"matched"`
			ActualIndex   int    `json:"actualIndex"`
			Reason        string `json:"reason"`
		} `json:"matches"`
		Summary string `json:"summary"`
	}

	err = json.Unmarshal([]byte(evaluation), &result)
	require.NoError(t, err, "Failed to parse evaluation result: %s", evaluation)

	// Check results and report
	t.Logf("Semantic Evaluation Summary: %s", result.Summary)

	hasFailures := false
	for _, match := range result.Matches {
		if match.ExpectedIndex >= len(expected) {
			t.Errorf("Invalid evaluation: expectedIndex %d out of range", match.ExpectedIndex)
			continue
		}

		expectedCall := expected[match.ExpectedIndex]
		if match.Matched {
			if match.ActualIndex >= 0 && match.ActualIndex < len(actual) {
				actualCall := actual[match.ActualIndex]
				t.Logf("✓ Expected call #%d (method=%s) semantically matched actual call #%d (method=%s): %s",
					match.ExpectedIndex+1, expectedCall.Method,
					match.ActualIndex+1, actualCall.Method,
					match.Reason)
			} else {
				t.Logf("✓ Expected call #%d (method=%s) matched: %s",
					match.ExpectedIndex+1, expectedCall.Method, match.Reason)
			}
		} else {
			hasFailures = true
			argStr := formatArguments(expectedCall.Arguments)
			t.Errorf("❌ Expected tool call not semantically matched: method=%s, arguments=[%s]\n   Reason: %s",
				expectedCall.Method, argStr, match.Reason)
		}
	}

	if hasFailures {
		// Log actual calls for debugging
		t.Logf("All actual tool calls:")
		for idx, call := range actual {
			t.Logf("  [%d] method=%s, arguments=[%s]", idx+1, call.Method, formatArguments(call.Arguments))
		}
	}
}

// callEvaluationModel calls an LLM evaluation model with the given prompt
func callEvaluationModel(t *testing.T, provider, model, apiKey, prompt string) string {
	t.Helper()

	switch provider {
	case "openai":
		return callOpenAI(t, model, apiKey, prompt)
	case "anthropic":
		return callAnthropic(t, model, apiKey, prompt)
	default:
		t.Fatalf("Unsupported evaluation model provider: %s", provider)
		return ""
	}
}

// callOpenAI makes a request to OpenAI's Chat Completions API
func callOpenAI(t *testing.T, model, apiKey, prompt string) string {
	t.Helper()

	requestBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.0, // Use deterministic output for testing
	}

	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err, "Failed to marshal OpenAI request")

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err, "Failed to create OpenAI request")

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to call OpenAI API")
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read OpenAI response")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("OpenAI API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	err = json.Unmarshal(respBody, &openAIResp)
	require.NoError(t, err, "Failed to parse OpenAI response")

	if openAIResp.Error != nil {
		t.Fatalf("OpenAI API returned error: %s", openAIResp.Error.Message)
	}

	require.NotEmpty(t, openAIResp.Choices, "OpenAI response has no choices")
	return openAIResp.Choices[0].Message.Content
}

// callAnthropic makes a request to Anthropic's Messages API
func callAnthropic(t *testing.T, model, apiKey, prompt string) string {
	t.Helper()

	requestBody := map[string]interface{}{
		"model":      model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.0,
	}

	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err, "Failed to marshal Anthropic request")

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err, "Failed to create Anthropic request")

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to call Anthropic API")
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read Anthropic response")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Anthropic API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var anthropicResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	err = json.Unmarshal(respBody, &anthropicResp)
	require.NoError(t, err, "Failed to parse Anthropic response")

	if anthropicResp.Error != nil {
		t.Fatalf("Anthropic API returned error: %s", anthropicResp.Error.Message)
	}

	require.NotEmpty(t, anthropicResp.Content, "Anthropic response has no content")

	// Find the first text content
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			return content.Text
		}
	}

	t.Fatal("No text content found in Anthropic response")
	return ""
}

// compareArguments compares two argument lists
// Returns true if they match, false otherwise
func compareArguments(expected, actual []ToolCallArgument) bool {
	if len(expected) != len(actual) {
		return false
	}

	// Create a map of actual arguments for easier lookup
	actualMap := make(map[string]interface{})
	for _, arg := range actual {
		actualMap[arg.Name] = arg.Value
	}

	// Check that each expected argument exists with the correct value
	for _, expectedArg := range expected {
		actualValue, exists := actualMap[expectedArg.Name]
		if !exists {
			return false
		}

		// Compare values using JSON marshaling for deep comparison
		expectedJSON, err1 := json.Marshal(expectedArg.Value)
		actualJSON, err2 := json.Marshal(actualValue)

		if err1 != nil || err2 != nil || string(expectedJSON) != string(actualJSON) {
			return false
		}
	}

	return true
}

// formatArguments converts arguments to a readable string format
func formatArguments(args []ToolCallArgument) string {
	if len(args) == 0 {
		return ""
	}
	argStrings := make([]string, len(args))
	for i, arg := range args {
		argStrings[i] = fmt.Sprintf("%s:%v", arg.Name, arg.Value)
	}
	return strings.Join(argStrings, ", ")
}

// AssertToolCallsContain checks that all expected tool calls are present in the actual list
// This is more lenient than AssertToolCalls as it allows extra tool calls in the actual list
func AssertToolCallsContain(t *testing.T, actual []ToolCall, expected []ToolCall) {
	t.Helper()

	// For each expected call, try to find it in actual calls
	for i, expectedCall := range expected {
		found := false
		for _, actualCall := range actual {
			// Check if method matches
			if actualCall.Method == expectedCall.Method {
				// Check if arguments match (if expected arguments are specified)
				if expectedCall.Arguments == nil || compareArguments(expectedCall.Arguments, actualCall.Arguments) {
					found = true
					t.Logf("✓ Found expected call #%d: method=%s", i+1, expectedCall.Method)
					break
				}
			}
		}

		if !found {
			argStr := formatArguments(expectedCall.Arguments)
			t.Errorf("❌ Expected tool call not found: method=%s, arguments=[%s]", expectedCall.Method, argStr)
			// Log actual calls in readable format
			t.Logf("Actual tool calls:")
			for idx, call := range actual {
				t.Logf("  [%d] method=%s, arguments=[%s]", idx+1, call.Method, formatArguments(call.Arguments))
			}
		}
	}
}
