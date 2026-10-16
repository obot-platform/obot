//go:build e2e

package harness

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Request formats the mock LLM serves.
const (
	FormatChat        = "openai-chat"
	FormatResponses   = "openai-responses"
	FormatInputTokens = "openai-input-tokens"
	FormatEmbeddings  = "openai-embeddings"
	FormatMessages    = "anthropic-messages"
	FormatCountTokens = "anthropic-count-tokens"
	FormatModels      = "models"
)

var (
	directiveRE = regexp.MustCompile(`@@([a-z]+)(?::([^@]*))?@@`)
)

// MockLLM is an upstream LLM provider speaking the OpenAI (chat completions, responses,
// embeddings, models) and Anthropic (messages, count_tokens, models) wire formats, streamed
// and not. Its behavior is controlled per request by directives in the request text, so
// tests running in parallel don't share state:
//
//	@@reply:<base64>@@      reply with this text (base64 so the request itself doesn't
//	                        contain what the reply does)
//	@@chunk:<n>@@           stream the reply in chunks of n bytes (default 4)
//	@@tool:<name>:<b64>@@   reply with a call to tool name with these JSON arguments
//	@@status:<code>@@       fail with this HTTP status
//	@@flaky:<n>@@           fail with 503 the first n times this exact request arrives
//	@@sleep:<ms>@@          wait before answering
//	@@usage:<in>:<out>@@    report this token usage (default 10 in, 5 out)
//
// A model whose name contains "broken" always fails with 503. Every request is recorded.
type MockLLM struct {
	URL string

	server   *http.Server
	listener net.Listener

	mu       sync.Mutex
	requests []RecordedRequest
	// models lists the models served to each upstream API key.
	models map[string][]string
	// seen counts identical request bodies, for @@flaky@@.
	seen map[string]int
}

// RecordedRequest is one request the mock received.
type RecordedRequest struct {
	Time   time.Time
	Method string
	Path   string
	Format string
	Model  string
	Stream bool
	// Key is the upstream API key the gateway sent.
	Key    string
	Header http.Header
	Body   string
}

type directives struct {
	reply    string
	hasReply bool
	chunk    int
	tool     string
	toolArgs string
	status   int
	flaky    int
	sleep    time.Duration
	in, out  int
}

type sse struct {
	w  http.ResponseWriter
	fl http.Flusher
}

// StartMockLLM starts the mock on a random localhost port.
func StartMockLLM() (*MockLLM, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m := &MockLLM{
		URL:      "http://" + l.Addr().String(),
		listener: l,
		models:   map[string][]string{},
		seen:     map[string]int{},
	}
	m.server = &http.Server{Handler: http.HandlerFunc(m.serve)}
	go func() { _ = m.server.Serve(l) }()
	return m, nil
}

// Close stops the mock.
func (m *MockLLM) Close() {
	_ = m.server.Close()
}

// SetModels sets the models listed for an upstream API key.
func (m *MockLLM) SetModels(key string, models ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.models[key] = models
}

// Requests returns the recorded requests whose body contains substr, oldest first.
func (m *MockLLM) Requests(substr string) []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []RecordedRequest
	for _, r := range m.requests {
		if substr == "" || strings.Contains(r.Body, substr) {
			out = append(out, r)
		}
	}
	return out
}

// InferenceRequests returns the recorded non-models requests containing substr.
func (m *MockLLM) InferenceRequests(substr string) []RecordedRequest {
	var out []RecordedRequest
	for _, r := range m.Requests(substr) {
		if r.Format != FormatModels {
			out = append(out, r)
		}
	}
	return out
}

// Reply returns a directive that makes the mock reply with text.
func Reply(text string) string {
	return "@@reply:" + base64.StdEncoding.EncodeToString([]byte(text)) + "@@"
}

// ToolCall returns a directive that makes the mock call tool name with args.
func ToolCall(name string, args map[string]any) string {
	data, _ := json.Marshal(args)
	return "@@tool:" + name + ":" + base64.StdEncoding.EncodeToString(data) + "@@"
}

