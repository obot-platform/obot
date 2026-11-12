package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

var (
	// auditEncryptionModes are the storage configurations every body test must hold under.
	auditEncryptionModes = []struct {
		name      string
		newClient func(t *testing.T) *Client
	}{
		{
			name:      "unencrypted",
			newClient: func(t *testing.T) *Client { t.Helper(); return newTestClient(t) },
		},
		{
			name: "encrypted",
			newClient: func(t *testing.T) *Client {
				t.Helper()
				c := newTestClient(t)
				c.encryptionConfig = contextBoundEncryptionConfig()
				return c
			},
		},
	}
)

// contextBoundTransformer stands in for an authenticated cipher: what it encrypts can only be
// decrypted under the same context, so a test fails if a body is read under the wrong one.
type contextBoundTransformer struct{}

func (contextBoundTransformer) TransformToStorage(_ context.Context, data []byte, dataCtx value.Context) ([]byte, error) {
	prefix := contextBoundPrefix(dataCtx)
	out := make([]byte, 0, len(prefix)+len(data))
	out = append(out, prefix...)
	for _, b := range data {
		out = append(out, b^0x5a)
	}
	return out, nil
}

func (contextBoundTransformer) TransformFromStorage(_ context.Context, data []byte, dataCtx value.Context) ([]byte, bool, error) {
	rest, ok := bytes.CutPrefix(data, contextBoundPrefix(dataCtx))
	if !ok {
		return nil, false, errors.New("ciphertext does not belong to this context")
	}
	out := make([]byte, len(rest))
	for i, b := range rest {
		out[i] = b ^ 0x5a
	}
	return out, false, nil
}

func contextBoundPrefix(dataCtx value.Context) []byte {
	return fmt.Appendf(nil, "enc[%s]:", dataCtx.AuthenticatedData())
}

func contextBoundEncryptionConfig() *encryptionconfig.EncryptionConfiguration {
	return &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			mcpAuditLogGroupResource: contextBoundTransformer{},
		},
	}
}

// testListResult returns a tool list result of at least minBytes. Different seeds give different
// results.
func testListResult(seed string, minBytes int) string {
	var b strings.Builder
	b.WriteString(`{"tools":[`)
	for i := 0; b.Len() < minBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":"%s_tool_%d","description":"Searches %s records by keyword and returns the matching entries with their identifiers.","inputSchema":{"type":"object","properties":{"query":{"type":"string","description":"Text to search for"},"limit":{"type":"integer","minimum":1,"maximum":%d}},"required":["query"]}}`, seed, i, seed, 100+i)
	}
	b.WriteString(`]}`)
	return b.String()
}

func testListResponse(id int, result string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, id, result))
}

// storeMCPAuditEntries puts entries through the same encoding and persistence steps as the audit
// log write path.
func storeMCPAuditEntries(t *testing.T, c *Client, entries ...types.MCPAuditLog) {
	t.Helper()
	for i := range entries {
		entries[i].NormalizeMCPFields()
		if err := c.encryptMCPAuditLog(t.Context(), &entries[i]); err != nil {
			t.Fatalf("encode audit entry %d: %v", i, err)
		}
	}
	if err := c.insertMCPAuditLogs(t.Context(), entries); err != nil {
		t.Fatalf("insert audit entries: %v", err)
	}
}

