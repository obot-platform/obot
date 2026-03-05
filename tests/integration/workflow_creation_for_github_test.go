//go:build integration

package integration

import (
	"os"
	"testing"
)

func TestWorkflowCreationForGithub(t *testing.T) {
	validateRequiredEnvVars(t)

	mcpServerID := ConnectToGithub(t)
	chatMessage := "Use GitHub MCP server to list all the issues from obot-platform/obot in milestone:v0.16.1.\n" +
		"Present the list sorted by Author and provide a count of issues broken down by author.\n" +
		"Present another list sorted by label and provide a count of issues broken down by label."

	CreateWorkflowAndSendPrompt(t, chatMessage)

	// Define expected tool calls
	expectedToolCalls := []ToolCall{
		{
			Method: "search_issues",
			Arguments: []ToolCallArgument{
				{Name: "perPage", Value: "100"},
				{Name: "query", Value: "repo:obot-platform/obot milestone:v0.16.1"},
			},
		},
	}

	// Get actual tool calls and compare with expected
	actualToolCalls := GetToolCalls(t, mcpServerID)
	AssertToolCallsContainSemantic(t, actualToolCalls, expectedToolCalls)
}

func ConnectToGithub(t *testing.T) string {
	t.Helper()
	baseURL := getBaseURL()
	token := getAuthToken(t)
	// Get GitHub token from environment
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		t.Fatal("GITHUB_TOKEN environment variable is required")
	}
	catalogEntryID := GetCatalogIDByName(t, "GitHub", getAdminAuthToken(t))
	// Step 1: Create MCP Server
	t.Log("\n=== Step 2: Creating MCP Server ===")
	mcpServerURL := baseURL + "/api/mcp-servers"

	serverPayload := map[string]interface{}{
		"catalogEntryID": catalogEntryID,
		"manifest": map[string]interface{}{
			"remoteConfig": map[string]interface{}{
				"url": "https://api.githubcopilot.com/mcp/",
			},
		},
	}
	serverResp := makePostRequest(t, mcpServerURL, serverPayload, token, &PostRequestOptions{
		StepName: "Step 2: Create MCP Server",
	})
	serverResponse := serverResp.Body

	idValue, ok := serverResponse["id"]
	if !ok {
		t.Fatalf("Step 2: Create MCP Server: response missing 'id' field: %+v", serverResponse)
	}
	mcpServerID, ok := idValue.(string)
	if !ok {
		t.Fatalf("Step 2: Create MCP Server: 'id' field is not a string (type %T, value %#v) in response: %+v", idValue, idValue, serverResponse)
	}
	t.Logf("✅ Step 2 PASSED: Created MCP server with ID: %s", mcpServerID)

	// Register cleanup to delete the MCP server after test completes
	t.Cleanup(func() {
		cleanupMCPServer(t, mcpServerID)
	})

	// Step 3: Configure MCP Server
	t.Log("\n=== Step 3: Configuring MCP Server ===")
	configureURL := baseURL + "/api/mcp-servers/" + mcpServerID + "/configure"

	// Get GitHub token from environment or use placeholder

	configurePayload := map[string]interface{}{
		"AUTHORIZATION": githubToken,
	}

	configureResp := makePostRequest(t, configureURL, configurePayload, token, &PostRequestOptions{
		StepName: "Step 3: Configure MCP Server",
	})
	configureResponse := configureResp.Body

	t.Logf("✅ Step 3 PASSED: Configured MCP server. Response: %+v", configureResponse)

	// Step 4: Launch MCP Server
	t.Log("\n=== Step 4: Launching MCP Server ===")
	launchURL := baseURL + "/api/mcp-servers/" + mcpServerID + "/launch"

	launchResp := makePostRequest(t, launchURL, map[string]interface{}{}, token, &PostRequestOptions{
		StepName: "Step 4: Launch MCP Server",
	})
	launchResponse := launchResp.Body

	t.Logf("✅ Step 4 PASSED: Launched MCP server. Response: %+v", launchResponse)

	return mcpServerID
}

// cleanupMCPServer deletes the MCP server created during the test
func cleanupMCPServer(t *testing.T, mcpServerID string) {
	t.Helper()
	baseURL := getBaseURL()
	token := getAuthToken(t)

	t.Log("\n=== Cleanup: Deleting MCP Server ===")
	deleteURL := baseURL + "/api/mcp-servers/" + mcpServerID

	_, statusCode := makeDeleteRequestWithStatus(t, deleteURL, token)
	if statusCode >= 200 && statusCode < 300 {
		t.Logf("✅ Cleanup PASSED: Deleted MCP server with ID: %s", mcpServerID)
	} else {
		t.Logf("⚠️ Cleanup WARNING: Failed to delete MCP server with ID: %s, status: %d", mcpServerID, statusCode)
	}
}
