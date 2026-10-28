package client

import (
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestAuditLogDurationFilters(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()
	type fixture struct {
		id       uint
		source   apitypes.AuditLogSourceType
		duration int64
		tool     string
		keyID    string
	}
	var fixtures []fixture
	sources := []apitypes.AuditLogSourceType{
		apitypes.AuditLogSourceTypeMCP,
		apitypes.AuditLogSourceTypeLocalAgentToolCall,
	}
	// -1 denotes a legacy SQL NULL; zero is the current missing-duration representation.
	for _, source := range sources {
		for _, duration := range []int64{-1, 0, 1, 999, 1000, 5000, 30000, 30001} {
			tool := fmt.Sprintf("%s-%d", source, duration)
			keyID := uint(len(fixtures) + 1)
			var log types.MCPAuditLog
			column := "processing_time_ms"
			if source == apitypes.AuditLogSourceTypeLocalAgentToolCall {
				log = validLocalAgentAuditLog(now, tool, apitypes.AuditLogOutcomeStatusSuccess)
				log.LocalAgentToolCallFields.ActionName = tool
				log.LocalAgentToolCallFields.DurationMs = max(duration, 0)
				column = "duration_ms"
			} else {
				log = types.MCPAuditLog{
					CreatedAt:  now,
					SourceType: source,
					MCPFields: &types.MCPAuditLogFields{
						MCPID:            "mcp-1",
						CallType:         "tools/call",
						CallIdentifier:   tool,
						ProcessingTimeMs: max(duration, 0),
						ResponseReceived: true,
					},
				}
			}
			log.APIKeyID = &keyID
			log.APIKeyName = tool
			if err := c.db.WithContext(ctx).Create(&log).Error; err != nil {
				t.Fatalf("insert %s: %v", tool, err)
			}
			if duration == -1 {
				if err := c.db.WithContext(ctx).Model(&log).Update(column, nil).Error; err != nil {
					t.Fatalf("set NULL duration for %s: %v", tool, err)
				}
			}
			fixtures = append(fixtures, fixture{
				id:       log.ID,
				source:   source,
				duration: duration,
				tool:     tool,
				keyID:    strconv.FormatUint(uint64(keyID), 10),
			})
		}
	}

	for _, scope := range []struct {
		name    string
		sources []apitypes.AuditLogSourceType
	}{
		{
			name:    "mcp",
			sources: sources[:1],
		},
		{
			name:    "local",
			sources: sources[1:],
		},
		{
			name:    "mixed",
			sources: sources,
		},
	} {
		t.Run(scope.name, func(t *testing.T) {
			for _, tt := range []struct {
				name string
				min  int64
				max  int64
				want []int64
			}{
				{
					name: "under one second",
					max:  1000,
					want: []int64{1, 999, 1000},
				},
				{
					name: "one to five seconds",
					min:  1000,
					max:  5000,
					want: []int64{1000, 5000},
				},
				{
					name: "five to thirty seconds",
					min:  5000,
					max:  30000,
					want: []int64{5000, 30000},
				},
				{
					name: "over thirty seconds",
					min:  30000,
					want: []int64{30000, 30001},
				},
				{
					name: "cleared",
					want: []int64{-1, 0, 1, 999, 1000, 5000, 30000, 30001},
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					opts := MCPAuditLogOptions{
						SourceTypes:       scope.sources,
						ProcessingTimeMin: tt.min,
						ProcessingTimeMax: tt.max,
					}
					var wantIDs []uint
					var wantTools, wantKeys []string
					for _, f := range fixtures {
						if slices.Contains(scope.sources, f.source) && slices.Contains(tt.want, f.duration) {
							wantIDs = append(wantIDs, f.id)
							wantTools = append(wantTools, f.tool)
							wantKeys = append(wantKeys, f.keyID)
						}
					}
					logs, total, err := c.GetMCPAuditLogs(ctx, opts)
					if err != nil {
						t.Fatal(err)
					}
					var gotIDs []uint
					for _, log := range logs {
						gotIDs = append(gotIDs, log.ID)
					}
					slices.Sort(gotIDs)
					if total != int64(len(wantIDs)) || !slices.Equal(gotIDs, wantIDs) {
						t.Errorf("list IDs = %v, total = %d; want IDs = %v, total = %d", gotIDs, total, wantIDs, len(wantIDs))
					}
					tools, err := c.GetAuditLogFilterOptions(ctx, "tool", opts)
					if err != nil {
						t.Fatal(err)
					}
					slices.Sort(wantTools)
					if !slices.Equal(tools, wantTools) {
						t.Errorf("tool options = %v, want %v", tools, wantTools)
					}
					keys, err := c.GetMCPAuditLogAPIKeyFilterOptions(ctx, opts)
					if err != nil {
						t.Fatal(err)
					}
					var gotKeys []string
					for _, key := range keys {
						gotKeys = append(gotKeys, key.Value)
					}
					slices.Sort(gotKeys)
					slices.Sort(wantKeys)
					if !slices.Equal(gotKeys, wantKeys) {
						t.Errorf("API key options = %v, want %v", gotKeys, wantKeys)
					}
				})
			}
		})
	}
}
