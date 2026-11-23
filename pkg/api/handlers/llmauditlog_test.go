package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestParseLLMAuditLogOptsDefaultsStartTimeToLast30Days(t *testing.T) {
	before := time.Now().UTC().AddDate(0, 0, -30).Add(-time.Second)
	opts := parseLLMAuditLogOpts(url.Values{})
	after := time.Now().UTC().AddDate(0, 0, -30).Add(time.Second)

	if opts.StartTime.Before(before) || opts.StartTime.After(after) {
		t.Fatalf("expected start time around 30 days ago, got %s", opts.StartTime)
	}
}

func TestParseLLMAuditLogOptsUsesProvidedStartTime(t *testing.T) {
	want := time.Date(2026, 6, 1, 2, 3, 4, 0, time.UTC)
	opts := parseLLMAuditLogOpts(url.Values{"start_time": {want.Format(time.RFC3339)}})

	if !opts.StartTime.Equal(want) {
		t.Fatalf("expected start time %s, got %s", want, opts.StartTime)
	}
}

func TestParseLLMAuditLogOptsParsesFilterFields(t *testing.T) {
	opts := parseLLMAuditLogOpts(url.Values{
		"api_key_id":        {"42,7", "9", "invalid", "0"},
		"target_model":      {"model-a,model-b"},
		"user_agent":        {"claude-code/2.1.211"},
		"client_session_id": {"session-1"},
	})
	if !slices.Equal(opts.APIKeyID, []uint{42, 7, 9}) {
		t.Fatalf("expected API key IDs [42 7 9], got %v", opts.APIKeyID)
	}

	if got, want := opts.TargetModel, []string{"model-a", "model-b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected target models %v, got %v", want, got)
	}
	if got := opts.ClientSessionID; len(got) != 1 || got[0] != "session-1" {
		t.Fatalf("expected client session ID, got %v", got)
	}
	if got := opts.UserAgent; len(got) != 1 || got[0] != "claude-code/2.1.211" {
		t.Fatalf("expected user agent, got %v", got)
	}
}

func TestParseLLMAuditLogOptsParsesActorFilters(t *testing.T) {
	opts := parseLLMAuditLogOpts(url.Values{
		"user_id":              {"7,agent:7"},
		"actor_kind":           {"agent"},
		"include_on_behalf_of": {"true"},
		"on_behalf_of":         {"12, 13", "14"},
	})
	if !slices.Equal(opts.UserID, []string{"7", "agent:7"}) || !slices.Equal(opts.ActorKind, []string{"agent"}) || !opts.IncludeOnBehalfOf ||
		!slices.Equal(opts.OnBehalfOf, []string{"12", "13", "14"}) {
		t.Fatalf("actor filters = user %v, kind %v, include on behalf of %v, on behalf of %v", opts.UserID, opts.ActorKind, opts.IncludeOnBehalfOf, opts.OnBehalfOf)
	}
	// By default, a user's requests are only their own.
	if opts := parseLLMAuditLogOpts(url.Values{"user_id": {"7"}}); opts.IncludeOnBehalfOf || opts.OnBehalfOf != nil {
		t.Fatalf("include on behalf of %v, on behalf of %v by default", opts.IncludeOnBehalfOf, opts.OnBehalfOf)
	}
}

// TestLLMAuditLogOnBehalfOfFilter checks the on_behalf_of parameter of the LLM audit log list and its filter options:
// with an agent as the user, it selects what that agent requested for the user.
func TestLLMAuditLogOnBehalfOfFilter(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	now := time.Now().UTC()
	for _, entry := range []gatewaytypes.LLMAuditLog{
		{ID: "seven-for-twelve", CreatedAt: now, PrincipalKind: "agent", PrincipalID: "7", OnBehalfOf: "12"},
		{ID: "eight-for-twelve", CreatedAt: now, PrincipalKind: "agent", PrincipalID: "8", OnBehalfOf: "12"},
		{ID: "seven-for-thirteen", CreatedAt: now, PrincipalKind: "agent", PrincipalID: "7", OnBehalfOf: "13"},
		{ID: "twelve", CreatedAt: now, PrincipalKind: "user", UserID: "12"},
	} {
		if err := gatewayClient.InsertLLMAuditLog(t.Context(), &entry); err != nil {
			t.Fatalf("insert LLM audit log: %v", err)
		}
	}
	get := func(t *testing.T, path, query string, out any) {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path+"?"+query, nil)
		ctx := api.Context{ResponseWriter: recorder, Request: request, GatewayClient: gatewayClient}
		handler := NewLLMAuditLogHandler()
		var err error
		if filter, ok := strings.CutPrefix(path, "/api/llm-audit-logs/filter-options/"); ok {
			request.SetPathValue("filter", filter)
			err = handler.ListFilterOptions(ctx)
		} else {
			err = handler.List(ctx)
		}
		if err != nil {
			t.Fatalf("GET %s?%s: %v", path, query, err)
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}

	for query, want := range map[string][]string{
		"user_id=agent:7&on_behalf_of=12":                      {"seven-for-twelve"},
		"on_behalf_of=12":                                      {"eight-for-twelve", "seven-for-twelve"},
		"on_behalf_of=12&on_behalf_of=13":                      {"eight-for-twelve", "seven-for-thirteen", "seven-for-twelve"},
		"user_id=12&on_behalf_of=12":                           nil,
		"user_id=12&include_on_behalf_of=true&on_behalf_of=12": {"eight-for-twelve", "seven-for-twelve"},
	} {
		var response types.LLMAuditLogResponse
		get(t, "/api/llm-audit-logs", query, &response)
		var got []string
		for _, item := range response.Items {
			got = append(got, item.ID)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) || response.Total != int64(len(want)) {
			t.Errorf("LLM audit logs for %s = %v (total %d), want %v", query, got, response.Total, want)
		}
	}

	for query, want := range map[string][]string{
		"":                {"12", "13"},
		"user_id=agent:8": {"12"},
	} {
		var response struct {
			Options []string `json:"options"`
		}
		get(t, "/api/llm-audit-logs/filter-options/on_behalf_of", query, &response)
		if !slices.Equal(response.Options, want) {
			t.Errorf("on_behalf_of options for %q = %v, want %v", query, response.Options, want)
		}
	}
}

