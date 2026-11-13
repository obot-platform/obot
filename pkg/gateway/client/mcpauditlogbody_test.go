package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

const (
	// rotatingNonceLen is the size of the nonce rotatingTransformer puts in front of what it
	// encrypts.
	rotatingNonceLen = 8
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

// unavailableBodyLogEntry is the logged summary of the bodies one read could not load.
type unavailableBodyLogEntry struct {
	Message string   `json:"msg"`
	Count   int      `json:"count"`
	Causes  []string `json:"causes"`
}

// lockedWriter serializes writes from whatever goroutines happen to log during a test.
type lockedWriter struct {
	lock *sync.Mutex
	w    *bytes.Buffer
}

// failingOnceTransformer fails its first decryption, as an encryption provider that is briefly
// unreachable would, and works from then on.
type failingOnceTransformer struct {
	rotatingTransformer
	failures *int
}

// hookedTransformer runs a function the first time it encrypts, which lets a test act between a
// replica reading a stored value and writing its replacement.
type hookedTransformer struct {
	rotatingTransformer
	beforeFirstEncrypt *func()
}

// cancellingTransformer cancels a context and fails, as a call to an encryption provider does
// when the time allowed for capturing an entry runs out during it.
type cancellingTransformer struct {
	rotatingTransformer
	cancel context.CancelFunc
}

// rotatingTransformer stands in for an encryption configuration with several keys: it writes
// under the first and can read any of them, reporting data under another key as stale.
type rotatingTransformer struct {
	keys []string
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.w.Write(p)
}

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
			// The second capture reads the stored copy, and the replica remembers it is usable.
			storeMCPAuditEntries(t, c, listCallEntries(3, "tools/list", testListResponse(3, result))...)

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
			c.auditBodies.known = nil
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

func (r rotatingTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	// A random nonce makes every encryption different, as a real cipher's would, so a test can
	// tell a stored copy that was written again from one that was left alone.
	nonce := make([]byte, rotatingNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(append(rotatingKeyPrefix(r.keys[0]), nonce...), data...), nil
}

func (r rotatingTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	for i, key := range r.keys {
		if rest, ok := bytes.CutPrefix(data, rotatingKeyPrefix(key)); ok && len(rest) >= rotatingNonceLen {
			return rest[rotatingNonceLen:], i != 0, nil
		}
	}
	return nil, false, errors.New("no configured key can decrypt this data")
}

func rotatingKeyPrefix(key string) []byte {
	return []byte("key[" + key + "]:")
}

func useRotatingKeys(c *Client, keys ...string) {
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			mcpAuditLogGroupResource: rotatingTransformer{keys: keys},
		},
	}
}

// rotatingKeysClient returns another replica on c's database that encrypts both audit logs and
// properties, as the shipped encryption configurations do.
func rotatingKeysClient(c *Client, keys ...string) *Client {
	return &Client{
		db: c.db,
		encryptionConfig: &encryptionconfig.EncryptionConfiguration{
			Transformers: map[schema.GroupResource]value.Transformer{
				mcpAuditLogGroupResource: rotatingTransformer{keys: keys},
				propertyGroupResource:    rotatingTransformer{keys: keys},
			},
		},
	}
}

// loadAuditBodies returns every shared body, split by whether it is stored encrypted.
func loadAuditBodies(t *testing.T, c *Client) (plain, encrypted []types.MCPAuditLogBody) {
	t.Helper()
	var bodies []types.MCPAuditLogBody
	if err := c.db.WithContext(t.Context()).Find(&bodies).Error; err != nil {
		t.Fatalf("load shared audit bodies: %v", err)
	}
	for _, body := range bodies {
		if body.Body[2]&auditBodyFlagEncrypted != 0 {
			encrypted = append(encrypted, body)
		} else {
			plain = append(plain, body)
		}
	}
	return plain, encrypted
}

// A list result first stored without encryption must not be what rows reference once encryption
// is on. Writers that encrypt hash the same result differently, so they store their own copy.
func TestEnablingEncryptionStoresASeparateEncryptedBody(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("enable", 4096)
	first, second, third := testListResponse(1, result), testListResponse(2, result), testListResponse(3, result)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", first)...)
	plainBefore, _ := loadAuditBodies(t, c)

	c.encryptionConfig = contextBoundEncryptionConfig()
	storeMCPAuditEntries(t, c, listCallEntries(2, "tools/list", second)...)
	storeMCPAuditEntries(t, c, listCallEntries(3, "tools/list", third)...)

	plain, encrypted := loadAuditBodies(t, c)
	if len(plain) != 1 || len(encrypted) != 1 {
		t.Fatalf("stored %d unencrypted and %d encrypted bodies, want one of each", len(plain), len(encrypted))
	}
	if !bytes.Equal(plain[0].Body, plainBefore[0].Body) || !plain[0].LastReferencedAt.Equal(plainBefore[0].LastReferencedAt) {
		t.Fatal("the unencrypted body was touched by a writer that encrypts")
	}
	logs := readMCPAuditLogs(t, c)
	for i, want := range []json.RawMessage{first, second, third} {
		if len(logs[i].UnavailablePayloads) != 0 || !bytes.Equal(logs[i].MCP().ResponseBody, want) {
			t.Fatalf("row %d did not read: unavailable=%v", i, logs[i].UnavailablePayloads)
		}
	}
}

