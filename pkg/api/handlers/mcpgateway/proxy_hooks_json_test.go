package mcpgateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
)

func TestAuthorizationCaseCollisionsCannotReachUpstream(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       string
		method     string
		identifier string
		properties map[string]any
	}{
		{
			name:       "uppercase ID cannot turn call into notification",
			body:       `{"jsonrpc":"2.0","id":1,"ID":null,"method":"tools/call","params":{"name":"protected"}}`,
			method:     "tools/call",
			properties: map[string]any{"tool": "protected"},
		},
		{
			name:       "uppercase method cannot bypass method selector",
			body:       `{"jsonrpc":"2.0","id":1,"method":"tools/call","Method":"ping","params":{"name":"protected"}}`,
			method:     "tools/call",
			identifier: "protected",
			properties: map[string]any{"tool": "protected"},
		},
		{
			name:       "uppercase params cannot replace operation",
			body:       `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"protected"},"Params":{"name":"public"}}`,
			method:     "tools/call",
			identifier: "protected",
			properties: map[string]any{"tool": "protected"},
		},
		{
			name:       "uppercase tool name cannot bypass identifier selector",
			body:       `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"protected","Name":"public"}}`,
			method:     "tools/call",
			identifier: "protected",
			properties: map[string]any{"tool": "protected"},
		},
		{
			name:       "uppercase prompt name",
			body:       `{"jsonrpc":"2.0","id":1,"method":"prompts/get","params":{"name":"protected","Name":"public"}}`,
			method:     "prompts/get",
			identifier: "protected",
			properties: map[string]any{"prompt": "protected"},
		},
		{
			name:       "uppercase URI",
			body:       `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"crm://protected","URI":"crm://public"}}`,
			method:     "resources/read",
			identifier: "crm://protected",
			properties: map[string]any{"uri": "crm://protected"},
		},
		{
			name:       "completion resource URI",
			body:       `{"jsonrpc":"2.0","id":1,"method":"completion/complete","params":{"ref":{"type":"ref/resource","uri":"crm://protected/{id}","URI":"crm://public/{id}"},"argument":{"name":"id","value":"secret"}}}`,
			method:     "completion/complete",
			properties: map[string]any{"ref": map[string]any{"type": "ref/resource", "uri": "crm://protected/{id}"}, "argument": "id"},
		},
		{
			name:       "nested completion selectors",
			body:       `{"jsonrpc":"2.0","id":1,"method":"completion/complete","params":{"ref":{"type":"ref/prompt","Type":"ref/resource","name":"protected","Name":"public"},"Ref":{"type":"ref/prompt","name":"public"},"argument":{"name":"style","Name":"other","value":"secret"},"Argument":{"name":"other"}}}`,
			method:     "completion/complete",
			properties: map[string]any{"ref": map[string]any{"type": "ref/prompt", "name": "protected"}, "argument": "style"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var evaluations, upstreamCalls atomic.Int32
			policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				evaluations.Add(1)
				var envelope authorizationRequest
				if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
					t.Error(err)
				}
				if envelope.Action.Name != tt.method || !reflect.DeepEqual(envelope.Action.Properties, tt.properties) {
					t.Errorf("unexpected action: %+v", envelope.Action)
				}
				_, _ = io.WriteString(w, `{"decision":false}`)
			}))
			defer policy.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { upstreamCalls.Add(1); w.WriteHeader(http.StatusOK) }))
			defer upstream.Close()
			target, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			hook := mcp.HookMapping{Name: "*", Authorization: &mcp.AuthorizationHook{FilterID: "policy", URL: policy.URL}}
			if tt.identifier != "" {
				hook.Name = tt.method
				hook.Params = map[string]string{"name": tt.identifier}
			}
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				processor, err := newHookProcessor(r, new(scriptedMCPHookRunner), mcp.Hooks{hook}, nil, nil, nil, nil, nil, testAuthorizer(policy))
				if errors.Is(err, errMCPCaseVariantFieldNames) {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err != nil {
					t.Error(err)
					http.Error(w, "parse failed", http.StatusInternalServerError)
					return
				}
				if body, blocked, blockErr := processor.blockedRequest(); blocked {
					w.WriteHeader(hookHTTPStatus(blockErr))
					_, _ = w.Write(body)
					return
				}
				proxy.ServeHTTP(w, r)
			}))
			defer gateway.Close()
			response, err := gateway.Client().Post(gateway.URL, "application/json", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			wantStatus, wantEvaluations := http.StatusBadRequest, int32(0)
			if tt.method == "completion/complete" {
				wantStatus, wantEvaluations = http.StatusForbidden, 1
			}
			if response.StatusCode != wantStatus || evaluations.Load() != wantEvaluations || upstreamCalls.Load() != 0 {
				t.Fatalf("status=%d evaluations=%d upstream calls=%d", response.StatusCode, evaluations.Load(), upstreamCalls.Load())
			}
		})
	}
}

func TestMCPHookDecodeCompatibility(t *testing.T) {
	var message mcp.Message
	if err := decodeMCPHookMessage([]byte(`{"jsonrpc":"2.0","id":9007199254740993,"method":"ping","extension":{"arbitrary":true}}`), &message); err != nil {
		t.Fatal(err)
	}
	if number, ok := message.ID.(json.Number); !ok || number.String() != "9007199254740993" {
		t.Fatalf("request ID lost precision: %#v", message.ID)
	}
	message = mcp.Message{}
	if err := decodeMCPHookMessage([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized","ID":123}`), &message); err != nil {
		t.Fatal(err)
	}
	if message.ID != nil {
		t.Fatalf("unknown uppercase ID became notification ID: %v", message.ID)
	}
	for _, body := range []string{`{"jsonrpc":"2.0","id":1,"method":"ping"}{}`, `{"jsonrpc":"2.0","id":1,"method":"ping"}garbage`} {
		if err := decodeMCPHookMessage([]byte(body), new(mcp.Message)); err == nil {
			t.Fatalf("accepted trailing data: %s", body)
		}
	}
}
