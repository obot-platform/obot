package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	mockDataUserCount        = 12
	mockDataDeviceCount      = 30
	mockDataDeviceScanCount  = 60
	mockDataLocalAuditCount  = 400
	mockDataMCPAuditCount    = 350
	mockDataLLMAuditCount    = 250
	mockDataEnforcementCount = 300
)

type MockDataSummary struct {
	RunID                string    `json:"runID"`
	WindowStart          time.Time `json:"windowStart"`
	WindowEnd            time.Time `json:"windowEnd"`
	Users                int       `json:"users"`
	Devices              int       `json:"devices"`
	DeviceScans          int       `json:"deviceScans"`
	LocalAgentAuditLogs  int       `json:"localAgentAuditLogs"`
	MCPAuditLogs         int       `json:"mcpAuditLogs"`
	LLMAuditLogs         int       `json:"llmAuditLogs"`
	EnforcementDecisions int       `json:"enforcementDecisions"`
}

// GenerateMockData appends one coherent demo dataset.
func (c *Client) GenerateMockData(ctx context.Context, userLimit UserLimit) (*MockDataSummary, error) {
	runID, err := newMockDataRunID()
	if err != nil {
		return nil, err
	}
	return c.generateMockData(ctx, time.Now().UTC(), runID, userLimit)
}

func (c *Client) generateMockData(ctx context.Context, now time.Time, runID string, userLimit UserLimit) (*MockDataSummary, error) {
	now = now.UTC()
	windowStart := now.Add(-30 * 24 * time.Hour)
	summary := &MockDataSummary{
		RunID:                runID,
		WindowStart:          windowStart,
		WindowEnd:            now,
		Users:                mockDataUserCount,
		Devices:              mockDataDeviceCount,
		DeviceScans:          mockDataDeviceScanCount,
		LocalAgentAuditLogs:  mockDataLocalAuditCount,
		MCPAuditLogs:         mockDataMCPAuditCount,
		LLMAuditLogs:         mockDataLLMAuditCount,
		EnforcementDecisions: mockDataEnforcementCount,
	}

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureUserCapacity(tx, userLimit, mockDataUserCount); err != nil {
			return err
		}

		users, userIDs, err := c.createMockDataUsers(ctx, tx, now, runID)
		if err != nil {
			return err
		}

		deviceIDs := mockDataDeviceIDs(runID)
		scans := buildMockDeviceScans(windowStart, now, userIDs, deviceIDs)
		if err := tx.CreateInBatches(&scans, 10).Error; err != nil {
			return fmt.Errorf("failed to insert mock device scans: %w", err)
		}

		localLogs := buildMockLocalAgentAuditLogs(windowStart, now, runID, users, userIDs, deviceIDs)
		if err := c.prepareMockMCPAuditLogs(ctx, localLogs); err != nil {
			return err
		}
		if err := tx.CreateInBatches(&localLogs, 100).Error; err != nil {
			return fmt.Errorf("failed to insert mock local-agent audit logs: %w", err)
		}

		mcpLogs := buildMockMCPAuditLogs(windowStart, now, runID, userIDs)
		if err := c.prepareMockMCPAuditLogs(ctx, mcpLogs); err != nil {
			return err
		}
		if err := tx.CreateInBatches(&mcpLogs, 100).Error; err != nil {
			return fmt.Errorf("failed to insert mock MCP audit logs: %w", err)
		}

		llmLogs := buildMockLLMAuditLogs(windowStart, now, runID, userIDs)
		for i := range llmLogs {
			if err := c.encryptLLMAuditLog(ctx, &llmLogs[i]); err != nil {
				return fmt.Errorf("failed to encrypt mock LLM audit log: %w", err)
			}
		}
		if err := tx.CreateInBatches(&llmLogs, 100).Error; err != nil {
			return fmt.Errorf("failed to insert mock LLM audit logs: %w", err)
		}

		enforcementDecisions := buildMockEnforcementDecisions(windowStart, now, deviceIDs)
		if err := tx.CreateInBatches(&enforcementDecisions, 100).Error; err != nil {
			return fmt.Errorf("failed to insert mock enforcement decisions: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return summary, nil
}

func newMockDataRunID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to create mock-data run ID: %w", err)
	}
	return "demo-" + hex.EncodeToString(b), nil
}