// A writer without the encryption configuration must neither rewrite an encrypted body in the
// clear nor reference one it cannot read. This is the case where it can still read the hash key,
// because the key is not stored encrypted; where it cannot, it does not share bodies at all (see
// TestHashKeyStoredBeforeEncryptionIsReplaced).
func TestUnencryptedWriterLeavesEncryptedSharedBodyAlone(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = contextBoundEncryptionConfig()
	result := testListResult("mixed", 4096)
	first := testListResponse(1, result)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", first)...)
	_, encryptedBefore := loadAuditBodies(t, c)

	unencryptedWriter := &Client{db: c.db}
	second := testListResponse(2, result)
	storeMCPAuditEntries(t, unencryptedWriter, listCallEntries(2, "tools/list", second)...)

	plain, encrypted := loadAuditBodies(t, c)
	if len(plain) != 1 || len(encrypted) != 1 {
		t.Fatalf("stored %d unencrypted and %d encrypted bodies, want one of each", len(plain), len(encrypted))
	}
	if !bytes.Equal(encrypted[0].Body, encryptedBefore[0].Body) {
		t.Fatal("the encrypted body was rewritten by a writer that does not encrypt")
	}

	// The unencrypted writer can read back what it wrote, and the encrypting one reads both.
	own, err := unencryptedWriter.GetMCPAuditLog(t.Context(), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(own.UnavailablePayloads) != 0 || !bytes.Equal(own.MCP().ResponseBody, second) {
		t.Fatalf("unencrypted writer cannot read its own row: unavailable=%v", own.UnavailablePayloads)
	}
	logs := readMCPAuditLogs(t, c)
	if !bytes.Equal(logs[0].MCP().ResponseBody, first) || !bytes.Equal(logs[1].MCP().ResponseBody, second) {
		t.Fatal("encrypting client did not read both rows")
	}
}

// Captured content that happens to look like an encoded field must be stored like any other body.
// If the encoder passed it through, it would land in the database unencrypted and the reader
// would act on a header the content chose.
func TestBodyThatLooksEncodedIsStillEncoded(t *testing.T) {
	dataCtx := value.DefaultContext("lookalike")
	lookalike := append([]byte{auditBodyMarker, auditBodyVersion, auditBodyFlagReference}, bytes.Repeat([]byte{7}, auditBodyHashLen+8)...)

	field := json.RawMessage(bytes.Clone(lookalike))
	if err := encodeAuditBodyField(t.Context(), contextBoundTransformer{}, dataCtx, &field); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(field, lookalike) {
		t.Fatal("content that looks encoded was stored as is")
	}
	if field[2]&auditBodyFlagEncrypted == 0 || field[2]&auditBodyFlagReference != 0 {
		t.Fatalf("stored field has flags %#x, want encrypted and not a reference", field[2])
	}
	plain, flags, err := decodeAuditBody(t.Context(), contextBoundTransformer{}, dataCtx, field)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, lookalike) || flags&auditBodyFlagReference != 0 {
		t.Fatal("content that looks encoded did not round trip as an ordinary body")
	}

	// The same holds on the path list responses take.
	c := newTestClient(t)
	c.encryptionConfig = contextBoundEncryptionConfig()
	log := types.MCPAuditLog{
		UserID: "user-1",
		MCPFields: &types.MCPAuditLogFields{
			MCPID:        "mcp-1",
			CallType:     "tools/list",
			ResponseBody: json.RawMessage(bytes.Clone(lookalike)),
		},
	}
	log.NormalizeMCPFields()
	if err := c.encryptMCPAuditLog(t.Context(), &log); err != nil {
		t.Fatal(err)
	}
	stored := log.MCP().ResponseBody
	if bytes.Equal(stored, lookalike) || stored[2]&auditBodyFlagEncrypted == 0 || stored[2]&auditBodyFlagReference != 0 {
		t.Fatalf("list response that looks encoded was not stored as an ordinary encrypted body (flags %#x)", stored[2])
	}
	if len(log.PendingBodies) != 0 {
		t.Fatal("list response that looks encoded produced a shared body")
	}
}

// captureLogs redirects logging for the rest of the test and returns a function giving the JSON
// log entries with the given message logged so far.
func captureLogs(t *testing.T) func(message string) []string {
	t.Helper()
	var (
		lock   sync.Mutex
		logged bytes.Buffer
	)
	// slog.SetDefault also redirects the standard log package, and restoring the previous slog
	// default does not undo that, so both are put back.
	previous, previousWriter, previousFlags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(lockedWriter{lock: &lock, w: &logged}, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})

	return func(message string) []string {
		t.Helper()
		lock.Lock()
		defer lock.Unlock()
		var entries []string
		for line := range strings.SplitSeq(logged.String(), "\n") {
			var entry struct {
				Message string `json:"msg"`
			}
			if line == "" || json.Unmarshal([]byte(line), &entry) != nil || entry.Message != message {
				continue
			}
			entries = append(entries, line)
		}
		return entries
	}
}

// captureUnavailableBodyLogs is captureLogs for the summary of bodies a read could not load.
func captureUnavailableBodyLogs(t *testing.T) func() []unavailableBodyLogEntry {
	t.Helper()
	logs := captureLogs(t)
	return func() []unavailableBodyLogEntry {
		t.Helper()
		var entries []unavailableBodyLogEntry
		for _, line := range logs("MCP audit log bodies are unavailable") {
			var entry unavailableBodyLogEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("unreadable log entry %q: %v", line, err)
			}
			entries = append(entries, entry)
		}
		return entries
	}
}

// Unreadable bodies are reported once per read, not once per body: a bad key makes every body on
// a page unreadable.
func TestUnreadableBodiesAreLoggedOncePerRead(t *testing.T) {
	result := testListResult("logged", 4096)

	t.Run("shared bodies are missing", func(t *testing.T) {
		c := newTestClient(t)
		for id := 1; id <= 3; id++ {
			storeMCPAuditEntries(t, c, listCallEntries(id, "tools/list", testListResponse(id, result))...)
		}
		if err := c.db.WithContext(t.Context()).Where("1 = 1").Delete(&types.MCPAuditLogBody{}).Error; err != nil {
			t.Fatal(err)
		}

		entries := captureUnavailableBodyLogs(t)
		logs := readMCPAuditLogs(t, c)
		for _, log := range logs {
			if !slices.Equal(log.UnavailablePayloads, []string{"response.body"}) {
				t.Fatalf("row %d unavailable payloads = %v", log.ID, log.UnavailablePayloads)
			}
		}

		got := entries()
		if len(got) != 1 {
			t.Fatalf("logged %d entries for one read, want 1", len(got))
		}
		if got[0].Count != 3 || len(got[0].Causes) != 1 {
			t.Fatalf("logged count %d with causes %v, want 3 bodies under one cause", got[0].Count, got[0].Causes)
		}
		want := fmt.Sprintf("3: shared audit log body is missing (first: response.body of audit log %d)", logs[0].ID)
		if got[0].Causes[0] != want {
			t.Fatalf("logged cause %q, want %q", got[0].Causes[0], want)
		}
	})

	// Bodies that fail while their own field is decoded are tallied with those that fail when
	// the shared body is loaded.
	t.Run("the key cannot decrypt anything", func(t *testing.T) {
		c := newTestClient(t)
		useRotatingKeys(c, "written")
		for id := 1; id <= 2; id++ {
			storeMCPAuditEntries(t, c, listCallEntries(id, "tools/list", testListResponse(id, result))...)
		}

		useRotatingKeys(c, "different")
		entries := captureUnavailableBodyLogs(t)
		logs := readMCPAuditLogs(t, c)
		for _, log := range logs {
			if !slices.Equal(log.UnavailablePayloads, []string{"request.body", "response.body"}) {
				t.Fatalf("row %d unavailable payloads = %v", log.ID, log.UnavailablePayloads)
			}
		}

		got := entries()
		if len(got) != 1 {
			t.Fatalf("logged %d entries for one read, want 1", len(got))
		}
		if got[0].Count != 4 || len(got[0].Causes) != 1 || !strings.Contains(got[0].Causes[0], "failed to decrypt audit body") {
			t.Fatalf("logged count %d with causes %v, want 4 bodies that failed to decrypt", got[0].Count, got[0].Causes)
		}
	})

	t.Run("nothing is unreadable", func(t *testing.T) {
		c := newTestClient(t)
		storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)

		entries := captureUnavailableBodyLogs(t)
		if logs := readMCPAuditLogs(t, c); len(logs[0].UnavailablePayloads) != 0 {
			t.Fatalf("unexpected unavailable payloads %v", logs[0].UnavailablePayloads)
		}
		if got := entries(); len(got) != 0 {
			t.Fatalf("a read with nothing unreadable logged %v", got)
		}
	})

	// One read can fail in both phases: a row's own field, and a shared body another row needs.
	t.Run("both a field and a shared body are unreadable", func(t *testing.T) {
		c := newTestClient(t)
		storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
		storeMCPAuditEntries(t, c, listCallEntries(2, "tools/call", testListResponse(2, result))...)
		if err := c.db.WithContext(t.Context()).Where("1 = 1").Delete(&types.MCPAuditLogBody{}).Error; err != nil {
			t.Fatal(err)
		}
		corrupt := []byte{auditBodyMarker, auditBodyVersion, auditBodyFlagCompressed, 'b', 'a', 'd'}
		if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLog{}).Where("call_type = ?", "tools/call").Update("response_body", corrupt).Error; err != nil {
			t.Fatal(err)
		}

		entries := captureUnavailableBodyLogs(t)
		readMCPAuditLogs(t, c)
		got := entries()
		if len(got) != 1 || got[0].Count != 2 || len(got[0].Causes) != 2 {
			t.Fatalf("logged %v, want one entry counting 2 bodies under 2 causes", got)
		}
	})
}

