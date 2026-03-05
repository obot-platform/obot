//go:build integration

package integration

import (
	"testing"
)

func TestWorkflowCreationForEverythingMCPServer(t *testing.T) {
	validateRequiredEnvVars(t)

	mcpServerID := CreateMCPCatalogAndConnect(t)
	chatMessage := "compute 299+499+9786+3454363 using TestSingleServer mcpserver."

	CreateWorkflowAndSendPrompt(t, chatMessage)

	// Define expected tool calls
	expectedToolCalls := []ToolCall{
		{
			Method: "get-sum",
			Arguments: []ToolCallArgument{
				{Name: "a", Value: 299},
				{Name: "b", Value: 499},
			},
		},
		{
			Method: "get-sum",
			Arguments: []ToolCallArgument{
				{Name: "a", Value: 798},
				{Name: "b", Value: 9786},
			},
		},
		{
			Method: "get-sum",
			Arguments: []ToolCallArgument{
				{Name: "a", Value: 10584},
				{Name: "b", Value: 3454363},
			},
		},
	}

	// Get actual tool calls and compare with expected
	actualToolCalls := GetToolCalls(t, mcpServerID)
	AssertToolCallsContainSemantic(t, actualToolCalls, expectedToolCalls)
}

func CreateMCPCatalogAndConnect(t *testing.T) string {
	t.Helper()
	baseURL := getBaseURL()
	adminToken := getAdminAuthToken(t)
	token := getAuthToken(t)
	// Step 1: Create MCP Catalog Entry
	t.Log("\n=== Step 1: Creating MCP Catalog Entry ===")
	catalogEntryURL := baseURL + "/api/mcp-catalogs/default/entries"

	catalogPayload := map[string]interface{}{
		"name":        "TestSingleServer",
		"description": "",
		"icon":        "",
		"env":         []interface{}{},
		"runtime":     "npx",
		"metadata": map[string]interface{}{
			"categories": "",
		},
		"npxConfig": map[string]interface{}{
			"package": "@modelcontextprotocol/server-everything",
			"args":    []interface{}{},
		},
	}

	catalogResp := makePostRequest(t, catalogEntryURL, catalogPayload, adminToken, &PostRequestOptions{
		StepName: "Step 1: Create MCP Catalog Entry",
	})
	catalogResponse := catalogResp.Body

	idValue, ok := catalogResponse["id"]
	if !ok {
		t.Fatalf("Step 1: Create MCP Catalog Entry: response missing 'id' field: %+v", catalogResponse)
	}
	catalogEntryID, ok := idValue.(string)
	if !ok {
		t.Fatalf("Step 1: Create MCP Catalog Entry: 'id' field is not a string (type %T, value %#v) in response: %+v", idValue, idValue, catalogResponse)
	}
	t.Logf("✅ Step 1 PASSED: Created catalog entry with ID: %s", catalogEntryID)

	// Register cleanup to delete the catalog entry after test completes
	t.Cleanup(func() {
		cleanupMCPCatalogEntry(t, catalogEntryID)
	})

	// Step 2: Create MCP Server
	t.Log("\n=== Step 2: Creating MCP Server ===")
	mcpServerURL := baseURL + "/api/mcp-servers"

	serverPayload := map[string]interface{}{
		"catalogEntryID": catalogEntryID,
		"manifest":       map[string]interface{}{},
	}

	serverResp := makePostRequest(t, mcpServerURL, serverPayload, token, &PostRequestOptions{
		StepName: "Step 2: Create MCP Server",
	})
	serverResponse := serverResp.Body

	idValue, ok = serverResponse["id"]
	if !ok {
		t.Fatalf("Step 2: Create MCP Server: response missing 'id' field: %+v", serverResponse)
	}
	mcpServerID, ok := idValue.(string)
	if !ok {
		t.Fatalf("Step 2: Create MCP Server: 'id' field is not a string (type %T, value %#v) in response: %+v", idValue, idValue, serverResponse)
	}
	t.Logf("✅ Step 2 PASSED: Created MCP server with ID: %s", mcpServerID)

	// Step 3: Configure MCP Server
	t.Log("\n=== Step 3: Configuring MCP Server ===")
	configureURL := baseURL + "/api/mcp-servers/" + mcpServerID + "/configure"

	configureResp := makePostRequest(t, configureURL, map[string]interface{}{}, token, &PostRequestOptions{
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

// cleanupMCPCatalogEntry deletes the MCP catalog entry created during the test
func cleanupMCPCatalogEntry(t *testing.T, catalogEntryID string) {
	t.Helper()
	baseURL := getBaseURL()
	token := getAdminAuthToken(t)

	t.Log("\n=== Cleanup: Deleting MCP Catalog Entry ===")
	deleteURL := baseURL + "/api/mcp-catalogs/default/entries/" + catalogEntryID

	_, statusCode := makeDeleteRequestWithStatus(t, deleteURL, token)
	if statusCode >= 200 && statusCode < 300 {
		t.Logf("✅ Cleanup PASSED: Deleted catalog entry with ID: %s", catalogEntryID)
	} else {
		t.Logf("⚠️ Cleanup WARNING: Failed to delete catalog entry with ID: %s, status: %d", catalogEntryID, statusCode)
	}
}