func (c *Client) createMockDataUsers(ctx context.Context, tx *gorm.DB, now time.Time, runID string) ([]types.User, []string, error) {
	names := []string{
		"Demo Engineer 01",
		"Demo Engineer 02",
		"Demo Engineer 03",
		"Demo Security Analyst 01",
		"Demo Security Analyst 02",
		"Demo Data Analyst 01",
		"Demo Data Analyst 02",
		"Demo Product Manager 01",
		"Demo Support Specialist 01",
		"Demo Researcher 01",
		"Demo Operations Lead 01",
		"Demo Contractor 01",
	}
	users := make([]types.User, 0, len(names))
	userIDs := make([]string, 0, len(names))
	for i, displayName := range names {
		username := fmt.Sprintf("%s-user-%02d", runID, i+1)
		email := fmt.Sprintf("%s-user-%02d@example.invalid", runID, i+1)
		user := types.User{
			CreatedAt:      now,
			DisplayName:    displayName,
			Username:       username,
			HashedUsername: hash.String(username),
			Email:          email,
			HashedEmail:    hash.String(email),
			Role:           apitypes.RoleBasic,
			Timezone:       "America/Los_Angeles",
		}
		stored := user
		if err := c.encryptUser(ctx, &stored); err != nil {
			return nil, nil, fmt.Errorf("failed to encrypt mock user: %w", err)
		}
		if err := tx.Create(&stored).Error; err != nil {
			return nil, nil, fmt.Errorf("failed to insert mock user: %w", err)
		}
		user.ID = stored.ID
		users = append(users, user)
		userIDs = append(userIDs, fmt.Sprint(stored.ID))
	}
	return users, userIDs, nil
}

func (c *Client) prepareMockMCPAuditLogs(ctx context.Context, logs []types.MCPAuditLog) error {
	for i := range logs {
		logs[i].CreatedAt = logs[i].CreatedAt.UTC()
		if logs[i].SourceType == apitypes.AuditLogSourceTypeMCP {
			logs[i].NormalizeMCPFields()
		}
		if err := logs[i].ValidateSourceFields(); err != nil {
			return fmt.Errorf("invalid mock audit log at index %d: %w", i, err)
		}
		if err := c.encryptMCPAuditLog(ctx, &logs[i]); err != nil {
			return fmt.Errorf("failed to encrypt mock audit log at index %d: %w", i, err)
		}
	}
	return nil
}

func mockDataDeviceIDs(runID string) []string {
	ids := make([]string, mockDataDeviceCount)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s-device-%02d", runID, i+1)
	}
	return ids
}