// The log entry stays bounded however many different things go wrong, and still accounts for
// every body.
func TestUnavailableBodyCausesAreCapped(t *testing.T) {
	var unavailable unavailableAuditBodies
	log := types.MCPAuditLog{ID: 7}
	for i := range unavailableAuditBodyCauseLimit + 3 {
		for range 2 {
			field := json.RawMessage("x")
			unavailable.mark(&log, "response.body", &field, fmt.Errorf("cause %d", i))
		}
	}

	entries := captureUnavailableBodyLogs(t)
	unavailable.report()
	got := entries()
	if len(got) != 1 {
		t.Fatalf("logged %d entries, want 1", len(got))
	}
	total := 2 * (unavailableAuditBodyCauseLimit + 3)
	if got[0].Count != total {
		t.Fatalf("logged count %d, want %d", got[0].Count, total)
	}
	if len(got[0].Causes) != unavailableAuditBodyCauseLimit+1 {
		t.Fatalf("logged %d causes, want %d and one line for the rest: %v", len(got[0].Causes), unavailableAuditBodyCauseLimit, got[0].Causes)
	}
	if last := got[0].Causes[len(got[0].Causes)-1]; last != "6: other causes" {
		t.Fatalf("last cause is %q, want the 6 bodies beyond the limit", last)
	}
}

// A row written now with an old created_at, as a backfill or import would, refreshes its body at
// write time like any other write. Retention removes the row first and the body only later, so
// nothing dangles.
func TestBackdatedRowDoesNotOutliveItsBody(t *testing.T) {
	c := newTestClient(t)
	const retentionDays = 30
	now := time.Now().UTC()
	result := testListResult("backdated", 4096)

	// The body already exists and was last referenced long enough ago to be due for deletion.
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	if err := c.db.WithContext(t.Context()).Where("1 = 1").Delete(&types.MCPAuditLog{}).Error; err != nil {
		t.Fatal(err)
	}
	setAuditBodyLastReferenced(t, c, now.AddDate(0, 0, -retentionDays-10))

	backdated := listCallEntries(2, "tools/list", testListResponse(2, result))
	for i := range backdated {
		backdated[i].CreatedAt = now.AddDate(0, 0, -retentionDays+1).Add(time.Duration(i) * time.Millisecond)
	}
	storeMCPAuditEntries(t, c, backdated...)
	if got := loadAuditBody(t, c).LastReferencedAt; now.Sub(got) > time.Minute {
		t.Fatalf("body of a backdated row was last referenced at %v, want the time of the write", got)
	}

	// While the row is within retention it keeps its body.
	if err := c.cleanupRetainedData(t.Context(), now, retentionDays, 0); err != nil {
		t.Fatal(err)
	}
	if logs := readMCPAuditLogs(t, c); len(logs) != 1 || len(logs[0].UnavailablePayloads) != 0 {
		t.Fatalf("backdated row lost its body while within retention: %d rows", len(logs))
	}

	// Two days later the row has aged out, and the body it referenced is still there.
	if err := c.cleanupRetainedData(t.Context(), now.AddDate(0, 0, 2), retentionDays, 0); err != nil {
		t.Fatal(err)
	}
	if got := countAuditLogs(t, c); got != 0 {
		t.Fatalf("%d backdated rows survived retention", got)
	}
	if got := countAuditBodies(t, c); got != 1 {
		t.Fatalf("%d shared bodies remain, want the body to outlive the row", got)
	}
}

// When a body's refresh is due and it is under a key that has been rotated out of first place,
// the writer stores it again under the current key. Without that, a body in constant use would
// stay under the old key. Once that key was retired, the rows referencing the body would be
// unreadable and new rows could no longer share it.
//
// A body is only checked when its refresh is due, so rows written in the first day after a
// rotation can still reference the copy under the old key. The old key has to stay readable until
// retention has removed those rows, a day later than for rows written before the rotation.
func TestSharedBodyMovesToRotatedKeyOnRefresh(t *testing.T) {
	c := newTestClient(t)
	useRotatingKeys(c, "old")
	result := testListResult("rotate", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)

	storedUnder := func(key string) bool {
		return bytes.HasPrefix(loadAuditBody(t, c).Body[auditBodyHeaderLen:], rotatingKeyPrefix(key))
	}
	if !storedUnder("old") {
		t.Fatal("shared body was not written under the only key")
	}

	// A refresh under the key the body is already stored with leaves the stored copy as it is.
	untouched := loadAuditBody(t, c).Body
	setAuditBodyLastReferenced(t, c, time.Now().UTC().Add(-2*auditBodyReferenceRefresh))
	storeMCPAuditEntries(t, c, listCallEntries(2, "tools/list", testListResponse(2, result))...)
	if refreshed := loadAuditBody(t, c); !bytes.Equal(refreshed.Body, untouched) || time.Since(refreshed.LastReferencedAt) > time.Minute {
		t.Fatal("a refresh under the same key should only update when the body was last referenced")
	}

	// A restarted replica has the new key first and no memory of which bodies are stored.
	rotated := &Client{db: c.db}
	useRotatingKeys(rotated, "new", "old")
	third := testListResponse(3, result)
	storeMCPAuditEntries(t, rotated, listCallEntries(3, "tools/list", third)...)
	if !storedUnder("old") {
		t.Fatal("a recently referenced body was rewritten before its refresh was due")
	}

	setAuditBodyLastReferenced(t, c, time.Now().UTC().Add(-2*auditBodyReferenceRefresh))
	fourth := testListResponse(4, result)
	storeMCPAuditEntries(t, rotated, listCallEntries(4, "tools/list", fourth)...)
	if !storedUnder("new") {
		t.Fatal("shared body was not rewritten under the current key when its refresh was due")
	}

	// Retire the old key. Rows written since the rotation still read their shared body.
	useRotatingKeys(rotated, "new")
	logs := readMCPAuditLogs(t, rotated)
	if len(logs) != 4 {
		t.Fatalf("read %d rows, want 4", len(logs))
	}
	for i, want := range map[int]json.RawMessage{2: third, 3: fourth} {
		if len(logs[i].UnavailablePayloads) != 0 || !bytes.Equal(logs[i].MCP().ResponseBody, want) {
			t.Fatalf("row %d written after the rotation cannot be read once the old key is retired: unavailable=%v", i, logs[i].UnavailablePayloads)
		}
	}
}

