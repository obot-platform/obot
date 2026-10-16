//go:build e2e

package llmgateway

import (
	"bufio"
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/tidwall/gjson"
)

// API formats of the gateway's inference endpoints.
const (
	apiChat      = "chat"
	apiResponses = "responses"
	apiMessages  = "messages"
)

var (
	allAPIs = []string{apiChat, apiResponses, apiMessages}
)

// llmResult is a gateway response with its output assembled from either a JSON body or an
// event stream.
type llmResult struct {
	Status int
	Header http.Header
	Body   string
	// Text is the assistant's text output.
	Text string
	// ToolArgs is the arguments of the tool calls the model made, concatenated.
	ToolArgs string
	// ErrorType and ErrorMessage describe an error response or stream error event.
	ErrorType    string
	ErrorMessage string
}

// nonce returns a unique marker to find a request in the mock's records.
func nonce(prefix string) string {
	return prefix + "-" + harness.RandomSuffix() + harness.RandomSuffix()
}

// llmRequest builds the request body for an API format with one user message.
func llmRequest(api, model, prompt string, stream bool, extra map[string]any) map[string]any {
	var body map[string]any
	switch api {
	case apiChat:
		body = map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": prompt}}}
		if stream {
			body["stream_options"] = map[string]any{"include_usage": true}
		}
	case apiResponses:
		body = map[string]any{"model": model, "input": []any{map[string]any{"role": "user", "content": prompt}}}
	case apiMessages:
		body = map[string]any{"model": model, "max_tokens": 256, "messages": []any{map[string]any{"role": "user", "content": prompt}}}
	}
	if stream {
		body["stream"] = true
	}
	maps.Copy(body, extra)
	return body
}

func apiPath(api string) string {
	switch api {
	case apiChat:
		return "/api/llm-proxy/v1/chat/completions"
	case apiResponses:
		return "/api/llm-proxy/v1/responses"
	default:
		return "/api/llm-proxy/v1/messages"
	}
}

// callLLM sends one user message in an API format and parses the result.
func callLLM(tb testing.TB, c *harness.Client, api, model, prompt string, stream bool, extra map[string]any, headers ...map[string]string) *llmResult {
	tb.Helper()
	h := map[string]string{}
	if api == apiMessages {
		h["anthropic-version"] = "2023-06-01"
	}
	for _, hh := range headers {
		maps.Copy(h, hh)
	}
	resp := c.Do(tb, http.MethodPost, apiPath(api), llmRequest(api, model, prompt, stream, extra), h)
	return parseLLM(api, resp)
}

func parseLLM(api string, resp *harness.Response) *llmResult {
	r := &llmResult{Status: resp.Status, Header: resp.Header, Body: string(resp.Body)}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		parseStream(api, r)
		return r
	}
	parseError(gjson.ParseBytes(resp.Body), r)
	if resp.Status != http.StatusOK {
		return r
	}
	doc := gjson.ParseBytes(resp.Body)
	switch api {
	case apiChat:
		for _, choice := range doc.Get("choices").Array() {
			r.Text += choice.Get("message.content").String()
			for _, tc := range choice.Get("message.tool_calls").Array() {
				r.ToolArgs += tc.Get("function.arguments").String()
			}
		}
	case apiResponses:
		for _, item := range doc.Get("output").Array() {
			switch item.Get("type").String() {
			case "message":
				for _, part := range item.Get("content").Array() {
					r.Text += part.Get("text").String()
				}
			case "function_call":
				r.ToolArgs += item.Get("arguments").String()
			}
		}
	case apiMessages:
		for _, block := range doc.Get("content").Array() {
			switch block.Get("type").String() {
			case "text":
				r.Text += block.Get("text").String()
			case "tool_use":
				r.ToolArgs += block.Get("input").Raw
			}
		}
	}
	return r
}

func parseError(doc gjson.Result, r *llmResult) {
	if e := doc.Get("error"); e.Exists() {
		if e.Type == gjson.String {
			r.ErrorMessage = e.String()
			r.ErrorType = doc.Get("type").String()
			return
		}
		r.ErrorType = e.Get("type").String()
		r.ErrorMessage = e.Get("message").String()
	}
}

func parseStream(api string, r *llmResult) {
	sc := bufio.NewScanner(strings.NewReader(r.Body))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var completedText string
	sawDelta := false
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" || !gjson.Valid(data) {
			continue
		}
		ev := gjson.Parse(data)
		if ev.Get("error").Exists() || ev.Get("type").String() == "error" || ev.Get("type").String() == "response.failed" {
			parseError(ev, r)
			if r.ErrorMessage == "" {
				r.ErrorMessage = ev.Get("response.error.message").String()
				r.ErrorType = ev.Get("response.error.code").String()
			}
		}
		switch api {
		case apiChat:
			for _, choice := range ev.Get("choices").Array() {
				r.Text += choice.Get("delta.content").String()
				for _, tc := range choice.Get("delta.tool_calls").Array() {
					r.ToolArgs += tc.Get("function.arguments").String()
				}
			}
		case apiResponses:
			switch ev.Get("type").String() {
			case "response.output_text.delta":
				r.Text += ev.Get("delta").String()
				sawDelta = true
			case "response.function_call_arguments.delta":
				r.ToolArgs += ev.Get("delta").String()
			case "response.completed":
				for _, item := range ev.Get("response.output").Array() {
					for _, part := range item.Get("content").Array() {
						completedText += part.Get("text").String()
					}
				}
			}
		case apiMessages:
			if ev.Get("type").String() == "content_block_delta" {
				r.Text += ev.Get("delta.text").String()
				r.ToolArgs += ev.Get("delta.partial_json").String()
			}
		}
	}
	if api == apiResponses && !sawDelta {
		r.Text = completedText
	}
}

// requireOK fails unless the call succeeded.
func (r *llmResult) requireOK(tb testing.TB) *llmResult {
	tb.Helper()
	if r.Status != http.StatusOK || r.ErrorMessage != "" {
		tb.Fatalf("want a successful response, got HTTP %d: %s", r.Status, truncate(r.Body))
	}
	return r
}

func truncate(s string) string {
	if len(s) > 3000 {
		return s[:3000] + "...(truncated)"
	}
	return s
}

// mustJSON encodes v.
func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