func buildMockDeviceScans(start, end time.Time, userIDs, deviceIDs []string) []types.DeviceScan {
	operatingSystems := []struct {
		os, arch string
	}{
		{os: "darwin", arch: "arm64"},
		{os: "darwin", arch: "amd64"},
		{os: "linux", arch: "amd64"},
		{os: "windows", arch: "amd64"},
	}
	servers := []struct {
		name, transport, command, url string
	}{
		{name: "GitHub", transport: "stdio", command: "npx"},
		{name: "PostgreSQL", transport: "stdio", command: "npx"},
		{name: "Slack", transport: "http", url: "https://mcp.example.invalid/slack"},
		{name: "Filesystem", transport: "stdio", command: "npx"},
	}
	clients := []string{"Claude Code", "Codex", "Cursor", "VS Code"}
	skills := []string{"incident-response", "database-review", "release-notes", "security-audit"}

	scans := make([]types.DeviceScan, 0, mockDataDeviceScanCount)
	for deviceIndex, deviceID := range deviceIDs {
		platform := operatingSystems[deviceIndex%len(operatingSystems)]
		for scanIndex := range 2 {
			sequence := deviceIndex*2 + scanIndex
			scannedAt := mockEventTime(start, end, sequence, mockDataDeviceScanCount)
			client := clients[(deviceIndex+scanIndex)%len(clients)]
			server := servers[(deviceIndex+scanIndex)%len(servers)]
			serverHash := sha256.Sum256([]byte(server.name + server.transport + server.command + server.url))
			scope := "global"
			projectPath := ""
			if sequence%3 == 0 {
				scope = "project"
				projectPath = "/workspaces/customer-portal"
			}
			submittedBy := ""
			if sequence%3 != 0 {
				submittedBy = userIDs[deviceIndex%len(userIDs)]
			}
			scan := types.DeviceScan{
				CreatedAt:      scannedAt,
				SubmittedBy:    submittedBy,
				DeviceID:       deviceID,
				Hostname:       fmt.Sprintf("demo-%s-%02d", platform.os, deviceIndex+1),
				Username:       fmt.Sprintf("demo-user-%02d", deviceIndex%len(userIDs)+1),
				OS:             platform.os,
				Arch:           platform.arch,
				ScannerVersion: "1.4.0",
				ScannedAt:      scannedAt,
				Clients: []types.DeviceScanClient{{
					CreatedAt:     scannedAt,
					Name:          client,
					Version:       fmt.Sprintf("1.%d.%d", deviceIndex%8, scanIndex+1),
					BinaryPath:    "/usr/local/bin/agent",
					ConfigPath:    "~/.config/agent/config.json",
					HasMCPServers: true,
					HasSkills:     true,
					HasPlugins:    scanIndex == 1,
				}},
				MCPServers: []types.DeviceScanMCPServer{{
					CreatedAt:   scannedAt,
					Client:      client,
					Scope:       scope,
					ProjectPath: projectPath,
					File:        "~/.config/agent/mcp.json",
					Name:        server.name,
					Transport:   server.transport,
					Command:     server.command,
					Args:        datatypes.JSONSlice[string]{"--yes", "@modelcontextprotocol/server"},
					URL:         server.url,
					EnvKeys:     datatypes.JSONSlice[string]{"API_TOKEN"},
					ConfigHash:  hex.EncodeToString(serverHash[:]),
				}},
				Skills: []types.DeviceScanSkill{{
					CreatedAt:    scannedAt,
					Client:       client,
					Scope:        scope,
					ProjectPath:  projectPath,
					File:         "~/.agents/skills/SKILL.md",
					Name:         skills[sequence%len(skills)],
					Description:  "A curated skill used in the demo fleet.",
					HasScripts:   sequence%2 == 0,
					GitRemoteURL: "https://github.com/obot-platform/skills",
					Files:        datatypes.JSONSlice[string]{"SKILL.md"},
				}},
				Files: []types.DeviceScanFile{{
					CreatedAt: scannedAt,
					Path:      "~/.config/agent/settings.json",
					SizeBytes: int64(800 + sequence*13),
					Content:   `{"permissions":{"default":"ask"}}`,
				}},
			}
			if scanIndex == 1 {
				scan.Plugins = []types.DeviceScanPlugin{{
					CreatedAt:     scannedAt,
					Client:        client,
					Scope:         scope,
					ProjectPath:   projectPath,
					ConfigPath:    "~/.config/agent/plugins.json",
					Name:          "demo-governance",
					PluginType:    "extension",
					Version:       "2.1.0",
					Description:   "Demo governance plugin",
					Author:        "Obot",
					Enabled:       sequence%5 != 0,
					Marketplace:   "obot",
					Files:         datatypes.JSONSlice[string]{"plugin.json"},
					HasMCPServers: true,
					HasSkills:     true,
				}}
			}
			scans = append(scans, scan)
		}
	}
	return scans
}