// A replica whose key the others cannot read must not replace a shared body they depend on, and
// must not reference it either: it could not read its own row back. It stores its copy of the
// response inline instead.
func TestReplicaThatCannotReadASharedBodyLeavesItAlone(t *testing.T) {
	c := newTestClient(t)
	useRotatingKeys(c, "shared")
	result := testListResult("foreign", 4096)
	first := testListResponse(1, result)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", first)...)

	due := time.Now().UTC().Add(-2 * auditBodyReferenceRefresh).Truncate(time.Second)
	setAuditBodyLastReferenced(t, c, due)
	before := loadAuditBody(t, c)

	misconfigured := &Client{db: c.db}
	useRotatingKeys(misconfigured, "foreign")
	second := testListResponse(2, result)
	storeMCPAuditEntries(t, misconfigured, listCallEntries(2, "tools/list", second)...)

	after := loadAuditBody(t, c)
	if !bytes.Equal(after.Body, before.Body) || !after.LastReferencedAt.Equal(before.LastReferencedAt) {
		t.Fatal("a replica with a key the others cannot read changed the shared body")
	}
	if got := countAuditBodies(t, c); got != 1 {
		t.Fatalf("stored %d shared bodies, want only the original", got)
	}
	row, err := c.GetMCPAuditLog(t.Context(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, first) {
		t.Fatalf("row written before the other replica's write can no longer be read: unavailable=%v", row.UnavailablePayloads)
	}
	own, err := misconfigured.GetMCPAuditLog(t.Context(), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(own.UnavailablePayloads) != 0 || !bytes.Equal(own.MCP().ResponseBody, second) {
		t.Fatalf("the other replica cannot read its own row: unavailable=%v", own.UnavailablePayloads)
	}
}

// A stored copy nobody with the current keys can read, such as one left by a replica whose key
// was lost, must not be referenced by new rows. If it were, every later row with that result
// would be unreadable for as long as the result stayed in use, because each reference would
// keep retention from removing the copy. Left alone it ages out, and the body is stored afresh.
func TestUnreadableSharedBodyIsNotReferencedAndAgesOut(t *testing.T) {
	const retentionDays = 30
	c := newTestClient(t)
	useRotatingKeys(c, "lost")
	result := testListResult("orphan", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	if err := c.db.WithContext(t.Context()).Where("1 = 1").Delete(&types.MCPAuditLog{}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	lastReferenced := now.AddDate(0, 0, -retentionDays).Truncate(time.Second)
	setAuditBodyLastReferenced(t, c, lastReferenced)

	// Correctly configured replicas only have the current key.
	for id := 2; id <= 4; id++ {
		writer := &Client{db: c.db}
		useRotatingKeys(writer, "current")
		response := testListResponse(id, result)
		storeMCPAuditEntries(t, writer, listCallEntries(id, "tools/list", response)...)

		row, err := writer.GetMCPAuditLog(t.Context(), uint(id), true)
		if err != nil {
			t.Fatal(err)
		}
		if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, response) {
			t.Fatalf("row %d references a shared body its writer cannot read: unavailable=%v", id, row.UnavailablePayloads)
		}
	}
	if got := loadAuditBody(t, c).LastReferencedAt; !got.Equal(lastReferenced) {
		t.Fatalf("the unreadable body was kept alive: last referenced %v, want %v", got, lastReferenced)
	}

	// Retention removes the copy once it is old enough, and the next write shares again.
	writer := &Client{db: c.db, auditLogDeleteBatchSize: 10}
	useRotatingKeys(writer, "current")
	if err := writer.cleanupRetainedData(t.Context(), now.AddDate(0, 0, 4), retentionDays, 0); err != nil {
		t.Fatal(err)
	}
	if got := countAuditBodies(t, c); got != 0 {
		t.Fatalf("%d shared bodies remain, want the unreadable copy removed by retention", got)
	}
	fifth := testListResponse(5, result)
	storeMCPAuditEntries(t, writer, listCallEntries(5, "tools/list", fifth)...)
	if got := countAuditBodies(t, c); got != 1 {
		t.Fatalf("stored %d shared bodies after the unreadable copy aged out, want 1", got)
	}
	logs := readMCPAuditLogs(t, writer)
	if last := logs[len(logs)-1]; len(last.UnavailablePayloads) != 0 || !bytes.Equal(last.MCP().ResponseBody, fifth) {
		t.Fatalf("row written after the copy aged out did not read: unavailable=%v", last.UnavailablePayloads)
	}
}

// A writer that encrypts does not reference a stored copy that is not encrypted, however it got
// there: a reader would refuse it, so the row would be unreadable.
func TestUnencryptedCopyUnderAnEncryptedHashIsNotReferenced(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = contextBoundEncryptionConfig()
	result := testListResult("planted", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	planted := append([]byte{auditBodyMarker, auditBodyVersion, 0}, []byte(`{"tools":[]}`)...)
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", planted).Error; err != nil {
		t.Fatal(err)
	}

	writer := &Client{db: c.db, encryptionConfig: contextBoundEncryptionConfig()}
	second := testListResponse(2, result)
	storeMCPAuditEntries(t, writer, listCallEntries(2, "tools/list", second)...)

	row, err := writer.GetMCPAuditLog(t.Context(), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, second) {
		t.Fatalf("row references the planted copy: unavailable=%v body=%.40s", row.UnavailablePayloads, row.MCP().ResponseBody)
	}
	if !bytes.Equal(loadAuditBody(t, c).Body, planted) {
		t.Fatal("the planted copy was expected to be left for retention to remove")
	}
}

// A field's header is part of what its ciphertext is bound to, and an encrypted reference only
// accepts an encrypted body. With a provider that authenticates its context and accepts nothing
// unencrypted, as the one used here does, a stored field that is relabelled, or a shared body
// swapped for an unencrypted one, does not read back as valid. This is not protection against
// someone who can write to the database in general: a field relabelled as not encrypted is not
// decrypted at all, and the shipped configurations also accept unencrypted data, for reading rows
// from before encryption was enabled.
func TestBodiesAlteredInTheDatabaseAreNotReadAsValid(t *testing.T) {
	tests := []struct {
		name  string
		alter func(t *testing.T, c *Client)
	}{
		{
			name: "reference relabelled as an inline body",
			alter: func(t *testing.T, c *Client) {
				t.Helper()
				field := storedResponseBody(t, c)
				field[2] &^= auditBodyFlagReference
				if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLog{}).Where("1 = 1").Update("response_body", field).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "shared body replaced with an unencrypted one",
			alter: func(t *testing.T, c *Client) {
				t.Helper()
				forged := append([]byte{auditBodyMarker, auditBodyVersion, 0}, []byte(`{"tools":[]}`)...)
				if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", forged).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "shared body relabelled as uncompressed",
			alter: func(t *testing.T, c *Client) {
				t.Helper()
				body := loadAuditBody(t, c).Body
				body[2] &^= auditBodyFlagCompressed
				if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", body).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			c.encryptionConfig = contextBoundEncryptionConfig()
			storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, testListResult("altered", 4096)))...)
			tt.alter(t, c)

			logs := readMCPAuditLogs(t, c)
			if len(logs) != 1 || !slices.Equal(logs[0].UnavailablePayloads, []string{"response.body"}) || len(logs[0].MCP().ResponseBody) != 0 {
				t.Fatalf("altered body was read back: unavailable=%v body=%.60s", logs[0].UnavailablePayloads, logs[0].MCP().ResponseBody)
			}
		})
	}
}

// A hash key created before encryption was enabled sits in the database unencrypted, where it
// would let a database reader confirm guessed bodies against the stored hashes. The first write
// with encryption replaces it with one stored encrypted.
func TestHashKeyStoredBeforeEncryptionIsReplaced(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("protect", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	before, err := c.GetProperty(t.Context(), auditBodyHashKeyProperty)
	if err != nil {
		t.Fatal(err)
	}
	if before.Encrypted {
		t.Fatal("hash key is stored encrypted without encryption configured")
	}

	encryption := &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			mcpAuditLogGroupResource: contextBoundTransformer{},
			propertyGroupResource:    contextBoundTransformer{},
		},
	}
	encrypting := &Client{db: c.db, encryptionConfig: encryption}
	second := testListResponse(2, result)
	storeMCPAuditEntries(t, encrypting, listCallEntries(2, "tools/list", second)...)

	var stored types.Property
	if err := c.db.WithContext(t.Context()).Where("key = ?", auditBodyHashKeyProperty).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Encrypted || stored.Value == before.Value {
		t.Fatal("hash key created before encryption was enabled is still stored as it was")
	}
	newKey, err := encrypting.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	oldKey, err := c.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(newKey, oldKey) {
		t.Fatal("the replacement hash key is the same as the unencrypted one")
	}

	// Another replica with encryption reads the replacement rather than making its own.
	other := &Client{db: c.db, encryptionConfig: encryption}
	otherKey, err := other.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(otherKey, newKey) {
		t.Fatal("a second encrypting replica did not adopt the replaced hash key")
	}
	if row, err := encrypting.GetMCPAuditLog(t.Context(), 2, true); err != nil || !bytes.Equal(row.MCP().ResponseBody, second) {
		t.Fatalf("row written under the replaced key did not read: %v", err)
	}

	// A replica without the encryption configuration cannot read the key any more. It stores
	// list responses inline rather than sharing them.
	bodies := countAuditBodies(t, c)
	plain := &Client{db: c.db}
	third := testListResponse(3, result)
	storeMCPAuditEntries(t, plain, listCallEntries(3, "tools/list", third)...)
	if got := countAuditBodies(t, c); got != bodies {
		t.Fatalf("a replica that cannot read the hash key stored a shared body: %d, want %d", got, bodies)
	}
	if row, err := plain.GetMCPAuditLog(t.Context(), 3, true); err != nil || !bytes.Equal(row.MCP().ResponseBody, third) {
		t.Fatalf("inline row from the replica without the key did not read: %v", err)
	}
}