func parseDirectives(text string) directives {
	d := directives{chunk: 4, in: 10, out: 5}
	for _, m := range directiveRE.FindAllStringSubmatch(text, -1) {
		arg := m[2]
		switch m[1] {
		case "reply":
			if b, err := base64.StdEncoding.DecodeString(arg); err == nil {
				d.reply, d.hasReply = string(b), true
			}
		case "chunk":
			if n, err := strconv.Atoi(arg); err == nil && n > 0 {
				d.chunk = n
			}
		case "tool":
			name, b64, _ := strings.Cut(arg, ":")
			if b, err := base64.StdEncoding.DecodeString(b64); err == nil {
				d.tool, d.toolArgs = name, string(b)
			}
		case "status":
			d.status, _ = strconv.Atoi(arg)
		case "flaky":
			d.flaky, _ = strconv.Atoi(arg)
		case "sleep":
			if ms, err := strconv.Atoi(arg); err == nil {
				d.sleep = time.Duration(ms) * time.Millisecond
			}
		case "usage":
			in, out, _ := strings.Cut(arg, ":")
			d.in, _ = strconv.Atoi(in)
			d.out, _ = strconv.Atoi(out)
		}
	}
	return d
}

// allText concatenates every string in a JSON document.
func allText(v any, sb *strings.Builder) {
	switch v := v.(type) {
	case string:
		sb.WriteString(v)
		sb.WriteString("\n")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			allText(v[k], sb)
		}
	case []any:
		for _, c := range v {
			allText(c, sb)
		}
	}
}