func TestParseLLMAuditLogOptsHideModelsRequests(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  bool
	}{
		{
			name: "absent defaults false",
		},
		{
			name:  "false",
			value: "false",
		},
		{
			name:  "invalid",
			value: "invalid",
		},
		{
			name:  "true",
			value: "true",
			want:  true,
		},
		{
			name:  "uppercase true",
			value: "TRUE",
			want:  true,
		},
		{
			name:  "one",
			value: "1",
			want:  true,
		},
		{
			name:  "short true",
			value: "t",
			want:  true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query := url.Values{}
			if tt.value != "" {
				query.Set("hide_models_requests", tt.value)
			}

			if got := parseLLMAuditLogOpts(query).HideModelsRequests; got != tt.want {
				t.Fatalf("expected HideModelsRequests=%t, got %t", tt.want, got)
			}
		})
	}
}

func TestHideModelsRequestsFilterOptions(t *testing.T) {
	for _, tt := range []struct {
		name  string
		paths []string
		want  []string
	}{
		{
			name: "no path",
			want: []string{"false", "true"},
		},
		{
			name:  "non-models path",
			paths: []string{"/api/llm-proxy/openai/v1/responses"},
			want:  []string{"false", "true"},
		},
		{
			name:  "models path",
			paths: []string{"/api/llm-proxy/openai/models"},
			want:  []string{"false"},
		},
		{
			name:  "models path with trailing slash",
			paths: []string{"/api/llm-proxy/openai/models/"},
			want:  []string{"false"},
		},
		{
			name:  "nested models path",
			paths: []string{"/api/llm-proxy/openai/v1/models"},
			want:  []string{"false"},
		},
		{
			name:  "specific model path",
			paths: []string{"/api/llm-proxy/openai/models/model-1"},
			want:  []string{"false", "true"},
		},
		{
			name:  "mixed paths",
			paths: []string{"/api/llm-proxy/openai/v1/responses", "/api/llm-proxy/openai/models"},
			want:  []string{"false"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hideModelsRequestsFilterOptions(tt.paths)
			if len(got) != len(tt.want) {
				t.Fatalf("expected options %v, got %v", tt.want, got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("expected options %v, got %v", tt.want, got)
				}
			}
		})
	}
}

func TestListLLMAuditLogAPIKeyFilterOptionsReturnsStructuredOptions(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	created, err := gatewayClient.CreateAPIKey(t.Context(), 7, "CLI token", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}
	entry := gatewaytypes.LLMAuditLog{
		ID: uuid.New().String(), CreatedAt: time.Now().UTC(), UserID: "7",
		APIKeyID: &created.ID, APIKeyName: "CLI token",
	}
	if err := gatewayClient.InsertLLMAuditLog(t.Context(), &entry); err != nil {
		t.Fatalf("insert LLM audit log: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/llm-audit-logs/filter-options/api_key_id", nil)
	request.SetPathValue("filter", "api_key_id")
	ctx := api.Context{ResponseWriter: recorder, Request: request, GatewayClient: gatewayClient}
	if err := NewLLMAuditLogHandler().ListFilterOptions(ctx); err != nil {
		t.Fatalf("list API-key filter options: %v", err)
	}

	var response struct {
		Options []types.AuditLogAPIKeyFilterOption `json:"options"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Options) != 1 || response.Options[0].Value != fmt.Sprint(created.ID) || response.Options[0].Name != "CLI token" || response.Options[0].MaskedKey != fmt.Sprintf("ok1-7-%d-*****", created.ID) {
		t.Fatalf("unexpected structured options: %#v", response.Options)
	}
}