// storedHashKeyUnder reports whether the stored hash key property is encrypted under key.
func storedHashKeyUnder(t *testing.T, c *Client, key string) bool {
	t.Helper()
	var stored types.Property
	if err := c.db.WithContext(t.Context()).Where("key = ?", auditBodyHashKeyProperty).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored.Value)
	return err == nil && stored.Encrypted && bytes.HasPrefix(ciphertext, rotatingKeyPrefix(key))
}

// The hash key is stored encrypted like any other property, so it has to follow an encryption key
// rotation. Otherwise retiring the old encryption key would leave no replica able to read it, and
// list responses would quietly stop being shared.
func TestHashKeyFollowsEncryptionKeyRotation(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("keyrotation", 4096)

	before := rotatingKeysClient(c, "old")
	storeMCPAuditEntries(t, before, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	original, err := before.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !storedHashKeyUnder(t, c, "old") {
		t.Fatal("hash key was not stored under the only encryption key")
	}

	// A replica restarted with the new key first stores the same hash key again under it.
	rotated := rotatingKeysClient(c, "new", "old")
	storeMCPAuditEntries(t, rotated, listCallEntries(2, "tools/list", testListResponse(2, result))...)
	if !storedHashKeyUnder(t, c, "new") {
		t.Fatal("hash key was not stored again under the current encryption key")
	}
	if got := countAuditBodies(t, c); got != 1 {
		t.Fatalf("stored %d shared bodies, want the rotation to keep the hash key and so the one body", got)
	}

	// With the old encryption key retired, the hash key still loads and is still the same.
	retired := rotatingKeysClient(c, "new")
	key, err := retired.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatalf("hash key cannot be loaded once the old encryption key is retired: %v", err)
	}
	if !bytes.Equal(key, original) {
		t.Fatal("hash key changed across the rotation")
	}
}

func (f failingOnceTransformer) TransformFromStorage(ctx context.Context, data []byte, dataCtx value.Context) ([]byte, bool, error) {
	if *f.failures > 0 {
		*f.failures--
		return nil, false, errors.New("encryption provider is unavailable")
	}
	return f.rotatingTransformer.TransformFromStorage(ctx, data, dataCtx)
}

// A hash key that cannot be decrypted is left as it is. The failure may be passing, or the
// replica may hold the wrong encryption key, and in either case other replicas are using the
// stored key. The replica stores list responses inline until it can read it.
func TestHashKeyThatCannotBeDecryptedIsNotReplaced(t *testing.T) {
	result := testListResult("keptkey", 4096)

	t.Run("replica with another encryption key", func(t *testing.T) {
		c := newTestClient(t)
		storeMCPAuditEntries(t, rotatingKeysClient(c, "shared"), listCallEntries(1, "tools/list", testListResponse(1, result))...)

		other := rotatingKeysClient(c, "other")
		second := testListResponse(2, result)
		storeMCPAuditEntries(t, other, listCallEntries(2, "tools/list", second)...)
		if !storedHashKeyUnder(t, c, "shared") {
			t.Fatal("a replica that could not decrypt the hash key replaced it")
		}
		if got := countAuditBodies(t, c); got != 1 {
			t.Fatalf("stored %d shared bodies, want the other replica to store inline", got)
		}
		row, err := other.GetMCPAuditLog(t.Context(), 2, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, second) {
			t.Fatalf("the other replica cannot read its own row: unavailable=%v", row.UnavailablePayloads)
		}
	})

	t.Run("passing failure of the encryption provider", func(t *testing.T) {
		c := newTestClient(t)
		original := rotatingKeysClient(c, "shared")
		storeMCPAuditEntries(t, original, listCallEntries(1, "tools/list", testListResponse(1, result))...)
		want, err := original.auditBodyHashKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		failures := 1
		restarted := rotatingKeysClient(c, "shared")
		restarted.encryptionConfig.Transformers[propertyGroupResource] = failingOnceTransformer{
			keys:     []string{"shared"},
			failures: &failures,
		}
		if _, err := restarted.auditBodyHashKey(t.Context()); err == nil {
			t.Fatal("expected the first load to fail")
		}
		if !storedHashKeyUnder(t, c, "shared") {
			t.Fatal("a passing decryption failure replaced the hash key")
		}

		// Once the provider is back and the replica tries again, it gets the same key.
		restarted.auditBodies.hashKeyRetryAt = time.Time{}
		got, err := restarted.auditBodyHashKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatal("hash key changed after a passing decryption failure")
		}
	})
}