func buildMockLocalAgentAuditLogs(start, end time.Time, runID string, users []types.User, userIDs, deviceIDs []string) []types.MCPAuditLog {
	providers := []apitypes.LocalAgentProvider{
		apitypes.LocalAgentProviderClaudeCode,
		apitypes.LocalAgentProviderCodex,
		apitypes.LocalAgentProviderVSCode,
		apitypes.LocalAgentProviderCursor,
	}
	tools := []string{"read_file", "search_code", "run_tests", "create_pull_request", "query_database", "list_issues"}
	logs := make([]types.MCPAuditLog, 0, mockDataLocalAuditCount)
	for i := range mockDataLocalAuditCount {
		occurredAt := mockEventTime(start, end, i, mockDataLocalAuditCount)
		status, reason, outcomeError := mockOutcome(i)
		actorType := apitypes.AuditLogActorTypeUser
		actorID := userIDs[i%len(userIDs)]
		userID := actorID
		deviceID := ""
		if i%3 == 0 {
			actorType = apitypes.AuditLogActorTypeDevice
			actorID = deviceIDs[i%len(deviceIDs)]
			userID = ""
			deviceID = actorID
		}
		local := &types.LocalAgentToolCallAuditLogFields{
			OccurredAt:        occurredAt,
			ActorType:         actorType,
			ActorID:           actorID,
			ActionName:        tools[i%len(tools)],
			ActionKind:        "tool",
			TargetType:        apitypes.AuditLogTargetTypeLocalTool,
			TargetName:        tools[i%len(tools)],
			OutcomeStatus:     status,
			OutcomeReason:     reason,
			OutcomeError:      outcomeError,
			DurationMs:        int64(40 + (i*37)%8000),
			IdempotencyKey:    fmt.Sprintf("%s-local-%04d", runID, i),
			ToolUseID:         fmt.Sprintf("tool-%04d", i),
			SessionID:         fmt.Sprintf("%s-agent-session-%03d", runID, i/8),
			TurnID:            fmt.Sprintf("turn-%04d", i/2),
			AgentProvider:     providers[i%len(providers)],
			AgentVersion:      "1.0.0",
			CLIName:           string(providers[i%len(providers)]),
			CLIVersion:        "1.2.0",
			Model:             []string{"claude-sonnet", "gpt-codex", "gemini-pro"}[i%3],
			PermissionMode:    []string{"ask", "auto", "read-only"}[i%3],
			DeviceID:          deviceID,
			Hostname:          fmt.Sprintf("demo-host-%02d", i%len(deviceIDs)+1),
			OS:                []string{"darwin", "linux", "windows"}[i%3],
			Architecture:      []string{"arm64", "amd64"}[i%2],
			LocalUsername:     "developer",
			CWD:               "/workspaces/customer-portal",
			GitRoot:           "/workspaces/customer-portal",
			GitRemotes:        datatypes.JSONSlice[string]{"https://github.com/example/customer-portal"},
			GitBranch:         []string{"main", "feature/demo", "release"}[i%3],
			GitCommit:         fmt.Sprintf("%040x", i+1),
			ReportedUserEmail: users[i%len(users)].Email,
			RequestBody:       json.RawMessage(fmt.Sprintf(`{"path":"src/example-%d.go"}`, i%20)),
			ResponseBody:      mockResponseBody(status),
			RawEvent:          json.RawMessage(fmt.Sprintf(`{"event":"tool_complete","sequence":%d}`, i)),
		}
		logs = append(logs, types.MCPAuditLog{
			CreatedAt:                occurredAt,
			SourceType:               apitypes.AuditLogSourceTypeLocalAgentToolCall,
			UserID:                   userID,
			ClientIP:                 fmt.Sprintf("192.0.2.%d", i%200+1),
			LocalAgentToolCallFields: local,
		})
	}
	return logs
}

func buildMockMCPAuditLogs(start, end time.Time, runID string, userIDs []string) []types.MCPAuditLog {
	servers := []string{"GitHub", "PostgreSQL", "Slack", "Filesystem"}
	tools := []string{"search_repositories", "query", "send_message", "read_file", "list_pull_requests"}
	clients := []string{"Claude Code", "Codex", "Cursor", "VS Code"}
	logs := make([]types.MCPAuditLog, 0, mockDataMCPAuditCount)
	for i := range mockDataMCPAuditCount {
		createdAt := mockEventTime(start, end, i, mockDataMCPAuditCount)
		status, responseStatus, responseError := mockHTTPOutcome(i)
		serverIndex := i % len(servers)
		logs = append(logs, types.MCPAuditLog{
			CreatedAt:  createdAt,
			SourceType: apitypes.AuditLogSourceTypeMCP,
			UserID:     userIDs[i%len(userIDs)],
			ClientIP:   fmt.Sprintf("198.51.100.%d", i%200+1),
			MCPFields: &types.MCPAuditLogFields{
				MCPID:                     fmt.Sprintf("%s-mcp-%d", runID, serverIndex+1),
				MCPServerDisplayName:      servers[serverIndex],
				MCPServerCatalogEntryName: servers[serverIndex],
				ClientName:                clients[i%len(clients)],
				ClientVersion:             "1.3.0",
				CallType:                  "tools/call",
				CallIdentifier:            tools[i%len(tools)],
				RequestBody:               json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call"}`, i+1)),
				ResponseBody:              mockMCPResponseBody(status, i),
				ResponseStatus:            responseStatus,
				Error:                     responseError,
				ProcessingTimeMs:          int64(25 + (i*53)%10000),
				SessionID:                 fmt.Sprintf("%s-mcp-session-%03d", runID, i/10),
				RequestID:                 fmt.Sprintf("%s-mcp-request-%04d", runID, i),
				UserAgent:                 "obot-demo-client/1.0",
				ResponseReceived:          true,
			},
		})
	}
	return logs
}

