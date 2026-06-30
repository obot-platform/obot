package server

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/system"
)

type llmResponseAccumulator struct {
	modelProvider string
	pending       string
	sawSSE        bool
	rawJSON       strings.Builder
	responseID    string

	openAI    openAIResponseAccumulator
	anthropic anthropicResponseAccumulator
}

func newLLMResponseAccumulator(modelProvider string) *llmResponseAccumulator {
	return &llmResponseAccumulator{modelProvider: modelProvider}
}

func (a *llmResponseAccumulator) Write(p []byte) {
	if a == nil || len(p) == 0 {
		return
	}

	s := a.pending + string(p)
	a.pending = ""
	for {
		line, rest, ok := strings.Cut(s, "\n")
		if !ok {
			a.pending = s
			return
		}
		a.processLine(strings.TrimSuffix(line, "\r"))
		s = rest
	}
}

func (a *llmResponseAccumulator) JSON() string {
	if a == nil {
		return "{}"
	}
	if a.pending != "" {
		a.processLine(strings.TrimSuffix(a.pending, "\r"))
		a.pending = ""
	}
	if !a.sawSSE && a.rawJSON.Len() > 0 {
		return compactJSONObject(a.rawJSON.String())
	}

	var raw string
	switch a.modelProvider {
	case system.OpenAIModelProvider:
		raw = a.openAI.JSON()
	case system.AnthropicModelProvider:
		raw = a.anthropic.JSON()
	}
	if raw == "" {
		return "{}"
	}
	return compactJSONObject(raw)
}

func (a *llmResponseAccumulator) ResponseID() string {
	if a == nil {
		return ""
	}
	if a.responseID != "" {
		return a.responseID
	}
	switch a.modelProvider {
	case system.OpenAIModelProvider:
		return a.openAI.ResponseID()
	case system.AnthropicModelProvider:
		return a.anthropic.ResponseID()
	default:
		return ""
	}
}

func (a *llmResponseAccumulator) processLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	data, ok := strings.CutPrefix(trimmed, "data:")
	if ok {
		a.sawSSE = true
		body := strings.TrimSpace(data)
		if body == "" || body == "[DONE]" || !json.Valid([]byte(body)) {
			return
		}
		a.processJSON(body)
		return
	}

	if !a.sawSSE && (a.rawJSON.Len() > 0 || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
		a.rawJSON.WriteString(line)
	}
}

func (a *llmResponseAccumulator) processJSON(body string) {
	if a.modelProvider == system.OpenAIModelProvider {
		a.openAI.Process(body)
		if a.responseID == "" {
			a.responseID = a.openAI.ResponseID()
		}
		return
	}
	if a.modelProvider == system.AnthropicModelProvider {
		a.anthropic.Process(body)
		if a.responseID == "" {
			a.responseID = a.anthropic.ResponseID()
		}
	}
}

type openAIResponseAccumulator struct {
	response map[string]any
	finalRaw string
}

func (a *openAIResponseAccumulator) Process(body string) {
	var event map[string]any
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return
	}

	typ, _ := event["type"].(string)
	if typ == "response.completed" || typ == "response.failed" || typ == "response.incomplete" {
		if response, ok := event["response"].(map[string]any); ok {
			a.response = cloneMap(response)
			a.finalRaw = mustMarshalString(response)
		}
		return
	}

	switch typ {
	case "response.created":
		if response, ok := event["response"].(map[string]any); ok {
			a.response = cloneMap(response)
			if _, ok := a.response["output"].([]any); !ok {
				a.response["output"] = []any{}
			}
		}
	case "response.output_item.added", "response.output_item.done":
		item, ok := event["item"].(map[string]any)
		if !ok {
			return
		}
		a.setOutputItem(intValue(event["output_index"]), cloneMap(item))
	case "response.output_text.delta":
		item := a.ensureOutputItem(intValue(event["output_index"]))
		item["type"] = defaultString(item["type"], "message")
		item["id"] = defaultString(item["id"], stringValue(event["item_id"]))
		item["role"] = defaultString(item["role"], "assistant")
		content := ensureSlice(item, "content")
		contentIndex := intValue(event["content_index"])
		for len(content) <= contentIndex {
			content = append(content, map[string]any{"type": "output_text", "text": ""})
		}
		block, _ := content[contentIndex].(map[string]any)
		if block == nil {
			block = map[string]any{"type": "output_text"}
		}
		block["type"] = defaultString(block["type"], "output_text")
		block["text"] = stringValue(block["text"]) + stringValue(event["delta"])
		content[contentIndex] = block
		item["content"] = content
	case "response.function_call_arguments.delta":
		item := a.ensureOutputItem(intValue(event["output_index"]))
		item["type"] = defaultString(item["type"], "function_call")
		item["id"] = defaultString(item["id"], stringValue(event["item_id"]))
		item["arguments"] = stringValue(item["arguments"]) + stringValue(event["delta"])
	}
}

func (a *openAIResponseAccumulator) JSON() string {
	if a.finalRaw != "" {
		return a.finalRaw
	}
	return mustMarshalString(a.response)
}

func (a *openAIResponseAccumulator) ResponseID() string {
	if a.response == nil {
		return ""
	}
	return stringValue(a.response["id"])
}