// A replica that cannot load the hash key says so once, then stays quiet until it tries again,
// rather than logging for every list response it stores inline.
func TestMissingHashKeyIsLoggedOncePerAttempt(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("quiet", 4096)
	storeMCPAuditEntries(t, rotatingKeysClient(c, "only"), listCallEntries(1, "tools/list", testListResponse(1, result))...)

	// This replica has no encryption configured, so it cannot read the encrypted hash key.
	plain := &Client{db: c.db}
	logs := captureLogs(t)
	for id := 2; id <= 4; id++ {
		response := testListResponse(id, result)
		storeMCPAuditEntries(t, plain, listCallEntries(id, "tools/list", response)...)
		row, err := plain.GetMCPAuditLog(t.Context(), uint(id), true)
		if err != nil || !bytes.Equal(row.MCP().ResponseBody, response) {
			t.Fatalf("row %d stored inline did not read: %v", id, err)
		}
	}
	if got := logs("MCP list responses will be stored inline until the audit body hash key can be loaded"); len(got) != 1 {
		t.Fatalf("logged the missing hash key %d times for three list responses, want once", len(got))
	}
	if storedHashKeyUnder(t, c, "only") == false {
		t.Fatal("a replica without encryption replaced the encrypted hash key")
	}
}

// A writer decides to replace a stored body before its transaction starts. If another replica
// has replaced the body in the meantime, the stored copy is no longer the one the writer judged,
// and it must be left alone.
func TestSharedBodyIsReplacedOnlyIfUnchangedSinceInspected(t *testing.T) {
	c := newTestClient(t)
	hash := bytes.Repeat([]byte{9}, auditBodyHashLen)
	inspected := []byte("copy the writer inspected")
	now := time.Now().UTC()
	if err := c.db.WithContext(t.Context()).Create(&types.MCPAuditLogBody{Hash: hash, Body: inspected, CreatedAt: now, LastReferencedAt: now.AddDate(0, 0, -3)}).Error; err != nil {
		t.Fatal(err)
	}
	write := auditBodyWrite{
		body:            types.MCPAuditLogBody{Hash: hash, Body: []byte("writer's replacement"), CreatedAt: now, LastReferencedAt: now},
		replaceIfStored: inspected,
	}

	// Another replica got there first.
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", []byte("another replica's copy")).Error; err != nil {
		t.Fatal(err)
	}
	if err := storeAuditBodies(c.db.WithContext(t.Context()), []auditBodyWrite{write}); err != nil {
		t.Fatal(err)
	}
	stored := loadAuditBody(t, c)
	if string(stored.Body) != "another replica's copy" {
		t.Fatalf("stored body is %q, want the other replica's copy left in place", stored.Body)
	}
	if now.Sub(stored.LastReferencedAt) > time.Minute {
		t.Fatal("the reference was not recorded")
	}

	// With the stored copy as inspected, the replacement goes in.
	write.replaceIfStored = []byte("another replica's copy")
	if err := storeAuditBodies(c.db.WithContext(t.Context()), []auditBodyWrite{write}); err != nil {
		t.Fatal(err)
	}
	if got := loadAuditBody(t, c).Body; string(got) != "writer's replacement" {
		t.Fatalf("stored body is %q, want the writer's replacement", got)
	}
}