func buildMockLLMAuditLogs(start, end time.Time, runID string, userIDs []string) []types.LLMAuditLog {
	providers := []string{system.OpenAIModelProvider, system.AnthropicModelProvider}
	models := []string{"gpt-5", "claude-sonnet-4", "gpt-4.1-mini", "claude-haiku"}
	logs := make([]types.LLMAuditLog, 0, mockDataLLMAuditCount)
	for i := range mockDataLLMAuditCount {
		createdAt := mockEventTime(start, end, i, mockDataLLMAuditCount)
		status, responseStatus, responseError := mockHTTPOutcome(i)
		outcome := types.LLMAuditOutcomeSuccess
		if status == apitypes.AuditLogOutcomeStatusTimeout {
			outcome = types.LLMAuditOutcomeCanceled
		} else if status != apitypes.AuditLogOutcomeStatusSuccess {
			outcome = types.LLMAuditOutcomeError
		}
		logs = append(logs, types.LLMAuditLog{
			ID:                     fmt.Sprintf("%s-llm-%04d", runID, i),
			CreatedAt:              createdAt,
			Duration:               int64(100 + (i*97)%20000),
			UserID:                 userIDs[i%len(userIDs)],
			ModelProvider:          providers[i%len(providers)],
			ModelID:                models[i%len(models)],
			TargetModel:            models[i%len(models)],
			ReasoningEffort:        []string{"low", "medium", "high"}[i%3],
			RequestPath:            "/v1/chat/completions",
			RequestMethod:          "POST",
			RequestHeaders:         json.RawMessage(`{"content-type":"application/json"}`),
			RequestBody:            json.RawMessage(fmt.Sprintf(`{"messages":[{"role":"user","content":"Demo request %d"}]}`, i+1)),
			MessagePolicyTriggered: i%20 == 17,
			ResponseHeaders:        json.RawMessage(`{"content-type":"application/json"}`),
			ResponseBody:           mockLLMResponseBody(status, i),
			ResponseID:             fmt.Sprintf("response-%s-%04d", runID, i),
			ResponseStatus:         responseStatus,
			Outcome:                outcome,
			Error:                  responseError,
			InputTokens:            100 + (i*17)%4000,
			OutputTokens:           40 + (i*11)%1200,
			RequestID:              fmt.Sprintf("%s-llm-request-%04d", runID, i),
			UserAgent:              "obot-demo-client/1.0",
			ClientSessionID:        fmt.Sprintf("%s-llm-session-%03d", runID, i/8),
			ClientIP:               fmt.Sprintf("203.0.113.%d", i%200+1),
		})
	}
	return logs
}