// listCallEntries returns the request entry and the separate response entry the proxy emits for
// one list call.
func listCallEntries(requestID int, method string, response json.RawMessage) []types.MCPAuditLog {
	now := time.Now().UTC()
	id := fmt.Sprint(requestID)
	return []types.MCPAuditLog{
		{
			CreatedAt: now,
			UserID:    "user-1",
			MCPFields: &types.MCPAuditLogFields{
				MCPID:           "mcp-1",
				RequestID:       id,
				ProxyExchangeID: "exchange-" + id,
				CallType:        method,
				RequestBody:     json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q}`, requestID, method)),
			},
		},
		{
			CreatedAt: now.Add(10 * time.Millisecond),
			UserID:    "user-1",
			MCPFields: &types.MCPAuditLogFields{
				MCPID:            "mcp-1",
				RequestID:        id,
				ProxyExchangeID:  "exchange-" + id,
				CallType:         mcpResponseCallType,
				ResponseMethod:   method,
				ResponseReceived: true,
				ResponseStatus:   200,
				ResponseBody:     response,
			},
		},
	}
}

func countAuditBodies(t *testing.T, c *Client) int64 {
	t.Helper()
	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Count(&count).Error; err != nil {
		t.Fatalf("count shared audit bodies: %v", err)
	}
	return count
}

// readMCPAuditLogs returns every MCP audit row with its payloads, oldest first.
func readMCPAuditLogs(t *testing.T, c *Client) []types.MCPAuditLog {
	t.Helper()
	logs, _, err := c.GetMCPAuditLogs(t.Context(), MCPAuditLogOptions{
		WithRequestAndResponse: true,
		SortOrder:              "asc",
	})
	if err != nil {
		t.Fatalf("read audit logs: %v", err)
	}
	return logs
}

// storedResponseBody returns the response_body column of the only audit row exactly as stored.
func storedResponseBody(t *testing.T, c *Client) []byte {
	t.Helper()
	var row types.MCPAuditLog
	if err := c.db.WithContext(t.Context()).First(&row).Error; err != nil {
		t.Fatalf("load stored audit row: %v", err)
	}
	return row.MCP().ResponseBody
}

func TestAuditBodyCodecRoundTrip(t *testing.T) {
	random := make([]byte, 4096)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		plain          []byte
		wantCompressed bool
	}{
		{
			name:           "small body is stored as is",
			plain:          []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`),
			wantCompressed: false,
		},
		{
			name:           "large body is compressed",
			plain:          []byte(testListResult("codec", 8192)),
			wantCompressed: true,
		},
		{
			name:           "body that does not shrink is stored as is",
			plain:          random,
			wantCompressed: false,
		},
	}
	transformers := []struct {
		name        string
		transformer value.Transformer
	}{
		{
			name:        "unencrypted",
			transformer: nil,
		},
		{
			name:        "encrypted",
			transformer: contextBoundTransformer{},
		},
	}
	dataCtx := value.DefaultContext("codec-test")

	for _, tt := range tests {
		for _, mode := range transformers {
			t.Run(tt.name+"/"+mode.name, func(t *testing.T) {
				field, err := encodeAuditBody(t.Context(), mode.transformer, dataCtx, tt.plain, 0)
				if err != nil {
					t.Fatalf("encode: %v", err)
				}
				if !isEncodedAuditBody(field) {
					t.Fatal("encoded field does not carry the format marker")
				}
				if got := field[2]&auditBodyFlagCompressed != 0; got != tt.wantCompressed {
					t.Fatalf("compressed flag = %v, want %v", got, tt.wantCompressed)
				}
				if got := field[2]&auditBodyFlagEncrypted != 0; got != (mode.transformer != nil) {
					t.Fatalf("encrypted flag = %v, want %v", got, mode.transformer != nil)
				}
				if tt.wantCompressed && len(field) >= len(tt.plain) {
					t.Fatalf("compressed field is %d bytes, not smaller than the %d-byte body", len(field), len(tt.plain))
				}

				plain, flags, err := decodeAuditBody(t.Context(), mode.transformer, dataCtx, field)
				if err != nil {
					t.Fatalf("decode: %v", err)
				}
				if !bytes.Equal(plain, tt.plain) {
					t.Fatal("decoded body differs from the original")
				}
				if flags&auditBodyFlagReference != 0 {
					t.Fatal("inline body decoded as a reference")
				}
			})
		}
	}
}

