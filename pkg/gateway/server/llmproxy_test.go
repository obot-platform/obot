package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/tidwall/gjson"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestNewRunTokenActivityCapturesUserAndAPIKey(t *testing.T) {
	requestUser := &user.DefaultInfo{
		UID: "7",
		Extra: map[string][]string{
			principal.APIKeyIDExtra:   {"42"},
			principal.APIKeyNameExtra: {"CLI token"},
		},
	}
	usage := types.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalSpend: 0.25}

	activity := newRunTokenActivity(requestUser, "gpt-5", usage)

	if activity.UserID != "7" || activity.Model != "gpt-5" || activity.Usage != usage {
		t.Fatalf("activity identity or usage = %#v", activity)
	}
	if activity.APIKeyID == nil || *activity.APIKeyID != 42 || activity.APIKeyName != "CLI token" {
		t.Fatalf("API key attribution = ID %v, name %q; want ID 42, name %q", activity.APIKeyID, activity.APIKeyName, "CLI token")
	}
}

func TestNewRunTokenActivityLeavesDirectUserUnattributed(t *testing.T) {
	activity := newRunTokenActivity(&user.DefaultInfo{UID: "7"}, "gpt-5", types.TokenUsage{})
	if activity.APIKeyID != nil || activity.APIKeyName != "" {
		t.Fatalf("direct user gained API key attribution: %#v", activity)
	}
}

func TestExtractModelFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "top-level model (OpenAI/Anthropic request)",
			body: `{"model":"gpt-4o","messages":[]}`,
			want: "gpt-4o",
		},
		{
			name: "nested under message",
			body: `{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`,
			want: "claude-sonnet-4-20250514",
		},
		{
			name: "nested under response",
			body: `{"type":"response.completed","response":{"model":"gpt-4o"}}`,
			want: "gpt-4o",
		},
		{
			name: "top-level takes precedence over nested",
			body: `{"model":"top-level","message":{"model":"nested"}}`,
			want: "top-level",
		},
		{
			name: "empty body",
			body: `{}`,
			want: "",
		},
		{
			name: "no model anywhere",
			body: `{"messages":[{"role":"user","content":"hello"}]}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModelFromBody([]byte(tt.body))
			if got != tt.want {
				t.Errorf("extractModelFromBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRewriteModelInBody(t *testing.T) {
	body := `{"model":"anthropic-model-provider/anthropic-claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}]}`

	rewritten, err := rewriteModelInBody([]byte(body), "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := extractModelFromBody(rewritten); got != "claude-sonnet-4-6" {
		t.Fatalf("model = %q, want claude-sonnet-4-6", got)
	}
}

// runModelListFilter builds a GET /v1/models response carrying body and runs it
// through filterModelListResponse exactly as the passthrough proxy would,
// returning the (possibly rewritten) response and its body.
func runModelListFilter(t *testing.T, statusCode int, allowAll bool, allowed map[string]bool, body string) (*http.Response, string) {
	t.Helper()

	resp := &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type":     []string{"application/json"},
			"Content-Encoding": []string{"gzip"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: &http.Request{URL: &url.URL{Path: "/v1/models"}},
	}

	if err := filterModelListResponse(resp, allowed, allowAll); err != nil {
		t.Fatalf("filterModelListResponse() error = %v", err)
	}

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, string(out)
}

func modelDataIDs(body string) []string {
	var ids []string
	for _, r := range gjson.Get(body, "data.#.id").Array() {
		ids = append(ids, r.String())
	}
	return ids
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertCursor checks a pagination cursor field: want == nil means the field must
// be absent (deleted, not present-but-null); otherwise it must equal *want.
func assertCursor(t *testing.T, body, key string, want *string) {
	t.Helper()
	got := gjson.Get(body, key)
	if want == nil {
		if got.Exists() {
			t.Errorf("%s = %s, want field absent", key, got.Raw)
		}
		return
	}
	if got.String() != *want {
		t.Errorf("%s = %q, want %q", key, got.String(), *want)
	}
}

// assertEntriesPreserved checks that every surviving entry is byte-identical to
// the one in the upstream body — the byte-perfect fidelity guarantee.
func assertEntriesPreserved(t *testing.T, in, out string, ids []string) {
	t.Helper()
	for _, id := range ids {
		q := fmt.Sprintf("data.#(id==%q)", id)
		if want, got := gjson.Get(in, q).Raw, gjson.Get(out, q).Raw; want != got {
			t.Errorf("entry %q not preserved byte-for-byte:\n upstream = %s\n filtered = %s", id, want, got)
		}
	}
}

func assertFilteredHeaders(t *testing.T, resp *http.Response, out string) {
	t.Helper()
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want removed", got)
	}
	if got, want := resp.Header.Get("Content-Length"), strconv.Itoa(len(out)); got != want {
		t.Errorf("Content-Length header = %q, want %q", got, want)
	}
	if resp.ContentLength != int64(len(out)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(out))
	}
}