func requestKey(r *http.Request) string {
	if k := r.Header.Get("X-Api-Key"); k != "" {
		return k
	}
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func (m *MockLLM) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	path := r.URL.Path
	format := ""
	switch {
	case strings.HasSuffix(path, "/models") && r.Method == http.MethodGet:
		format = FormatModels
	case strings.HasSuffix(path, "/chat/completions"):
		format = FormatChat
	case strings.HasSuffix(path, "/responses/input_tokens"):
		format = FormatInputTokens
	case strings.HasSuffix(path, "/responses"):
		format = FormatResponses
	case strings.HasSuffix(path, "/embeddings"):
		format = FormatEmbeddings
	case strings.HasSuffix(path, "/messages/count_tokens"):
		format = FormatCountTokens
	case strings.HasSuffix(path, "/messages"):
		format = FormatMessages
	}

	var doc map[string]any
	_ = json.Unmarshal(body, &doc)
	model, _ := doc["model"].(string)
	stream, _ := doc["stream"].(bool)
	key := requestKey(r)

	m.mu.Lock()
	m.requests = append(m.requests, RecordedRequest{
		Time:   time.Now(),
		Method: r.Method,
		Path:   path,
		Format: format,
		Model:  model,
		Stream: stream,
		Key:    key,
		Header: r.Header.Clone(),
		Body:   string(body),
	})
	m.mu.Unlock()

	if format == FormatModels {
		m.serveModels(w, r, key)
		return
	}
	if format == "" {
		http.NotFound(w, r)
		return
	}

	var sb strings.Builder
	allText(doc, &sb)
	d := parseDirectives(sb.String())
	if d.sleep > 0 {
		select {
		case <-time.After(d.sleep):
		case <-r.Context().Done():
			return
		}
	}
	if strings.Contains(model, "broken") && d.status == 0 {
		d.status = http.StatusServiceUnavailable
	}
	if d.flaky > 0 {
		m.mu.Lock()
		m.seen[string(body)]++
		attempt := m.seen[string(body)]
		m.mu.Unlock()
		if attempt <= d.flaky && d.status == 0 {
			d.status = http.StatusServiceUnavailable
		}
	}
	anthropic := format == FormatMessages || format == FormatCountTokens
	if d.status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(d.status)
		if anthropic {
			_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":"api_error","message":"mock upstream failure %d"}}`, d.status)
		} else {
			_, _ = fmt.Fprintf(w, `{"error":{"message":"mock upstream failure %d","type":"server_error","code":"mock_failure"}}`, d.status)
		}
		return
	}
	if !d.hasReply {
		d.reply = fmt.Sprintf("mock reply from %s for %s", format, model)
	}

	switch format {
	case FormatChat:
		m.serveChat(w, model, stream, d)
	case FormatResponses:
		m.serveResponses(w, model, stream, d)
	case FormatInputTokens:
		writeJSON(w, map[string]any{"object": "response.input_tokens", "input_tokens": d.in})
	case FormatEmbeddings:
		m.serveEmbeddings(w, model, doc, d)
	case FormatMessages:
		m.serveMessages(w, model, stream, d)
	case FormatCountTokens:
		writeJSON(w, map[string]any{"input_tokens": d.in})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (m *MockLLM) serveModels(w http.ResponseWriter, r *http.Request, key string) {
	m.mu.Lock()
	models := slices.Clone(m.models[key])
	m.mu.Unlock()
	if r.Header.Get("Anthropic-Version") != "" || r.Header.Get("X-Api-Key") != "" {
		data := make([]map[string]any, 0, len(models))
		for _, id := range models {
			data = append(data, map[string]any{"type": "model", "id": id, "display_name": id, "created_at": "2025-01-01T00:00:00Z"})
		}
		resp := map[string]any{"data": data, "has_more": false}
		if len(models) > 0 {
			resp["first_id"], resp["last_id"] = models[0], models[len(models)-1]
		}
		writeJSON(w, resp)
		return
	}
	data := make([]map[string]any, 0, len(models))
	for _, id := range models {
		data = append(data, map[string]any{"id": id, "object": "model", "created": 1700000000, "owned_by": "mock"})
	}
	writeJSON(w, map[string]any{"object": "list", "data": data})
}

func chunks(s string, n int) []string {
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	if s != "" || len(out) == 0 {
		out = append(out, s)
	}
	return out
}

func newSSE(w http.ResponseWriter) *sse {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	return &sse{w: w, fl: fl}
}

// event writes one SSE event; an empty name writes a data-only event.
func (s *sse) event(name string, v any) {
	var data []byte
	if str, ok := v.(string); ok {
		data = []byte(str)
	} else {
		data, _ = json.Marshal(v)
	}
	if name != "" {
		_, _ = fmt.Fprintf(s.w, "event: %s\n", name)
	}
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", data)
	if s.fl != nil {
		s.fl.Flush()
	}
	// Give each chunk its own network write so the gateway sees separate events.
	time.Sleep(2 * time.Millisecond)
}

func (m *MockLLM) serveChat(w http.ResponseWriter, model string, stream bool, d directives) {
	usage := map[string]any{"prompt_tokens": d.in, "completion_tokens": d.out, "total_tokens": d.in + d.out}
	if !stream {
		msg := map[string]any{"role": "assistant", "content": d.reply}
		finish := "stop"
		if d.tool != "" {
			msg = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"id": "call_mock1", "type": "function", "function": map[string]any{"name": d.tool, "arguments": d.toolArgs},
			}}}
			finish = "tool_calls"
		}
		writeJSON(w, map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion", "created": time.Now().Unix(), "model": model,
			"choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}},
			"usage":   usage,
		})
		return
	}
	s := newSSE(w)
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
	}
	s.event("", chunk(map[string]any{"role": "assistant", "content": ""}, nil))
	finish := "stop"
	if d.tool != "" {
		s.event("", chunk(map[string]any{"tool_calls": []any{map[string]any{
			"index": 0, "id": "call_mock1", "type": "function", "function": map[string]any{"name": d.tool, "arguments": ""},
		}}}, nil))
		for _, part := range chunks(d.toolArgs, d.chunk) {
			s.event("", chunk(map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "function": map[string]any{"arguments": part},
			}}}, nil))
		}
		finish = "tool_calls"
	} else {
		for _, part := range chunks(d.reply, d.chunk) {
			s.event("", chunk(map[string]any{"content": part}, nil))
		}
	}
	s.event("", chunk(map[string]any{}, finish))
	s.event("", map[string]any{
		"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
		"choices": []any{}, "usage": usage,
	})
	s.event("", "[DONE]")
}