func TestDecodeAuditBodyRejectsUnreadableFields(t *testing.T) {
	dataCtx := value.DefaultContext("codec-test")
	encrypted, err := encodeAuditBody(t.Context(), contextBoundTransformer{}, dataCtx, []byte(`{"ok":true}`), 0)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		field       []byte
		transformer value.Transformer
		dataCtx     value.Context
	}{
		{
			name:        "truncated header",
			field:       []byte{auditBodyMarker, auditBodyVersion},
			transformer: nil,
			dataCtx:     dataCtx,
		},
		{
			name:        "unknown version",
			field:       []byte{auditBodyMarker, 0x7f, 0x00, '{', '}'},
			transformer: nil,
			dataCtx:     dataCtx,
		},
		{
			name:        "unknown flag",
			field:       []byte{auditBodyMarker, auditBodyVersion, 0x80, '{', '}'},
			transformer: nil,
			dataCtx:     dataCtx,
		},
		{
			name:        "corrupt compressed payload",
			field:       []byte{auditBodyMarker, auditBodyVersion, auditBodyFlagCompressed, 'n', 'o', 'p', 'e'},
			transformer: nil,
			dataCtx:     dataCtx,
		},
		{
			name:        "encrypted without a transformer",
			field:       encrypted,
			transformer: nil,
			dataCtx:     dataCtx,
		},
		{
			name:        "encrypted under another context",
			field:       encrypted,
			transformer: contextBoundTransformer{},
			dataCtx:     value.DefaultContext("another-context"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeAuditBody(t.Context(), tt.transformer, tt.dataCtx, tt.field); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSplitJSONRPCResult(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantResult string
		wantOK     bool
	}{
		{
			name:       "compact response",
			body:       `{"jsonrpc":"2.0","id":7,"result":{"tools":[]}}`,
			wantResult: `{"tools":[]}`,
			wantOK:     true,
		},
		{
			name:       "result first with whitespace",
			body:       "{ \"result\" :  {\"tools\": [ 1, 2 ]} ,\n \"id\": \"a\", \"jsonrpc\": \"2.0\" }\n",
			wantResult: `{"tools": [ 1, 2 ]}`,
			wantOK:     true,
		},
		{
			name:       "nested result key is not the top-level result",
			body:       `{"jsonrpc":"2.0","id":7,"result":{"result":{"x":1}}}`,
			wantResult: `{"result":{"x":1}}`,
			wantOK:     true,
		},
		{
			name:   "error response",
			body:   `{"jsonrpc":"2.0","id":7,"error":{"code":-32000,"message":"boom"}}`,
			wantOK: false,
		},
		{
			name:   "request",
			body:   `{"jsonrpc":"2.0","id":7,"method":"tools/list","result":{}}`,
			wantOK: false,
		},
		{
			name:   "duplicate result",
			body:   `{"jsonrpc":"2.0","id":7,"result":{"a":1},"result":{"b":2}}`,
			wantOK: false,
		},
		{
			name:   "batch",
			body:   `[{"jsonrpc":"2.0","id":7,"result":{}}]`,
			wantOK: false,
		},
		{
			name:   "non-JSON body captured as a string",
			body:   `"event: message"`,
			wantOK: false,
		},
		{
			name:   "trailing content",
			body:   `{"jsonrpc":"2.0","id":7,"result":{}} {}`,
			wantOK: false,
		},
		{
			name:   "truncated",
			body:   `{"jsonrpc":"2.0","id":7,"result":{"tools":[`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := splitJSONRPCResult([]byte(tt.body))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got := tt.body[start:end]; got != tt.wantResult {
				t.Fatalf("result = %q, want %q", got, tt.wantResult)
			}

			envelope := tt.body[:start] + tt.body[end:]
			reference, err := decodeAuditBodyReference(encodeAuditBodyReference(bytes.Repeat([]byte{1}, auditBodyHashLen), start, []byte(envelope)))
			if err != nil {
				t.Fatalf("decode reference: %v", err)
			}
			if got := string(reference.splice([]byte(tt.wantResult))); got != tt.body {
				t.Fatalf("rebuilt body = %q, want %q", got, tt.body)
			}
		})
	}
}

func TestIdenticalListResultsAreStoredOnce(t *testing.T) {
	for _, mode := range auditEncryptionModes {
		t.Run(mode.name, func(t *testing.T) {
			c := mode.newClient(t)
			result := testListResult("shared", 8192)
			first, second := testListResponse(7, result), testListResponse(8, result)

			storeMCPAuditEntries(t, c, listCallEntries(7, "tools/list", first)...)
			storeMCPAuditEntries(t, c, listCallEntries(8, "tools/list", second)...)
			// An entry that arrives complete carries the method itself rather than a hint, and shares
			// the same stored body regardless of the server or user it belongs to.
			complete := testListResponse(9, result)
			storeMCPAuditEntries(t, c, types.MCPAuditLog{
				CreatedAt: time.Now().UTC().Add(time.Second),
				UserID:    "user-2",
				MCPFields: &types.MCPAuditLogFields{
					MCPID:            "mcp-2",
					RequestID:        "9",
					CallType:         "tools/list",
					RequestBody:      json.RawMessage(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}`),
					ResponseReceived: true,
					ResponseBody:     complete,
				},
			})

			if got := countAuditBodies(t, c); got != 1 {
				t.Fatalf("stored %d shared bodies, want 1", got)
			}

			var rows []types.MCPAuditLog
			if err := c.db.WithContext(t.Context()).Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				stored := row.MCP().ResponseBody
				if !isEncodedAuditBody(stored) || stored[2]&auditBodyFlagReference == 0 {
					t.Fatalf("row %d does not store a reference", row.ID)
				}
				if len(stored) > 512 {
					t.Fatalf("row %d stores %d bytes for a reference", row.ID, len(stored))
				}
			}

			logs := readMCPAuditLogs(t, c)
			if len(logs) != 3 {
				t.Fatalf("read %d audit rows, want 3", len(logs))
			}
			for i, want := range []json.RawMessage{first, second, complete} {
				if got := logs[i].MCP().ResponseBody; !bytes.Equal(got, want) {
					t.Fatalf("row %d response body was not rebuilt byte for byte:\n got %.120s\nwant %.120s", i, got, want)
				}
				if len(logs[i].UnavailablePayloads) != 0 {
					t.Fatalf("row %d reports unavailable payloads %v", i, logs[i].UnavailablePayloads)
				}
			}

			single, err := c.GetMCPAuditLog(t.Context(), logs[0].ID, true)
			if err != nil {
				t.Fatalf("read single audit log: %v", err)
			}
			if !bytes.Equal(single.MCP().ResponseBody, first) {
				t.Fatal("single audit log response body was not rebuilt")
			}
		})
	}
}

func TestDifferentListResultsAreStoredSeparately(t *testing.T) {
	for _, mode := range auditEncryptionModes {
		t.Run(mode.name, func(t *testing.T) {
			c := mode.newClient(t)
			first := testListResponse(1, testListResult("alpha", 4096))
			second := testListResponse(2, testListResult("beta", 4096))
			storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", first)...)
			storeMCPAuditEntries(t, c, listCallEntries(2, "prompts/list", second)...)

			if got := countAuditBodies(t, c); got != 2 {
				t.Fatalf("stored %d shared bodies, want 2", got)
			}
			logs := readMCPAuditLogs(t, c)
			if !bytes.Equal(logs[0].MCP().ResponseBody, first) || !bytes.Equal(logs[1].MCP().ResponseBody, second) {
				t.Fatal("response bodies were not rebuilt from their own shared bodies")
			}
		})
	}
}

func TestOriginalAndMutatedListResponsesAreBothShared(t *testing.T) {
	for _, mode := range auditEncryptionModes {
		t.Run(mode.name, func(t *testing.T) {
			c := mode.newClient(t)
			original := testListResponse(3, testListResult("original", 4096))
			filtered := testListResponse(3, testListResult("filtered", 4096))
			entries := listCallEntries(3, "tools/list", filtered)
			entries[1].MCPFields.OriginalResponseBody = original
			storeMCPAuditEntries(t, c, entries...)

			if got := countAuditBodies(t, c); got != 2 {
				t.Fatalf("stored %d shared bodies, want 2", got)
			}
			logs := readMCPAuditLogs(t, c)
			if !bytes.Equal(logs[0].MCP().ResponseBody, filtered) || !bytes.Equal(logs[0].MCP().OriginalResponseBody, original) {
				t.Fatal("mutated and original response bodies were not both rebuilt")
			}
		})
	}
}

func TestBodiesThatAreNotLargeListResultsAreStoredInline(t *testing.T) {
	large := testListResult("inline", 8192)

	tests := []struct {
		name           string
		method         string
		responseMethod string
		response       json.RawMessage
	}{
		{
			name:           "tool call",
			method:         "tools/call",
			responseMethod: "tools/call",
			response:       testListResponse(1, large),
		},
		{
			name:           "list error response",
			method:         "tools/list",
			responseMethod: "tools/list",
			response:       json.RawMessage(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"` + strings.Repeat("x", 4096) + `"}}`),
		},
		{
			name:           "list response that is not JSON-RPC",
			method:         "tools/list",
			responseMethod: "tools/list",
			response:       json.RawMessage(`"` + strings.Repeat("event: message ", 400) + `"`),
		},
		{
			name:           "small list result",
			method:         "tools/list",
			responseMethod: "tools/list",
			response:       json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`),
		},
		{
			name:           "list response whose method is not known",
			method:         "tools/list",
			responseMethod: "",
			response:       testListResponse(1, large),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, mode := range auditEncryptionModes {
				t.Run(mode.name, func(t *testing.T) {
					c := mode.newClient(t)
					entries := listCallEntries(1, tt.method, tt.response)
					entries[1].MCPFields.ResponseMethod = tt.responseMethod
					storeMCPAuditEntries(t, c, entries...)

					if got := countAuditBodies(t, c); got != 0 {
						t.Fatalf("stored %d shared bodies, want none", got)
					}
					stored := storedResponseBody(t, c)
					if !isEncodedAuditBody(stored) || stored[2]&auditBodyFlagReference != 0 {
						t.Fatal("response body is not stored inline in the current format")
					}
					logs := readMCPAuditLogs(t, c)
					if !bytes.Equal(logs[0].MCP().ResponseBody, tt.response) {
						t.Fatalf("response body did not round trip: got %.120s", logs[0].MCP().ResponseBody)
					}
				})
			}
		})
	}
}

func TestEveryBodyFieldRoundTrips(t *testing.T) {
	for _, mode := range auditEncryptionModes {
		t.Run(mode.name, func(t *testing.T) {
			c := mode.newClient(t)
			padding := strings.Repeat("compressible ", 100)
			mcpEntry := types.MCPAuditLog{
				CreatedAt: time.Now().UTC(),
				UserID:    "user-1",
				MCPFields: &types.MCPAuditLogFields{
					MCPID:                "mcp-1",
					RequestID:            "1",
					CallType:             "tools/call",
					ResponseReceived:     true,
					RequestBody:          json.RawMessage(`{"request":"` + padding + `"}`),
					MutatedRequestBody:   json.RawMessage(`{"mutatedRequest":"` + padding + `"}`),
					ResponseBody:         json.RawMessage(`{"response":"` + padding + `"}`),
					OriginalResponseBody: json.RawMessage(`{"originalResponse":"` + padding + `"}`),
					RequestHeaders:       json.RawMessage(`{"accept":"application/json"}`),
					ResponseHeaders:      json.RawMessage(`{"content-type":"application/json"}`),
				},
			}
			want := *mcpEntry.MCPFields
			storeMCPAuditEntries(t, c, mcpEntry)

			var stored types.MCPAuditLog
			if err := c.db.WithContext(t.Context()).First(&stored).Error; err != nil {
				t.Fatal(err)
			}
			for name, field := range map[string]json.RawMessage{
				"request_body":           stored.MCP().RequestBody,
				"mutated_request_body":   stored.MCP().MutatedRequestBody,
				"response_body":          stored.MCP().ResponseBody,
				"original_response_body": stored.MCP().OriginalResponseBody,
			} {
				if !isEncodedAuditBody(field) || field[2]&auditBodyFlagCompressed == 0 {
					t.Fatalf("%s is not stored compressed", name)
				}
			}

			got := readMCPAuditLogs(t, c)[0].MCP()
			if !bytes.Equal(got.RequestBody, want.RequestBody) ||
				!bytes.Equal(got.MutatedRequestBody, want.MutatedRequestBody) ||
				!bytes.Equal(got.ResponseBody, want.ResponseBody) ||
				!bytes.Equal(got.OriginalResponseBody, want.OriginalResponseBody) ||
				!bytes.Equal(got.RequestHeaders, want.RequestHeaders) ||
				!bytes.Equal(got.ResponseHeaders, want.ResponseHeaders) {
				t.Fatalf("MCP payload fields did not round trip: %#v", got)
			}

			local := validLocalAgentAuditLog(time.Now().UTC(), "body-round-trip", types2.AuditLogOutcomeStatusSuccess)
			local.LocalAgentToolCallFields.RequestBody = json.RawMessage(`{"toolInput":"` + padding + `"}`)
			local.LocalAgentToolCallFields.ResponseBody = json.RawMessage(`{"toolOutput":"` + padding + `"}`)
			local.LocalAgentToolCallFields.RawEvent = json.RawMessage(`{"rawEvent":"` + padding + `"}`)
			wantLocal := *local.LocalAgentToolCallFields
			if err := c.InsertLocalAgentAuditLogs(t.Context(), []types.MCPAuditLog{local}); err != nil {
				t.Fatalf("insert local-agent audit log: %v", err)
			}

			var storedLocal types.MCPAuditLog
			if err := c.db.WithContext(t.Context()).Where("source_type = ?", types2.AuditLogSourceTypeLocalAgentToolCall).First(&storedLocal).Error; err != nil {
				t.Fatal(err)
			}
			for name, field := range map[string]json.RawMessage{
				"local_agent_request_body":  storedLocal.LocalAgentToolCallFields.RequestBody,
				"local_agent_response_body": storedLocal.LocalAgentToolCallFields.ResponseBody,
				"local_agent_raw_event":     storedLocal.LocalAgentToolCallFields.RawEvent,
			} {
				if !isEncodedAuditBody(field) || field[2]&auditBodyFlagCompressed == 0 {
					t.Fatalf("%s is not stored compressed", name)
				}
			}

			logs, _, err := c.GetMCPAuditLogs(t.Context(), MCPAuditLogOptions{
				WithRequestAndResponse: true,
				SourceTypes:            []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
			})
			if err != nil || len(logs) != 1 {
				t.Fatalf("read local-agent audit log: %v (%d rows)", err, len(logs))
			}
			gotLocal := logs[0].LocalAgentToolCallFields
			if !bytes.Equal(gotLocal.RequestBody, wantLocal.RequestBody) ||
				!bytes.Equal(gotLocal.ResponseBody, wantLocal.ResponseBody) ||
				!bytes.Equal(gotLocal.RawEvent, wantLocal.RawEvent) ||
				gotLocal.Hostname != wantLocal.Hostname {
				t.Fatalf("local-agent payload fields did not round trip: %#v", gotLocal)
			}
		})
	}
}

// legacyEncryptedBody returns body as versions before the field format stored it under encryption.
func legacyEncryptedBody(t *testing.T, c *Client, log *types.MCPAuditLog, body string) json.RawMessage {
	t.Helper()
	field := json.RawMessage(body)
	if err := encryptRawMessageField(t.Context(), c.mcpAuditLogTransformer(), mcpAuditLogDataCtx(log), &field); err != nil {
		t.Fatalf("encrypt legacy body: %v", err)
	}
	return field
}

func TestRowsInTheLegacyFormatStillRead(t *testing.T) {
	const (
		requestBody  = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		responseBody = `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`
	)

	t.Run("unencrypted", func(t *testing.T) {
		c := newTestClient(t)
		row := types.MCPAuditLog{
			CreatedAt: time.Now().UTC(),
			UserID:    "user-1",
			MCPFields: &types.MCPAuditLogFields{
				MCPID:        "mcp-1",
				CallType:     "tools/list",
				RequestBody:  json.RawMessage(requestBody),
				ResponseBody: json.RawMessage(responseBody),
			},
		}
		row.NormalizeMCPFields()
		if err := c.db.WithContext(t.Context()).Create(&row).Error; err != nil {
			t.Fatal(err)
		}

		got := readMCPAuditLogs(t, c)[0].MCP()
		if string(got.RequestBody) != requestBody || string(got.ResponseBody) != responseBody {
			t.Fatalf("legacy unencrypted row did not read: %#v", got)
		}
	})

	t.Run("encrypted", func(t *testing.T) {
		c := newTestClient(t)
		c.encryptionConfig = contextBoundEncryptionConfig()
		row := types.MCPAuditLog{
			CreatedAt: time.Now().UTC(),
			UserID:    "user-1",
			Encrypted: true,
			MCPFields: &types.MCPAuditLogFields{
				MCPID:    "mcp-1",
				CallType: "tools/list",
			},
		}
		row.NormalizeMCPFields()
		row.MCP().RequestBody = legacyEncryptedBody(t, c, &row, requestBody)
		row.MCP().ResponseBody = legacyEncryptedBody(t, c, &row, responseBody)
		if err := c.db.WithContext(t.Context()).Create(&row).Error; err != nil {
			t.Fatal(err)
		}

		got := readMCPAuditLogs(t, c)[0].MCP()
		if string(got.RequestBody) != requestBody || string(got.ResponseBody) != responseBody {
			t.Fatalf("legacy encrypted row did not read: %#v", got)
		}
	})

	// During a rolling upgrade, an old replica can insert the request and a new one add the response.
	t.Run("legacy request with a current-format response", func(t *testing.T) {
		c := newTestClient(t)
		c.encryptionConfig = contextBoundEncryptionConfig()
		response := testListResponse(1, testListResult("mixed", 4096))
		entries := listCallEntries(1, "tools/list", response)

		request := entries[0]
		request.NormalizeMCPFields()
		request.Encrypted = true
		request.MCP().RequestBody = legacyEncryptedBody(t, c, &request, requestBody)
		if err := c.db.WithContext(t.Context()).Create(&request).Error; err != nil {
			t.Fatal(err)
		}
		storeMCPAuditEntries(t, c, entries[1])

		logs := readMCPAuditLogs(t, c)
		if len(logs) != 1 {
			t.Fatalf("response was not merged into the request row: %d rows", len(logs))
		}
		got := logs[0].MCP()
		if string(got.RequestBody) != requestBody || !bytes.Equal(got.ResponseBody, response) {
			t.Fatalf("mixed-format row did not read: request=%s response=%.80s", got.RequestBody, got.ResponseBody)
		}
	})

	t.Run("legacy decrypt failure still fails the read", func(t *testing.T) {
		c := newTestClient(t)
		c.encryptionConfig = contextBoundEncryptionConfig()
		row := types.MCPAuditLog{
			CreatedAt: time.Now().UTC(),
			UserID:    "user-1",
			Encrypted: true,
			MCPFields: &types.MCPAuditLogFields{
				MCPID:       "mcp-1",
				CallType:    "tools/call",
				RequestBody: json.RawMessage(`bm90IGVuY3J5cHRlZCBmb3IgdGhpcyByb3c=`),
			},
		}
		row.NormalizeMCPFields()
		if err := c.db.WithContext(t.Context()).Create(&row).Error; err != nil {
			t.Fatal(err)
		}

		if _, _, err := c.GetMCPAuditLogs(t.Context(), MCPAuditLogOptions{WithRequestAndResponse: true}); err == nil {
			t.Fatal("expected the legacy decrypt failure to fail the read")
		}
	})
}

func TestUnreadableBodiesAreReportedOnTheRow(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(t *testing.T, c *Client)
		want    []string
	}{
		{
			name: "shared body is missing",
			corrupt: func(t *testing.T, c *Client) {
				t.Helper()
				if err := c.db.WithContext(t.Context()).Where("1 = 1").Delete(&types.MCPAuditLogBody{}).Error; err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"response.body"},
		},
		{
			name: "shared body is corrupt",
			corrupt: func(t *testing.T, c *Client) {
				t.Helper()
				corrupt := []byte{auditBodyMarker, auditBodyVersion, auditBodyFlagCompressed, 'b', 'a', 'd'}
				if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", corrupt).Error; err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"response.body"},
		},
		{
			name: "inline body is corrupt",
			corrupt: func(t *testing.T, c *Client) {
				t.Helper()
				corrupt := []byte{auditBodyMarker, auditBodyVersion, auditBodyFlagCompressed, 'b', 'a', 'd'}
				if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLog{}).Where("1 = 1").Update("request_body", corrupt).Error; err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"request.body"},
		},
		{
			name: "reference is corrupt",
			corrupt: func(t *testing.T, c *Client) {
				t.Helper()
				corrupt := []byte{auditBodyMarker, auditBodyVersion, auditBodyFlagReference, 'x'}
				if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLog{}).Where("1 = 1").Update("response_body", corrupt).Error; err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"response.body"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			response := testListResponse(1, testListResult("unreadable", 4096))
			storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", response)...)
			tt.corrupt(t, c)

			logs := readMCPAuditLogs(t, c)
			if len(logs) != 1 {
				t.Fatalf("read %d rows, want the row to still be returned", len(logs))
			}
			log := logs[0]
			if !slices.Equal(log.UnavailablePayloads, tt.want) {
				t.Fatalf("unavailable payloads = %v, want %v", log.UnavailablePayloads, tt.want)
			}
			if log.MCP().CallType != "tools/list" || log.MCP().ResponseStatus != 200 {
				t.Fatalf("row metadata was lost: %#v", log.MCP())
			}
			unavailableRequest := slices.Contains(tt.want, "request.body")
			if (len(log.MCP().RequestBody) == 0) != unavailableRequest {
				t.Fatalf("request body presence does not match its availability: %q", log.MCP().RequestBody)
			}
			if unavailableResponse := slices.Contains(tt.want, "response.body"); unavailableResponse {
				if len(log.MCP().ResponseBody) != 0 {
					t.Fatal("unreadable response body was returned")
				}
			} else if !bytes.Equal(log.MCP().ResponseBody, response) {
				t.Fatal("readable response body was not returned")
			}

			single, err := c.GetMCPAuditLog(t.Context(), log.ID, true)
			if err != nil {
				t.Fatalf("read single audit log: %v", err)
			}
			if !slices.Equal(single.UnavailablePayloads, tt.want) {
				t.Fatalf("single audit log unavailable payloads = %v, want %v", single.UnavailablePayloads, tt.want)
			}
		})
	}
}

func loadAuditBody(t *testing.T, c *Client) types.MCPAuditLogBody {
	t.Helper()
	var body types.MCPAuditLogBody
	if err := c.db.WithContext(t.Context()).First(&body).Error; err != nil {
		t.Fatalf("load shared audit body: %v", err)
	}
	return body
}

func setAuditBodyLastReferenced(t *testing.T, c *Client, at time.Time) {
	t.Helper()
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("last_referenced_at", at).Error; err != nil {
		t.Fatalf("set last referenced time: %v", err)
	}
}

func TestReferencingASharedBodyRefreshesItOnlyWhenStale(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("refresh", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)

	recent := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	setAuditBodyLastReferenced(t, c, recent)
	storeMCPAuditEntries(t, c, listCallEntries(2, "tools/list", testListResponse(2, result))...)
	if got := loadAuditBody(t, c).LastReferencedAt; !got.Equal(recent) {
		t.Fatalf("recently referenced body was rewritten: last referenced %v, want %v", got, recent)
	}

	stale := time.Now().UTC().Add(-3 * auditBodyReferenceRefresh).Truncate(time.Second)
	setAuditBodyLastReferenced(t, c, stale)
	storeMCPAuditEntries(t, c, listCallEntries(3, "tools/list", testListResponse(3, result))...)
	if got := loadAuditBody(t, c).LastReferencedAt; time.Since(got) > time.Minute {
		t.Fatalf("stale body was not refreshed: last referenced %v", got)
	}
	if got := countAuditBodies(t, c); got != 1 {
		t.Fatalf("stored %d shared bodies, want 1", got)
	}
}

// A replica that believes a body is stored skips encoding it again. If retention removed the body
// in the meantime, the write must still produce a row whose body can be read.
func TestSharedBodyIsRestoredWhenAReplicaWronglyBelievesItIsStored(t *testing.T) {
	for _, mode := range auditEncryptionModes {
		t.Run(mode.name, func(t *testing.T) {
			c := mode.newClient(t)
			result := testListResult("believed", 4096)
			storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)

			if err := c.db.WithContext(t.Context()).Where("1 = 1").Delete(&types.MCPAuditLogBody{}).Error; err != nil {
				t.Fatal(err)
			}
			if err := c.db.WithContext(t.Context()).Where("1 = 1").Delete(&types.MCPAuditLog{}).Error; err != nil {
				t.Fatal(err)
			}

			second := testListResponse(2, result)
			entries := listCallEntries(2, "tools/list", second)
			for i := range entries {
				entries[i].NormalizeMCPFields()
				if err := c.encryptMCPAuditLog(t.Context(), &entries[i]); err != nil {
					t.Fatal(err)
				}
			}
			pending := entries[1].PendingBodies
			if len(pending) != 1 || pending[0].Encoded != nil || pending[0].Plain == nil {
				t.Fatalf("expected encoding to be skipped for a body believed stored, got %d pending bodies", len(pending))
			}
			if err := c.insertMCPAuditLogs(t.Context(), entries); err != nil {
				t.Fatal(err)
			}

			if got := countAuditBodies(t, c); got != 1 {
				t.Fatalf("stored %d shared bodies, want the missing one restored", got)
			}
			logs := readMCPAuditLogs(t, c)
			if len(logs[0].UnavailablePayloads) != 0 || !bytes.Equal(logs[0].MCP().ResponseBody, second) {
				t.Fatalf("row written after the body was removed cannot be read: unavailable=%v", logs[0].UnavailablePayloads)
			}
		})
	}
}

func TestLosingTheHashKeyStoresANewCopyWithoutBreakingReads(t *testing.T) {
	for _, mode := range auditEncryptionModes {
		t.Run(mode.name, func(t *testing.T) {
			c := mode.newClient(t)
			result := testListResult("rekey", 4096)
			first, second := testListResponse(1, result), testListResponse(2, result)
			storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", first)...)

			if err := c.DeleteProperty(t.Context(), auditBodyHashKeyProperty); err != nil {
				t.Fatal(err)
			}
			c.auditBodies.hashKey = nil
			c.auditBodies.knownStored = nil
			storeMCPAuditEntries(t, c, listCallEntries(2, "tools/list", second)...)

			if got := countAuditBodies(t, c); got != 2 {
				t.Fatalf("stored %d shared bodies, want a second copy under the new key", got)
			}
			logs := readMCPAuditLogs(t, c)
			if !bytes.Equal(logs[0].MCP().ResponseBody, first) || !bytes.Equal(logs[1].MCP().ResponseBody, second) {
				t.Fatal("rows written under the old and new keys did not both read")
			}
		})
	}
}

func TestListResponseIsStoredInlineWhenTheHashKeyIsUnavailable(t *testing.T) {
	c := newTestClient(t)
	// A stored key of the wrong length cannot be used.
	if _, err := c.SetProperty(t.Context(), auditBodyHashKeyProperty, "bm90IGEga2V5"); err != nil {
		t.Fatal(err)
	}
	response := testListResponse(1, testListResult("nokey", 4096))
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", response)...)

	if got := countAuditBodies(t, c); got != 0 {
		t.Fatalf("stored %d shared bodies without a usable hash key", got)
	}
	if logs := readMCPAuditLogs(t, c); !bytes.Equal(logs[0].MCP().ResponseBody, response) {
		t.Fatal("inline response body did not round trip")
	}
}

func TestAuditBodyHashKeyIsCreatedOnceAndReused(t *testing.T) {
	c := newTestClient(t)
	key, err := c.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != auditBodyHashLen {
		t.Fatalf("hash key is %d bytes, want %d", len(key), auditBodyHashLen)
	}

	// Another replica shares the database but not the in-memory copy.
	other := &Client{db: c.db}
	otherKey, err := other.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, otherKey) {
		t.Fatal("a second client generated its own hash key instead of reusing the stored one")
	}
}

func TestRetentionDeletesOnlyBodiesNoRowCanReference(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	const retentionDays = 30
	now := time.Now().UTC()
	rowCutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)

	bodies := []struct {
		name             string
		lastReferencedAt time.Time
		wantKept         bool
	}{
		{
			name:             "referenced today",
			lastReferencedAt: now,
			wantKept:         true,
		},
		{
			name:             "referenced just inside row retention",
			lastReferencedAt: rowCutoff.Add(time.Minute),
			wantKept:         true,
		},
		{
			name:             "referenced within the margin before the row cutoff",
			lastReferencedAt: rowCutoff.Add(-auditBodyCleanupMargin + time.Minute),
			wantKept:         true,
		},
		{
			name:             "referenced before the margin",
			lastReferencedAt: rowCutoff.Add(-auditBodyCleanupMargin - time.Minute),
			wantKept:         false,
		},
		{
			name:             "referenced long ago",
			lastReferencedAt: rowCutoff.AddDate(0, 0, -60),
			wantKept:         false,
		},
	}
	// More stale bodies than one delete batch, so the batching loop is exercised.
	for i := range c.auditLogDeleteBatchSize * 2 {
		bodies = append(bodies, struct {
			name             string
			lastReferencedAt time.Time
			wantKept         bool
		}{
			name:             fmt.Sprintf("stale filler %d", i),
			lastReferencedAt: rowCutoff.AddDate(0, 0, -90),
			wantKept:         false,
		})
	}
	for i, body := range bodies {
		hash := bytes.Repeat([]byte{byte(i + 1)}, auditBodyHashLen)
		if err := c.db.WithContext(ctx).Create(&types.MCPAuditLogBody{
			Hash:             hash,
			Body:             []byte(body.name),
			CreatedAt:        body.lastReferencedAt,
			LastReferencedAt: body.lastReferencedAt,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := c.cleanupRetainedData(ctx, now, retentionDays, 0); err != nil {
		t.Fatalf("retention cleanup: %v", err)
	}

	var remaining []types.MCPAuditLogBody
	if err := c.db.WithContext(ctx).Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	kept := make(map[string]bool, len(remaining))
	for _, body := range remaining {
		kept[string(body.Body)] = true
	}
	for _, body := range bodies {
		if kept[body.name] != body.wantKept {
			t.Errorf("body %q kept = %v, want %v", body.name, kept[body.name], body.wantKept)
		}
	}
}

// A row that survives retention must keep its body: the write leaves the body's last-referenced
// time within the refresh interval of the write, which the cleanup margin covers.
func TestRetentionKeepsBodiesOfSurvivingRows(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	const retentionDays = 1
	response := testListResponse(1, testListResult("surviving", 4096))
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", response)...)

	// The oldest state a write can leave behind: the body was last refreshed a full interval
	// before the row was written, and the row is as old as retention allows.
	now := time.Now().UTC()
	rowCutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)
	if err := c.db.WithContext(ctx).Model(&types.MCPAuditLog{}).Where("1 = 1").Update("created_at", rowCutoff).Error; err != nil {
		t.Fatal(err)
	}
	setAuditBodyLastReferenced(t, c, rowCutoff.Add(-auditBodyReferenceRefresh))

	if err := c.cleanupRetainedData(ctx, now, retentionDays, 0); err != nil {
		t.Fatalf("retention cleanup: %v", err)
	}

	logs := readMCPAuditLogs(t, c)
	if len(logs) != 1 {
		t.Fatalf("surviving row was deleted: %d rows", len(logs))
	}
	if len(logs[0].UnavailablePayloads) != 0 || !bytes.Equal(logs[0].MCP().ResponseBody, response) {
		t.Fatalf("surviving row lost its body: unavailable=%v", logs[0].UnavailablePayloads)
	}
}

func TestRetentionDisabledKeepsSharedBodies(t *testing.T) {
	c := newTestClient(t)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, testListResult("kept", 4096)))...)
	setAuditBodyLastReferenced(t, c, time.Now().UTC().AddDate(-1, 0, 0))

	if err := c.cleanupRetainedData(t.Context(), time.Now().UTC(), 0, 0); err != nil {
		t.Fatal(err)
	}
	if got := countAuditBodies(t, c); got != 1 {
		t.Fatalf("shared body was deleted with retention disabled: %d remain", got)
	}
}