// A replica trusts a stored copy only after reading it itself. Writing a row that references a
// body proves nothing about the copy that is stored: another writer may have stored it between
// this replica capturing the response and writing the row.
func TestCopyStoredBetweenCaptureAndWriteIsReadBeforeItIsTrusted(t *testing.T) {
	c := newTestClient(t)
	useRotatingKeys(c, "mine")
	result := testListResult("between", 4096)

	// Captured while nothing is stored, so the entry carries the body to insert.
	entries := listCallEntries(1, "tools/list", testListResponse(1, result))
	for i := range entries {
		entries[i].NormalizeMCPFields()
		if err := c.encryptMCPAuditLog(t.Context(), &entries[i]); err != nil {
			t.Fatal(err)
		}
	}
	pending := entries[1].PendingBodies
	if len(pending) != 1 || pending[0].Encoded == nil {
		t.Fatalf("expected the captured entry to carry the body to store, got %d pending bodies", len(pending))
	}

	// Before the row is written, a replica with another key stores its own copy.
	other := rotatingTransformer{keys: []string{"theirs"}}
	foreign, err := encodeAuditBody(t.Context(), other, auditBodyDataCtx(pending[0].Hash), []byte(result), 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := c.db.WithContext(t.Context()).Create(&types.MCPAuditLogBody{Hash: pending[0].Hash, Body: foreign, CreatedAt: now, LastReferencedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.insertMCPAuditLogs(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loadAuditBody(t, c).Body, foreign) {
		t.Fatal("the copy stored by the other replica was overwritten")
	}

	// The row just written references a copy this replica never read. That window is accepted.
	// What must not happen is the replica going on to trust the copy: the next capture reads it,
	// finds it unusable, and stores the response inline.
	second := testListResponse(2, result)
	storeMCPAuditEntries(t, c, listCallEntries(2, "tools/list", second)...)
	row, err := c.GetMCPAuditLog(t.Context(), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, second) {
		t.Fatalf("the next row references a copy its writer never read and cannot use: unavailable=%v", row.UnavailablePayloads)
	}
}

// A stored copy is read again once what the replica remembers about it has expired, however
// often the result is written in between. A copy that has gone bad since is then no longer
// referenced.
func TestStoredCopyIsReadAgainAfterItWasLastChecked(t *testing.T) {
	c := newTestClient(t)
	useRotatingKeys(c, "key")
	result := testListResult("recheck", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	storeMCPAuditEntries(t, c, listCallEntries(2, "tools/list", testListResponse(2, result))...)

	hash := [auditBodyHashLen]byte(loadAuditBody(t, c).Hash)
	checked, ok := c.auditBodies.known[hash]
	if !ok || checked.unusable {
		t.Fatal("expected the second capture to have read the stored copy and found it usable")
	}

	// Writing more rows does not count as checking the copy again.
	storeMCPAuditEntries(t, c, listCallEntries(3, "tools/list", testListResponse(3, result))...)
	if got := c.auditBodies.known[hash]; !got.at.Equal(checked.at) {
		t.Fatal("writing a row extended how long the stored copy is trusted without reading it")
	}

	// The copy goes bad.
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", []byte{auditBodyMarker, auditBodyVersion, auditBodyFlagEncrypted, 'b', 'a', 'd'}).Error; err != nil {
		t.Fatal(err)
	}
	// Once that time is up the replica reads it again, and stores the response inline.
	c.auditBodies.known[hash] = auditBodyKnown{at: time.Now().Add(-auditBodyKnownFreshFor - time.Minute)}
	fourth := testListResponse(4, result)
	storeMCPAuditEntries(t, c, listCallEntries(4, "tools/list", fourth)...)
	row, err := c.GetMCPAuditLog(t.Context(), 4, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, fourth) {
		t.Fatalf("row written after the re-check references the bad copy: unavailable=%v", row.UnavailablePayloads)
	}
	if got := c.auditBodies.known[hash]; !got.unusable {
		t.Fatal("the bad copy was not remembered as unusable")
	}

	// The copy becomes good again. The replica goes by what it remembers for a short while,
	// without reading the copy, and then looks again: an unusable verdict is kept only briefly,
	// since its cause may pass.
	good, err := encodeAuditBody(t.Context(), c.mcpAuditLogTransformer(), auditBodyDataCtx(hash[:]), []byte(result), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", good).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if got := c.auditBodyStoredState(t.Context(), c.mcpAuditLogTransformer(), hash[:], now); got != auditBodyStoredUnusable {
		t.Fatalf("state while the unusable verdict is remembered = %d, want unusable without reading the copy", got)
	}
	if got := c.auditBodyStoredState(t.Context(), c.mcpAuditLogTransformer(), hash[:], now.Add(auditBodyUnusableFor+time.Minute)); got != auditBodyStoredUsable {
		t.Fatalf("state once the unusable verdict has expired = %d, want usable", got)
	}
	storeMCPAuditEntries(t, c, listCallEntries(5, "tools/list", testListResponse(5, result))...)
	if stored := storedResponseBodyOf(t, c, 5); stored[2]&auditBodyFlagReference == 0 {
		t.Fatal("the replica did not go back to sharing once the stored copy was usable again")
	}
}

func (c cancellingTransformer) TransformFromStorage(ctx context.Context, _ []byte, _ value.Context) ([]byte, bool, error) {
	c.cancel()
	return nil, false, ctx.Err()
}

// A check that was cut short says nothing about the stored copy, so the replica does not remember
// it as a verdict and looks again next time.
func TestCheckCutShortIsNotRememberedAsAVerdict(t *testing.T) {
	c := newTestClient(t)
	useRotatingKeys(c, "key")
	result := testListResult("cutshort", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	hash := loadAuditBody(t, c).Hash

	ctx, cancel := context.WithCancel(t.Context())
	interrupted := cancellingTransformer{keys: []string{"key"}, cancel: cancel}
	if got := c.auditBodyStoredState(ctx, interrupted, hash, time.Now()); got != auditBodyStoredUnusable {
		t.Fatalf("state of a check that was cut short = %d, want unusable for this entry", got)
	}
	if _, remembered := c.auditBodies.known[[auditBodyHashLen]byte(hash)]; remembered {
		t.Fatal("a check that was cut short was remembered as a verdict on the stored copy")
	}
	if got := c.auditBodyStoredState(t.Context(), c.mcpAuditLogTransformer(), hash, time.Now()); got != auditBodyStoredUsable {
		t.Fatalf("state on the next check = %d, want usable", got)
	}
}

// When a write finds that a copy due for refresh cannot be decoded, the rows being written
// already reference it, but the replica remembers the copy as unusable so that later rows do
// not. A decode cut short by its context is not taken as a verdict.
func TestUndecodableCopyFoundAtRefreshIsRememberedAsUnusable(t *testing.T) {
	c := newTestClient(t)
	useRotatingKeys(c, "key")
	result := testListResult("dueandbad", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	storeMCPAuditEntries(t, c, listCallEntries(2, "tools/list", testListResponse(2, result))...)
	hash := [auditBodyHashLen]byte(loadAuditBody(t, c).Hash)
	if known, ok := c.auditBodies.known[hash]; !ok || known.unusable {
		t.Fatal("expected the stored copy to be remembered as usable")
	}

	makeDue := func() {
		setAuditBodyLastReferenced(t, c, time.Now().UTC().Add(-2*auditBodyReferenceRefresh))
	}

	// A decode cut short by its context changes nothing that is remembered.
	makeDue()
	ctx, cancel := context.WithCancel(t.Context())
	interrupted := cancellingTransformer{keys: []string{"key"}, cancel: cancel}
	outdated, err := c.outdatedAuditBodies(ctx, interrupted, [][]byte{hash[:]})
	if err != nil || len(outdated) != 0 || ctx.Err() == nil {
		t.Fatalf("expected the decode to be cut short and nothing to be replaced: outdated=%d err=%v", len(outdated), err)
	}
	if c.auditBodies.known[hash].unusable {
		t.Fatal("a decode cut short by its context was remembered as an unusable copy")
	}

	// The copy goes bad, and its refresh is due when the next row is written.
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", []byte{auditBodyMarker, auditBodyVersion, auditBodyFlagEncrypted, 'b', 'a', 'd'}).Error; err != nil {
		t.Fatal(err)
	}
	makeDue()
	storeMCPAuditEntries(t, c, listCallEntries(3, "tools/list", testListResponse(3, result))...)
	if !c.auditBodies.known[hash].unusable {
		t.Fatal("a copy that could not be decoded at its refresh was not remembered as unusable")
	}

	// So the row after that is stored inline and can be read.
	fourth := testListResponse(4, result)
	storeMCPAuditEntries(t, c, listCallEntries(4, "tools/list", fourth)...)
	row, err := c.GetMCPAuditLog(t.Context(), 4, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, fourth) {
		t.Fatalf("row written after the failed refresh references the bad copy: unavailable=%v", row.UnavailablePayloads)
	}
}

// If the encryption key the hash key is stored under is truly lost, deleting the property is the
// way out: a replica makes a new hash key at its next attempt, without a restart, and shares
// list responses again.
func TestDeletingAnUnreadableHashKeyRestoresSharing(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("remedy", 4096)
	storeMCPAuditEntries(t, rotatingKeysClient(c, "lost"), listCallEntries(1, "tools/list", testListResponse(1, result))...)

	current := rotatingKeysClient(c, "current")
	storeMCPAuditEntries(t, current, listCallEntries(2, "tools/list", testListResponse(2, result))...)
	if stored := storedResponseBodyOf(t, c, 2); stored[2]&auditBodyFlagReference != 0 {
		t.Fatal("expected the list response to be stored inline while the hash key cannot be read")
	}

	if err := current.DeleteProperty(t.Context(), auditBodyHashKeyProperty); err != nil {
		t.Fatal(err)
	}
	// The replica tries again once its retry time has passed.
	current.auditBodies.hashKeyRetryAt = time.Time{}
	third := testListResponse(3, result)
	storeMCPAuditEntries(t, current, listCallEntries(3, "tools/list", third)...)
	if !storedHashKeyUnder(t, c, "current") {
		t.Fatal("no new hash key was stored after the unreadable one was deleted")
	}
	if stored := storedResponseBodyOf(t, c, 3); stored[2]&auditBodyFlagReference == 0 {
		t.Fatal("the replica did not go back to sharing after the unreadable hash key was deleted")
	}
	row, err := current.GetMCPAuditLog(t.Context(), 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, third) {
		t.Fatalf("row written under the new hash key did not read: unavailable=%v", row.UnavailablePayloads)
	}
}

// When the database fails while a stored copy is being checked, the response is stored inline
// and the replica leaves stored copies unread for a while, so that capturing list responses does
// not wait on a failing database once per response.
func TestDatabaseFailureWhileCheckingStoredCopyBacksOff(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("backoff", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	hash := loadAuditBody(t, c).Hash

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if got := c.auditBodyStoredState(cancelled, nil, hash, time.Now()); got != auditBodyStoredUnusable {
		t.Fatalf("state with a failing database = %d, want unusable", got)
	}
	if _, remembered := c.auditBodies.known[[auditBodyHashLen]byte(hash)]; remembered {
		t.Fatal("a failed check was remembered as a verdict on the stored copy")
	}
	if got := c.auditBodyStoredState(t.Context(), nil, hash, time.Now()); got != auditBodyStoredUnusable {
		t.Fatalf("state during the backoff = %d, want unusable without reading the database", got)
	}
	if got := c.auditBodyStoredState(t.Context(), nil, hash, time.Now().Add(auditBodyStoredCheckRetryAfter+time.Second)); got != auditBodyStoredUsable {
		t.Fatalf("state after the backoff = %d, want usable", got)
	}
}

// storedResponseBodyOf returns the response_body column of one audit row exactly as stored.
func storedResponseBodyOf(t *testing.T, c *Client, id uint) []byte {
	t.Helper()
	var row types.MCPAuditLog
	if err := c.db.WithContext(t.Context()).First(&row, id).Error; err != nil {
		t.Fatalf("load stored audit row %d: %v", id, err)
	}
	return row.MCP().ResponseBody
}

func (h hookedTransformer) TransformToStorage(ctx context.Context, data []byte, dataCtx value.Context) ([]byte, error) {
	if hook := *h.beforeFirstEncrypt; hook != nil {
		*h.beforeFirstEncrypt = nil
		hook()
	}
	return h.rotatingTransformer.TransformToStorage(ctx, data, dataCtx)
}

// Two replicas that both find the hash key stored unencrypted each generate a replacement. Only
// one may win, and the other must take the winner's key rather than keep its own, or the two
// would never share a stored body.
func TestReplicasReplacingTheHashKeyAtOnceAgreeOnOne(t *testing.T) {
	c := newTestClient(t)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, testListResult("race", 4096)))...)

	first, second := rotatingKeysClient(c, "key"), rotatingKeysClient(c, "key")
	var secondKey []byte
	// The second replica replaces the key after the first has read it and before the first
	// writes its own replacement.
	hook := func() {
		var err error
		if secondKey, err = second.auditBodyHashKey(t.Context()); err != nil {
			t.Errorf("second replica: %v", err)
		}
	}
	first.encryptionConfig.Transformers[propertyGroupResource] = hookedTransformer{
		keys:               []string{"key"},
		beforeFirstEncrypt: &hook,
	}

	firstKey, err := first.auditBodyHashKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if secondKey == nil {
		t.Fatal("the second replica did not run between the first replica's read and write")
	}
	if !bytes.Equal(firstKey, secondKey) {
		t.Fatal("the replicas ended up with different hash keys")
	}
	stored, err := rotatingKeysClient(c, "key").auditBodyHashKey(t.Context())
	if err != nil || !bytes.Equal(stored, secondKey) {
		t.Fatalf("the stored hash key is not the one the replicas agreed on: %v", err)
	}
}

// The format version is part of what a field's ciphertext is bound to, alongside its flags.
func TestHeaderContextCarriesVersionAndFlags(t *testing.T) {
	base := value.DefaultContext("row-context")
	got := string(auditBodyHeaderCtx(base, auditBodyFlagEncrypted|auditBodyFlagReference).AuthenticatedData())
	want := fmt.Sprintf("row-context#%d.%d", auditBodyVersion, auditBodyFlagEncrypted|auditBodyFlagReference)
	if got != want {
		t.Fatalf("header context = %q, want %q", got, want)
	}
	if string(base.AuthenticatedData()) != "row-context" {
		t.Fatal("building the header context changed the row's context")
	}
}

// A stored copy that is itself a reference is not something a row can point at.
func TestStoredCopyThatIsAReferenceIsNotReferenced(t *testing.T) {
	c := newTestClient(t)
	result := testListResult("nested", 4096)
	storeMCPAuditEntries(t, c, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	hash := loadAuditBody(t, c).Hash
	nested, err := encodeAuditBody(t.Context(), nil, auditBodyDataCtx(hash), encodeAuditBodyReference(hash, 0, nil), auditBodyFlagReference)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLogBody{}).Where("1 = 1").Update("body", nested).Error; err != nil {
		t.Fatal(err)
	}

	writer := &Client{db: c.db}
	if got := writer.auditBodyStoredState(t.Context(), nil, hash, time.Now()); got != auditBodyStoredUnusable {
		t.Fatalf("state of a stored copy that is a reference = %d, want unusable", got)
	}
	second := testListResponse(2, result)
	storeMCPAuditEntries(t, writer, listCallEntries(2, "tools/list", second)...)
	row, err := writer.GetMCPAuditLog(t.Context(), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(row.UnavailablePayloads) != 0 || !bytes.Equal(row.MCP().ResponseBody, second) {
		t.Fatalf("row references a stored copy that is a reference: unavailable=%v", row.UnavailablePayloads)
	}
}

// What a replica remembers about stored copies stays bounded.
func TestRememberedStoredCopiesAreBounded(t *testing.T) {
	c := &Client{}
	now := time.Now()
	for i := range auditBodyKnownMax + 10 {
		hash := make([]byte, auditBodyHashLen)
		binary.BigEndian.PutUint64(hash, uint64(i))
		c.rememberAuditBody(hash, now, false)
	}
	if got := len(c.auditBodies.known); got > auditBodyKnownMax {
		t.Fatalf("remembers %d stored copies, want at most %d", got, auditBodyKnownMax)
	}
}