func (m *MockLLM) serveResponses(w http.ResponseWriter, model string, stream bool, d directives) {
	usage := map[string]any{"input_tokens": d.in, "output_tokens": d.out, "total_tokens": d.in + d.out}
	var item map[string]any
	if d.tool != "" {
		item = map[string]any{"type": "function_call", "id": "fc_mock1", "call_id": "call_mock1", "name": d.tool, "arguments": d.toolArgs, "status": "completed"}
	} else {
		item = map[string]any{"type": "message", "id": "msg_mock1", "status": "completed", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": d.reply, "annotations": []any{}}}}
	}
	response := func(status string, output []any, withUsage bool) map[string]any {
		r := map[string]any{"id": "resp_mock1", "object": "response", "created_at": time.Now().Unix(), "status": status, "model": model, "output": output}
		if withUsage {
			r["usage"] = usage
		}
		return r
	}
	if !stream {
		writeJSON(w, response("completed", []any{item}, true))
		return
	}
	s := newSSE(w)
	seq := 0
	ev := func(typ string, fields map[string]any) {
		fields["type"] = typ
		fields["sequence_number"] = seq
		seq++
		s.event(typ, fields)
	}
	ev("response.created", map[string]any{"response": response("in_progress", []any{}, false)})
	ev("response.in_progress", map[string]any{"response": response("in_progress", []any{}, false)})
	if d.tool != "" {
		added := map[string]any{"type": "function_call", "id": "fc_mock1", "call_id": "call_mock1", "name": d.tool, "arguments": "", "status": "in_progress"}
		ev("response.output_item.added", map[string]any{"output_index": 0, "item": added})
		for _, part := range chunks(d.toolArgs, d.chunk) {
			ev("response.function_call_arguments.delta", map[string]any{"item_id": "fc_mock1", "output_index": 0, "delta": part})
		}
		ev("response.function_call_arguments.done", map[string]any{"item_id": "fc_mock1", "output_index": 0, "arguments": d.toolArgs})
	} else {
		added := map[string]any{"type": "message", "id": "msg_mock1", "status": "in_progress", "role": "assistant", "content": []any{}}
		ev("response.output_item.added", map[string]any{"output_index": 0, "item": added})
		ev("response.content_part.added", map[string]any{"item_id": "msg_mock1", "output_index": 0, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
		for _, part := range chunks(d.reply, d.chunk) {
			ev("response.output_text.delta", map[string]any{"item_id": "msg_mock1", "output_index": 0, "content_index": 0, "delta": part})
		}
		ev("response.output_text.done", map[string]any{"item_id": "msg_mock1", "output_index": 0, "content_index": 0, "text": d.reply})
		ev("response.content_part.done", map[string]any{"item_id": "msg_mock1", "output_index": 0, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": d.reply, "annotations": []any{}}})
	}
	ev("response.output_item.done", map[string]any{"output_index": 0, "item": item})
	ev("response.completed", map[string]any{"response": response("completed", []any{item}, true)})
}

func (m *MockLLM) serveEmbeddings(w http.ResponseWriter, model string, doc map[string]any, d directives) {
	n := 1
	if arr, ok := doc["input"].([]any); ok {
		n = len(arr)
	}
	data := make([]any, 0, n)
	for i := range n {
		data = append(data, map[string]any{"object": "embedding", "index": i, "embedding": []float64{0.1, 0.2, 0.3, float64(i)}})
	}
	writeJSON(w, map[string]any{
		"object": "list", "data": data, "model": model,
		"usage": map[string]any{"prompt_tokens": d.in, "total_tokens": d.in},
	})
}

func (m *MockLLM) serveMessages(w http.ResponseWriter, model string, stream bool, d directives) {
	var block map[string]any
	stop := "end_turn"
	if d.tool != "" {
		var input any = map[string]any{}
		_ = json.Unmarshal([]byte(d.toolArgs), &input)
		block = map[string]any{"type": "tool_use", "id": "toolu_mock1", "name": d.tool, "input": input}
		stop = "tool_use"
	} else {
		block = map[string]any{"type": "text", "text": d.reply}
	}
	if !stream {
		writeJSON(w, map[string]any{
			"id": "msg_mock1", "type": "message", "role": "assistant", "model": model,
			"content": []any{block}, "stop_reason": stop, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": d.in, "output_tokens": d.out},
		})
		return
	}
	s := newSSE(w)
	s.event("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_mock1", "type": "message", "role": "assistant", "model": model, "content": []any{},
		"stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": d.in, "output_tokens": 1},
	}})
	if d.tool != "" {
		s.event("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "tool_use", "id": "toolu_mock1", "name": d.tool, "input": map[string]any{}}})
		for _, part := range chunks(d.toolArgs, d.chunk) {
			s.event("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": part}})
		}
	} else {
		s.event("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""}})
		s.event("ping", map[string]any{"type": "ping"})
		for _, part := range chunks(d.reply, d.chunk) {
			s.event("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "text_delta", "text": part}})
		}
	}
	s.event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	s.event("message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": d.out}})
	s.event("message_stop", map[string]any{"type": "message_stop"})
}