func TestModifyResponse_FiltersAnthropicModelList(t *testing.T) {
	tests := []struct {
		name        string
		allowed     map[string]bool
		body        string
		wantIDs     []string
		wantFirstID *string
		wantLastID  *string
	}{
		{
			name:        "keeps allowed subset and repoints cursors to boundaries",
			allowed:     map[string]bool{"claude-opus-4": true, "claude-haiku-4": true},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"},{"type":"model","id":"claude-sonnet-4"},{"type":"model","id":"claude-haiku-4"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-haiku-4"}`,
			wantIDs:     []string{"claude-opus-4", "claude-haiku-4"},
			wantFirstID: new("claude-opus-4"),
			wantLastID:  new("claude-haiku-4"),
		},
		{
			name:        "single middle survivor becomes both cursors",
			allowed:     map[string]bool{"claude-sonnet-4": true},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"},{"type":"model","id":"claude-sonnet-4"},{"type":"model","id":"claude-haiku-4"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-haiku-4"}`,
			wantIDs:     []string{"claude-sonnet-4"},
			wantFirstID: new("claude-sonnet-4"),
			wantLastID:  new("claude-sonnet-4"),
		},
		{
			name:        "empty page on last page drops both cursors",
			allowed:     map[string]bool{},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-opus-4"}`,
			wantIDs:     nil,
			wantFirstID: nil,
			wantLastID:  nil,
		},
		{
			name:        "empty page with more pages retains upstream last_id",
			allowed:     map[string]bool{},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"}],"first_id":"claude-opus-4","has_more":true,"last_id":"page-cursor"}`,
			wantIDs:     nil,
			wantFirstID: nil,
			wantLastID:  new("page-cursor"),
		},
		{
			name:        "preserves entry bytes (key order and large integers)",
			allowed:     map[string]bool{"claude-sonnet-4": true},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"},{"zzz_meta":1,"id":"claude-sonnet-4","created":1234567890123456789,"display_name":"Sonnet"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-sonnet-4"}`,
			wantIDs:     []string{"claude-sonnet-4"},
			wantFirstID: new("claude-sonnet-4"),
			wantLastID:  new("claude-sonnet-4"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := runModelListFilter(t, http.StatusOK, false, tt.allowed, tt.body)

			if got := modelDataIDs(out); !sameStrings(got, tt.wantIDs) {
				t.Errorf("data ids = %v, want %v", got, tt.wantIDs)
			}
			assertCursor(t, out, "first_id", tt.wantFirstID)
			assertCursor(t, out, "last_id", tt.wantLastID)
			assertEntriesPreserved(t, tt.body, out, tt.wantIDs)
			assertFilteredHeaders(t, resp, out)
		})
	}
}

func TestModifyResponse_FiltersOpenAIModelList(t *testing.T) {
	tests := []struct {
		name    string
		allowed map[string]bool
		body    string
		wantIDs []string
	}{
		{
			name:    "keeps allowed subset and leaves the envelope alone",
			allowed: map[string]bool{"gpt-4o": true},
			body:    `{"object":"list","data":[{"id":"gpt-4o","object":"model","created":1686935002,"owned_by":"openai"},{"id":"gpt-3.5-turbo","object":"model","created":1677610602,"owned_by":"openai"}]}`,
			wantIDs: []string{"gpt-4o"},
		},
		{
			name:    "allowed nothing yields empty list",
			allowed: map[string]bool{},
			body:    `{"object":"list","data":[{"id":"gpt-4o","object":"model","created":1686935002}]}`,
			wantIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := runModelListFilter(t, http.StatusOK, false, tt.allowed, tt.body)

			if got := modelDataIDs(out); !sameStrings(got, tt.wantIDs) {
				t.Errorf("data ids = %v, want %v", got, tt.wantIDs)
			}
			if got := gjson.Get(out, "object").String(); got != "list" {
				t.Errorf("object = %q, want \"list\"", got)
			}
			// OpenAI lists carry no cursors; we must not invent them.
			assertCursor(t, out, "first_id", nil)
			assertCursor(t, out, "last_id", nil)
			assertEntriesPreserved(t, tt.body, out, tt.wantIDs)
			assertFilteredHeaders(t, resp, out)
		})
	}
}

func TestModifyResponse_ModelListPassthrough(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		allowAll   bool
		allowed    map[string]bool
		body       string
	}{
		{
			name:       "allow-all forwards the full list untouched",
			statusCode: http.StatusOK,
			allowAll:   true,
			body:       `{"data":[{"id":"a"},{"id":"b"}],"first_id":"a","has_more":false,"last_id":"b"}`,
		},
		{
			name:       "non-200 is forwarded untouched",
			statusCode: http.StatusInternalServerError,
			allowed:    map[string]bool{},
			body:       `{"type":"error","error":{"message":"boom"}}`,
		},
		{
			name:       "non-JSON body is forwarded untouched",
			statusCode: http.StatusOK,
			allowed:    map[string]bool{},
			body:       "not json at all",
		},
		{
			name:       "JSON without a data array is forwarded untouched",
			statusCode: http.StatusOK,
			allowed:    map[string]bool{},
			body:       `{"error":{"message":"nope"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := runModelListFilter(t, tt.statusCode, tt.allowAll, tt.allowed, tt.body)

			if out != tt.body {
				t.Errorf("body modified:\n got = %s\nwant = %s", out, tt.body)
			}
			// Passthrough must not strip transfer headers.
			if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
				t.Errorf("Content-Encoding = %q, want gzip (untouched)", got)
			}
		})
	}
}