func buildMockEnforcementDecisions(start, end time.Time, deviceIDs []string) []types.EnforcementDecisionLog {
	agents := []string{"claude_code", "codex", "cursor", "vscode"}
	tools := []string{"read_file", "write_file", "bash", "search_repositories", "query", "send_message"}
	kinds := []string{"read", "write", "shell", "mcp", "generic", "task"}
	servers := []struct {
		name, url, hostname, packageName, packageVersion string
	}{
		{name: "GitHub", url: "https://mcp.example.invalid/github", hostname: "mcp.example.invalid"},
		{name: "PostgreSQL", packageName: "@modelcontextprotocol/server-postgres", packageVersion: "0.6.2"},
		{name: "Slack", url: "https://mcp.example.invalid/slack", hostname: "mcp.example.invalid"},
		{name: "Filesystem", packageName: "@modelcontextprotocol/server-filesystem", packageVersion: "0.6.2"},
	}

	decisions := make([]types.EnforcementDecisionLog, 0, mockDataEnforcementCount)
	for i := range mockDataEnforcementCount {
		kind := kinds[i%len(kinds)]
		decision := apitypes.EnforcementDecisionAllow
		reason := "matched fleet allowlist"
		if i%4 == 3 {
			decision = apitypes.EnforcementDecisionDeny
			reason = "tool is not allowed by fleet policy"
		}
		entry := types.EnforcementDecisionLog{
			CreatedAt:          mockEventTime(start, end, i, mockDataEnforcementCount),
			MDMConfigurationID: 0,
			DeviceID:           deviceIDs[i%len(deviceIDs)],
			ClientIP:           fmt.Sprintf("192.0.2.%d", i%200+1),
			Agent:              agents[i%len(agents)],
			Tool:               tools[i%len(tools)],
			Kind:               kind,
			Decision:           decision,
			Reason:             reason,
		}
		if kind == "mcp" {
			server := servers[i%len(servers)]
			entry.ServerName = server.name
			entry.ServerURL = server.url
			entry.ServerHostname = server.hostname
			entry.ServerPackageSource = "npm"
			entry.ServerPackageName = server.packageName
			entry.ServerPackageVersion = server.packageVersion
			entry.ObotHosted = i%2 == 0
		}
		if i%20 == 19 {
			entry.Decision = apitypes.EnforcementDecisionDeny
			entry.Reason = "device could not identify the target"
			entry.Unresolved = true
			entry.UnresolvedReason = "server_not_found"
			entry.ServerName = ""
			entry.ServerURL = ""
			entry.ServerHostname = ""
			entry.ServerPackageSource = ""
			entry.ServerPackageName = ""
			entry.ServerPackageVersion = ""
		}
		decisions = append(decisions, entry)
	}
	return decisions
}

func mockEventTime(start, end time.Time, index, count int) time.Time {
	if count <= 1 {
		return start
	}
	return start.Add(time.Duration(index) * end.Sub(start) / time.Duration(count-1))
}

func mockOutcome(index int) (apitypes.AuditLogOutcomeStatus, string, string) {
	switch index % 20 {
	case 17:
		return apitypes.AuditLogOutcomeStatusDenied, "policy_denied", "operation denied by policy"
	case 18:
		return apitypes.AuditLogOutcomeStatusTimeout, "deadline_exceeded", "operation timed out"
	case 19:
		return apitypes.AuditLogOutcomeStatusFailure, "tool_error", "tool returned an error"
	default:
		return apitypes.AuditLogOutcomeStatusSuccess, "", ""
	}
}

func mockHTTPOutcome(index int) (apitypes.AuditLogOutcomeStatus, int, string) {
	status, _, outcomeError := mockOutcome(index)
	switch status {
	case apitypes.AuditLogOutcomeStatusDenied:
		return status, 403, outcomeError
	case apitypes.AuditLogOutcomeStatusTimeout:
		return status, 504, outcomeError
	case apitypes.AuditLogOutcomeStatusFailure:
		return status, 500, outcomeError
	default:
		return status, 200, ""
	}
}

func mockResponseBody(status apitypes.AuditLogOutcomeStatus) json.RawMessage {
	if status == apitypes.AuditLogOutcomeStatusSuccess {
		return json.RawMessage(`{"result":"completed"}`)
	}
	return json.RawMessage(`null`)
}

func mockMCPResponseBody(status apitypes.AuditLogOutcomeStatus, index int) json.RawMessage {
	if status == apitypes.AuditLogOutcomeStatusSuccess {
		return json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":"Demo result"}]}}`, index+1))
	}
	return json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":{"code":-32000,"message":"Demo failure"}}`, index+1))
}

func mockLLMResponseBody(status apitypes.AuditLogOutcomeStatus, index int) json.RawMessage {
	if status == apitypes.AuditLogOutcomeStatusSuccess {
		return json.RawMessage(fmt.Sprintf(`{"id":"chatcmpl-demo-%d","choices":[{"message":{"role":"assistant","content":"Demo response"}}]}`, index+1))
	}
	return json.RawMessage(`{"error":{"message":"Demo request failed"}}`)
}
