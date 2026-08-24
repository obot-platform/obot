package client

import (
	"fmt"
	"slices"
	"testing"
	"time"
	"uuid"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestAuditLogAPIKeyFilterOptions(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	user := types.User{ID: 7, DisplayName: "Calvin McLean", Username: "calvin", Email: "calvin@example.com"}
	if err := c.db.WithContext(ctx).Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	named, err := c.CreateAPIKey(ctx, user.ID, "CLI token", "", nil, types.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatalf("create named key: %v", err)
	}
	duplicateName, err := c.CreateAPIKey(ctx, user.ID, "CLI token", "", nil, types.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatalf("create duplicate-name key: %v", err)
	}
	unnamed, err := c.CreateAPIKey(ctx, user.ID, "", "", nil, types.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatalf("create unnamed key: %v", err)
	}
	revokedAt := time.Now().UTC()
	if err := c.db.WithContext(ctx).Model(&types.APIKey{}).Where("id = ?", duplicateName.ID).Update("revoked_at", revokedAt).Error; err != nil {
		t.Fatalf("mark duplicate-name key revoked: %v", err)
	}

	now := time.Now().UTC()
	keys := []*types.APIKeyCreateResponse{named, duplicateName, unnamed}
	for i, key := range keys {
		name := key.Name
		if name == "" {
			name = fmt.Sprintf("ok1-%d-%d-*****", user.ID, key.ID)
		}
		mcpID := "mcp-1"
		modelProvider := "openai"
		if key.ID == unnamed.ID {
			mcpID = "mcp-2"
			modelProvider = "anthropic"
		}
		mcpLog := types.MCPAuditLog{
			CreatedAt: now.Add(time.Duration(i) * time.Minute), SourceType: apitypes.AuditLogSourceTypeMCP,
			UserID: fmt.Sprint(user.ID), APIKeyID: &key.ID, APIKeyName: name,
			MCPFields: &types.MCPAuditLogFields{MCPID: mcpID, CallType: "tools/call", CallIdentifier: fmt.Sprintf("tool-%d", key.ID)},
		}
		if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{mcpLog}); err != nil {
			t.Fatalf("insert MCP audit log: %v", err)
		}
		llmLog := types.LLMAuditLog{
			ID: uuid.New().String(), CreatedAt: now.Add(time.Duration(i) * time.Minute), UserID: fmt.Sprint(user.ID),
			APIKeyID: &key.ID, APIKeyName: name, ModelProvider: modelProvider,
		}
		if err := c.InsertLLMAuditLog(ctx, &llmLog); err != nil {
			t.Fatalf("insert LLM audit log: %v", err)
		}
	}
	missingKeyID := uint(999999)
	missingKeyMCPLog := types.MCPAuditLog{
		CreatedAt: now.Add(3 * time.Minute), SourceType: apitypes.AuditLogSourceTypeMCP,
		UserID: fmt.Sprint(user.ID), APIKeyID: &missingKeyID, APIKeyName: "Deleted key snapshot",
		MCPFields: &types.MCPAuditLogFields{MCPID: "mcp-missing", CallType: "tools/call", CallIdentifier: "missing-key-tool"},
	}
	if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{missingKeyMCPLog}); err != nil {
		t.Fatalf("insert missing-key MCP audit log: %v", err)
	}
	missingKeyLLMLog := types.LLMAuditLog{
		ID: uuid.New().String(), CreatedAt: now.Add(3 * time.Minute), UserID: fmt.Sprint(user.ID),
		APIKeyID: &missingKeyID, APIKeyName: "Deleted key snapshot", ModelProvider: "missing",
	}
	if err := c.InsertLLMAuditLog(ctx, &missingKeyLLMLog); err != nil {
		t.Fatalf("insert missing-key LLM audit log: %v", err)
	}
	localLog := validLocalAgentAuditLog(now.Add(4*time.Minute), "local-api-key", apitypes.AuditLogOutcomeStatusSuccess)
	localLog.APIKeyID = &named.ID
	localLog.APIKeyName = named.Name
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{localLog}); err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			userGroupResource: testTransformer{},
		},
	}
	if err := c.encryptUser(ctx, &user); err != nil {
		t.Fatalf("encrypt user: %v", err)
	}
	if err := c.db.WithContext(ctx).Save(&user).Error; err != nil {
		t.Fatalf("save encrypted user: %v", err)
	}

	mcpLogs, _, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
		SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP},
	})
	if err != nil {
		t.Fatalf("get enriched MCP audit logs: %v", err)
	}
	var revokedMCPLogID uint
	for _, log := range mcpLogs {
		if log.APIKeyID == nil {
			continue
		}
		wantRevoked := *log.APIKeyID == duplicateName.ID
		if log.APIKeyRevoked != wantRevoked {
			t.Fatalf("MCP key %d revoked = %v, want %v", *log.APIKeyID, log.APIKeyRevoked, wantRevoked)
		}
		if wantRevoked {
			revokedMCPLogID = log.ID
		}
	}
	if revokedMCPLogID == 0 {
		t.Fatal("revoked MCP audit log not found")
	}
	mcpDetail, err := c.GetMCPAuditLog(ctx, revokedMCPLogID, false)
	if err != nil {
		t.Fatalf("get enriched MCP audit log detail: %v", err)
	}
	if !mcpDetail.APIKeyRevoked {
		t.Fatal("revoked MCP audit log detail was not enriched")
	}

	llmLogs, _, err := c.GetLLMAuditLogs(ctx, LLMAuditLogOptions{})
	if err != nil {
		t.Fatalf("get enriched LLM audit logs: %v", err)
	}
	var revokedLLMLogID string
	for _, log := range llmLogs {
		if log.APIKeyID == nil {
			continue
		}
		wantRevoked := *log.APIKeyID == duplicateName.ID
		if log.APIKeyRevoked != wantRevoked {
			t.Fatalf("LLM key %d revoked = %v, want %v", *log.APIKeyID, log.APIKeyRevoked, wantRevoked)
		}
		if wantRevoked {
			revokedLLMLogID = log.ID
		}
	}
	if revokedLLMLogID == "" {
		t.Fatal("revoked LLM audit log not found")
	}
	llmDetail, err := c.GetLLMAuditLog(ctx, revokedLLMLogID, false)
	if err != nil {
		t.Fatalf("get enriched LLM audit log detail: %v", err)
	}
	if !llmDetail.APIKeyRevoked {
		t.Fatal("revoked LLM audit log detail was not enriched")
	}

	assertOptions := func(t *testing.T, options []apitypes.AuditLogAPIKeyFilterOption) {
		t.Helper()
		if len(options) != 4 {
			t.Fatalf("expected four distinct key options, got %#v", options)
		}
		values := []string{options[0].Value, options[1].Value, options[2].Value, options[3].Value}
		slices.Sort(values)
		wantValues := []string{fmt.Sprint(named.ID), fmt.Sprint(duplicateName.ID), fmt.Sprint(unnamed.ID), fmt.Sprint(missingKeyID)}
		slices.Sort(wantValues)
		if !slices.Equal(values, wantValues) {
			t.Fatalf("values = %v, want %v", values, wantValues)
		}
		byValue := make(map[string]apitypes.AuditLogAPIKeyFilterOption, len(options))
		for _, option := range options {
			byValue[option.Value] = option
			if option.Value == fmt.Sprint(missingKeyID) {
				continue
			}
			if option.UserID != "7" || option.UserDisplayName != "" {
				t.Fatalf("unexpected owner metadata: %#v", option)
			}
			if option.MaskedKey != fmt.Sprintf("ok1-7-%s-*****", option.Value) {
				t.Fatalf("unexpected masked key: %#v", option)
			}
		}
		if !byValue[fmt.Sprint(duplicateName.ID)].Revoked {
			t.Fatalf("revoked key not marked revoked: %#v", byValue[fmt.Sprint(duplicateName.ID)])
		}
		if byValue[fmt.Sprint(named.ID)].Name != "CLI token" || byValue[fmt.Sprint(duplicateName.ID)].Name != "CLI token" {
			t.Fatalf("duplicate event-time names were not preserved: %#v", options)
		}
		if got := byValue[fmt.Sprint(unnamed.ID)].Name; got != fmt.Sprintf("ok1-7-%d-*****", unnamed.ID) {
			t.Fatalf("unnamed key name = %q", got)
		}
		if missing := byValue[fmt.Sprint(missingKeyID)]; missing.Name != "Deleted key snapshot" ||
			missing.UserID != "" || missing.UserDisplayName != "" || missing.MaskedKey != "" || missing.Revoked {
			t.Fatalf("missing key metadata was not preserved as an event-time snapshot: %#v", missing)
		}
	}

	mcpOptions, err := c.GetMCPAuditLogAPIKeyFilterOptions(ctx, MCPAuditLogOptions{})
	if err != nil {
		t.Fatalf("get MCP key options: %v", err)
	}
	assertOptions(t, mcpOptions)

	localOptions, err := c.GetMCPAuditLogAPIKeyFilterOptions(ctx, MCPAuditLogOptions{
		SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeLocalAgentToolCall},
	})
	if err != nil {
		t.Fatalf("get local-agent key options: %v", err)
	}
	if len(localOptions) != 1 || localOptions[0].Value != fmt.Sprint(named.ID) {
		t.Fatalf("unexpected local-agent key options: %#v", localOptions)
	}

	mixedOptions, err := c.GetMCPAuditLogAPIKeyFilterOptions(ctx, MCPAuditLogOptions{
		SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP, apitypes.AuditLogSourceTypeLocalAgentToolCall},
	})
	if err != nil {
		t.Fatalf("get mixed-source key options: %v", err)
	}
	assertOptions(t, mixedOptions)

	llmOptions, err := c.GetLLMAuditLogAPIKeyFilterOptions(ctx, LLMAuditLogOptions{})
	if err != nil {
		t.Fatalf("get LLM key options: %v", err)
	}
	assertOptions(t, llmOptions)

	scopedMCP, err := c.GetMCPAuditLogAPIKeyFilterOptions(ctx, MCPAuditLogOptions{
		SourceTypes:     []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP},
		OwnServerMCPIDs: []string{"mcp-1"},
		EndTime:         now.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatalf("get scoped MCP key options: %v", err)
	}
	if len(scopedMCP) != 2 {
		t.Fatalf("MCP scope and time filters were not applied: %#v", scopedMCP)
	}
	unifiedOptions, err := c.GetAuditLogFilterOptions(ctx, "tool", MCPAuditLogOptions{
		SourceTypes: []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP},
		APIKeyID:    []uint{named.ID},
	})
	if err != nil {
		t.Fatalf("get API-key-scoped unified options: %v", err)
	}
	if len(unifiedOptions) != 1 || unifiedOptions[0] != fmt.Sprintf("tool-%d", named.ID) {
		t.Fatalf("API key filter did not narrow unified options: %#v", unifiedOptions)
	}

	scopedLLM, err := c.GetLLMAuditLogAPIKeyFilterOptions(ctx, LLMAuditLogOptions{ModelProvider: []string{"openai"}})
	if err != nil {
		t.Fatalf("get scoped LLM key options: %v", err)
	}
	if len(scopedLLM) != 2 {
		t.Fatalf("LLM filters were not applied: %#v", scopedLLM)
	}

	limited, err := c.GetLLMAuditLogAPIKeyFilterOptions(ctx, LLMAuditLogOptions{ModelProvider: []string{"openai"}, Limit: 1})
	if err != nil {
		t.Fatalf("get limited LLM key options: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limit was not applied: %#v", limited)
	}
}