func TestRewriteAnthropicListCursors(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantFirstID   *string
		wantLastID    *string
		wantUnchanged bool
	}{
		{
			name:        "non-empty repoints to boundary ids",
			body:        `{"data":[{"id":"a"},{"id":"b"},{"id":"c"}],"first_id":"x","has_more":false,"last_id":"y"}`,
			wantFirstID: new("a"),
			wantLastID:  new("c"),
		},
		{
			name:          "openai shape without cursors is untouched",
			body:          `{"object":"list","data":[{"id":"a"}]}`,
			wantFirstID:   nil,
			wantLastID:    nil,
			wantUnchanged: true,
		},
		{
			name:        "empty page on last page drops both cursors",
			body:        `{"data":[],"first_id":"x","has_more":false,"last_id":"y"}`,
			wantFirstID: nil,
			wantLastID:  nil,
		},
		{
			name:        "empty page with more pages retains last_id",
			body:        `{"data":[],"first_id":"x","has_more":true,"last_id":"y"}`,
			wantFirstID: nil,
			wantLastID:  new("y"),
		},
		{
			name:        "only first_id present is the only field set",
			body:        `{"data":[{"id":"a"}],"first_id":"x"}`,
			wantFirstID: new("a"),
			wantLastID:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := rewriteAnthropicListCursors([]byte(tt.body))
			if err != nil {
				t.Fatalf("rewriteAnthropicListCursors() error = %v", err)
			}
			if tt.wantUnchanged && string(out) != tt.body {
				t.Errorf("body changed:\n got = %s\nwant = %s", out, tt.body)
			}
			assertCursor(t, string(out), "first_id", tt.wantFirstID)
			assertCursor(t, string(out), "last_id", tt.wantLastID)
		})
	}
}

func TestCopyBody(t *testing.T) {
	t.Run("restores body and returns a copy safe to modify", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader("hello"))

		got, err := copyBody(&body)
		if err != nil {
			t.Fatalf("copyBody() error = %v", err)
		}
		if string(got) != "hello" {
			t.Fatalf("copyBody() = %q, want %q", got, "hello")
		}

		// Mutating the returned slice must not affect the restored body.
		got[0] = 'J'

		restored, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("read restored body: %v", err)
		}
		if string(restored) != "hello" {
			t.Errorf("restored body = %q, want %q (unaffected by caller mutation)", restored, "hello")
		}
	})

	t.Run("works on an http.Response body field", func(t *testing.T) {
		resp := &http.Response{Body: io.NopCloser(strings.NewReader("payload"))}

		got, err := copyBody(&resp.Body)
		if err != nil {
			t.Fatalf("copyBody() error = %v", err)
		}
		if string(got) != "payload" {
			t.Fatalf("copyBody() = %q, want %q", got, "payload")
		}

		restored, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read restored body: %v", err)
		}
		if string(restored) != "payload" {
			t.Errorf("restored body = %q, want %q", restored, "payload")
		}
	})
}