func (a *openAIResponseAccumulator) ensureResponse() map[string]any {
	if a.response == nil {
		a.response = map[string]any{"object": "response", "output": []any{}}
	}
	return a.response
}

func (a *openAIResponseAccumulator) setOutputItem(index int, item map[string]any) {
	response := a.ensureResponse()
	output := ensureSlice(response, "output")
	if index < 0 {
		index = len(output)
	}
	for len(output) <= index {
		output = append(output, map[string]any{})
	}
	output[index] = item
	response["output"] = output
}

func (a *openAIResponseAccumulator) ensureOutputItem(index int) map[string]any {
	response := a.ensureResponse()
	output := ensureSlice(response, "output")
	if index < 0 {
		index = 0
	}
	for len(output) <= index {
		output = append(output, map[string]any{})
	}
	item, _ := output[index].(map[string]any)
	if item == nil {
		item = map[string]any{}
	}
	output[index] = item
	response["output"] = output
	return item
}

type anthropicResponseAccumulator struct {
	message       map[string]any
	partialInputs map[int]*strings.Builder
}

func (a *anthropicResponseAccumulator) Process(body string) {
	var event map[string]any
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		return
	}

	switch stringValue(event["type"]) {
	case "message_start":
		if message, ok := event["message"].(map[string]any); ok {
			a.message = cloneMap(message)
		}
	case "content_block_start":
		block, ok := event["content_block"].(map[string]any)
		if ok {
			a.setContentBlock(intValue(event["index"]), cloneMap(block))
		}
	case "content_block_delta":
		a.applyContentDelta(intValue(event["index"]), event["delta"])
	case "content_block_stop":
		a.finalizeInput(intValue(event["index"]))
	case "message_delta":
		message := a.ensureMessage()
		if delta, ok := event["delta"].(map[string]any); ok {
			for k, v := range delta {
				message[k] = v
			}
		}
		if usage, ok := event["usage"].(map[string]any); ok {
			message["usage"] = cloneMap(usage)
		}
	}
}

func (a *anthropicResponseAccumulator) JSON() string {
	for index := range a.partialInputs {
		a.finalizeInput(index)
	}
	return mustMarshalString(a.message)
}

func (a *anthropicResponseAccumulator) ResponseID() string {
	if a.message == nil {
		return ""
	}
	return stringValue(a.message["id"])
}

func (a *anthropicResponseAccumulator) ensureMessage() map[string]any {
	if a.message == nil {
		a.message = map[string]any{"type": "message", "role": "assistant", "content": []any{}}
	}
	return a.message
}

func (a *anthropicResponseAccumulator) setContentBlock(index int, block map[string]any) {
	message := a.ensureMessage()
	content := ensureSlice(message, "content")
	if index < 0 {
		index = len(content)
	}
	for len(content) <= index {
		content = append(content, map[string]any{})
	}
	content[index] = block
	message["content"] = content
}

func (a *anthropicResponseAccumulator) contentBlock(index int) map[string]any {
	message := a.ensureMessage()
	content := ensureSlice(message, "content")
	if index < 0 {
		index = 0
	}
	for len(content) <= index {
		content = append(content, map[string]any{})
	}
	block, _ := content[index].(map[string]any)
	if block == nil {
		block = map[string]any{}
	}
	content[index] = block
	message["content"] = content
	return block
}

func (a *anthropicResponseAccumulator) applyContentDelta(index int, raw any) {
	delta, ok := raw.(map[string]any)
	if !ok {
		return
	}
	block := a.contentBlock(index)
	switch stringValue(delta["type"]) {
	case "text_delta":
		block["text"] = stringValue(block["text"]) + stringValue(delta["text"])
	case "input_json_delta":
		if a.partialInputs == nil {
			a.partialInputs = map[int]*strings.Builder{}
		}
		b := a.partialInputs[index]
		if b == nil {
			b = new(strings.Builder)
			a.partialInputs[index] = b
		}
		b.WriteString(stringValue(delta["partial_json"]))
	case "thinking_delta":
		block["thinking"] = stringValue(block["thinking"]) + stringValue(delta["thinking"])
	case "signature_delta":
		block["signature"] = stringValue(block["signature"]) + stringValue(delta["signature"])
	}
}

func (a *anthropicResponseAccumulator) finalizeInput(index int) {
	if a.partialInputs == nil || a.partialInputs[index] == nil {
		return
	}
	partial := a.partialInputs[index].String()
	block := a.contentBlock(index)
	var input any
	if err := json.Unmarshal([]byte(partial), &input); err == nil {
		block["input"] = input
	} else if partial != "" {
		block["input_partial_json"] = partial
	}
	delete(a.partialInputs, index)
}

func ensureSlice(m map[string]any, key string) []any {
	v, _ := m[key].([]any)
	if v == nil {
		v = []any{}
	}
	return v
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func mustMarshalString(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func compactJSONObject(raw string) string {
	if !json.Valid([]byte(raw)) {
		return "{}"
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "{}"
	}
	if _, ok := v.(map[string]any); !ok {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(raw)); err != nil {
		return "{}"
	}
	return buf.String()
}

func intValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		i, _ := strconv.Atoi(x.String())
		return i
	default:
		return 0
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func defaultString(v any, fallback string) string {
	if s := stringValue(v); s != "" {
		return s
	}
	return fallback
}
