package guardrails

import (
	"github.com/maximhq/bifrost/core/schemas"
)

// textRef is one piece of text in a Bifrost request or response, with a way to replace it.
type textRef struct {
	segment Segment
	set     func(string)
}

type textRefs []textRef

func (t textRefs) segments() []Segment {
	out := make([]Segment, len(t))
	for i, r := range t {
		out[i] = r.segment
	}
	return out
}

// apply writes changed segment text back into the request or response.
func (t textRefs) apply(segments []Segment) {
	for i, r := range t {
		if i < len(segments) && segments[i].Text != r.segment.Text {
			r.set(segments[i].Text)
		}
	}
}

func (t *textRefs) add(role, name string, text *string) {
	if text == nil || *text == "" {
		return
	}
	*t = append(*t, textRef{segment: Segment{Role: role, Name: name, Text: *text}, set: func(s string) { *text = s }})
}

func chatRole(role schemas.ChatMessageRole) string {
	switch role {
	case schemas.ChatMessageRoleSystem, schemas.ChatMessageRoleDeveloper:
		return RoleSystem
	case schemas.ChatMessageRoleTool:
		return RoleToolResult
	case schemas.ChatMessageRoleUser:
		return RoleUser
	default:
		return RoleAssistant
	}
}

func (t *textRefs) addChatMessage(m *schemas.ChatMessage) {
	role := chatRole(m.Role)
	if m.Content != nil {
		t.add(role, "", m.Content.ContentStr)
		for i := range m.Content.ContentBlocks {
			t.add(role, "", m.Content.ContentBlocks[i].Text)
		}
	}
	if m.ChatAssistantMessage != nil {
		t.add(role, "", m.Refusal)
		t.add(role, "", m.Reasoning)
		for i := range m.ReasoningDetails {
			t.add(role, "", m.ReasoningDetails[i].Text)
			t.add(role, "", m.ReasoningDetails[i].Summary)
		}
		for i := range m.ToolCalls {
			tc := &m.ToolCalls[i]
			name := ""
			if tc.Function.Name != nil {
				name = *tc.Function.Name
			}
			t.add(RoleToolCall, name, &tc.Function.Arguments)
		}
	}
}

func responsesRole(role *schemas.ResponsesMessageRoleType) string {
	if role == nil {
		return RoleUser
	}
	switch *role {
	case schemas.ResponsesInputMessageRoleSystem, schemas.ResponsesInputMessageRoleDeveloper:
		return RoleSystem
	case schemas.ResponsesInputMessageRoleAssistant:
		return RoleAssistant
	default:
		return RoleUser
	}
}

func (t *textRefs) addResponsesMessage(m *schemas.ResponsesMessage) {
	if m.Type != nil && m.ResponsesToolMessage != nil {
		switch *m.Type {
		case schemas.ResponsesMessageTypeFunctionCall:
			name := ""
			if m.Name != nil {
				name = *m.Name
			}
			t.add(RoleToolCall, name, m.Arguments)
			return
		case schemas.ResponsesMessageTypeCustomToolCall:
			if m.ResponsesCustomToolCall != nil {
				name := ""
				if m.Name != nil {
					name = *m.Name
				}
				t.add(RoleToolCall, name, &m.ResponsesCustomToolCall.Input)
			}
			return
		case schemas.ResponsesMessageTypeFunctionCallOutput, schemas.ResponsesMessageTypeCustomToolCallOutput:
			if m.Output != nil {
				t.add(RoleToolResult, "", m.Output.ResponsesToolCallOutputStr)
				for i := range m.Output.ResponsesFunctionToolCallOutputBlocks {
					t.add(RoleToolResult, "", m.Output.ResponsesFunctionToolCallOutputBlocks[i].Text)
				}
			}
			return
		}
	}
	role := responsesRole(m.Role)
	if m.Content != nil {
		t.add(role, "", m.Content.ContentStr)
		for i := range m.Content.ContentBlocks {
			b := &m.Content.ContentBlocks[i]
			t.add(role, "", b.Text)
			if b.ResponsesOutputMessageContentRefusal != nil {
				t.add(role, "", &b.Refusal)
			}
		}
	}
	if m.ResponsesReasoning != nil {
		for i := range m.Summary {
			t.add(RoleAssistant, "", &m.Summary[i].Text)
		}
	}
}

func (t *textRefs) addResponsesRequest(r *schemas.BifrostResponsesRequest) {
	if r.Params != nil {
		t.add(RoleSystem, "", r.Params.Instructions)
	}
	for i := range r.Input {
		t.addResponsesMessage(&r.Input[i])
	}
}

// requestText returns the text of a request that guardrails check.
func requestText(req *schemas.BifrostRequest) textRefs {
	var refs textRefs
	switch {
	case req.ChatRequest != nil:
		for i := range req.ChatRequest.Input {
			refs.addChatMessage(&req.ChatRequest.Input[i])
		}
	case req.ResponsesRequest != nil:
		refs.addResponsesRequest(req.ResponsesRequest)
	case req.CountTokensRequest != nil:
		// Token counting sends the whole prompt to the provider, so it is checked too.
		refs.addResponsesRequest(req.CountTokensRequest)
	case req.EmbeddingRequest != nil:
		// Embedding inputs can mix text with images, audio, video and files; only text is
		// checked.
		for i := range req.EmbeddingRequest.Input {
			content := req.EmbeddingRequest.Input[i].Content
			for j := range content {
				if content[j].Type == schemas.EmbeddingContentPartTypeText {
					refs.add(RoleUser, "", content[j].Text)
				}
			}
		}
	}
	return refs
}

// responseText returns the text of a complete (non-streamed) response.
func responseText(resp *schemas.BifrostResponse) textRefs {
	var refs textRefs
	switch {
	case resp.ChatResponse != nil:
		for i := range resp.ChatResponse.Choices {
			c := &resp.ChatResponse.Choices[i]
			if c.ChatNonStreamResponseChoice != nil && c.Message != nil {
				refs.addChatMessage(c.Message)
			}
		}
	case resp.ResponsesResponse != nil:
		for i := range resp.ResponsesResponse.Output {
			refs.addResponsesMessage(&resp.ResponsesResponse.Output[i])
		}
	}
	return refs
}

// requestFields returns the model ("provider/model") and provider of a request.
func requestFields(req *schemas.BifrostRequest) (string, string) {
	provider, model, _ := req.GetRequestFields()
	if provider == "" {
		return model, ""
	}
	return string(provider) + "/" + model, string(provider)
}
