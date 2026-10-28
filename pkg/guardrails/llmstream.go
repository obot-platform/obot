package guardrails

import (
	"fmt"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// streamPart is one piece of streamed content: assistant text, reasoning, a refusal, or a
// tool call's arguments.
type streamPart struct {
	role string
	name string
	text strings.Builder
}

// streamState accumulates a streamed response so the complete text can be checked. While
// the stream is held, content is blanked as it passes through, and once checked the held
// chunks are rewritten: the first chunk of each part carries the part's complete (possibly
// redacted) text and later chunks carry none. Clients see the same content, delivered at
// once after the check.
//
// Every text field a chunk can carry is tracked by a key, so that what is observed is also
// what is blanked and rewritten. Complete-text fields (the "done" events, and whole output
// items) also count as observations, so text that a provider sends without deltas is
// checked as well.
type streamState struct {
	held  bool
	order []string
	parts map[string]*streamPart
	// names maps Responses output item IDs to their tool names.
	names map[string]string
}

// textField is one text field in a chunk, identified by its stream part key.
type textField struct {
	key  string
	role string
	name string
	text *string
	// delta is true for incremental text and false for a field that carries a part's
	// complete text.
	delta bool
}

func newStreamState() *streamState {
	return &streamState{parts: map[string]*streamPart{}, names: map[string]string{}}
}

func (s *streamState) part(key, role, name string) *streamPart {
	p, ok := s.parts[key]
	if !ok {
		p = &streamPart{role: role, name: name}
		s.parts[key] = p
		s.order = append(s.order, key)
	}
	if p.name == "" {
		p.name = name
	}
	return p
}

func chatTextKey(choice int) string { return fmt.Sprintf("c%d", choice) }

func chatFieldKey(choice int, field string) string { return fmt.Sprintf("c%d:%s", choice, field) }

func chatToolKey(choice int, index uint16) string { return fmt.Sprintf("c%d:t%d", choice, index) }

func respTextKey(itemID string, contentIndex int) string {
	return fmt.Sprintf("i:%s:%d", itemID, contentIndex)
}

func respRefusalKey(itemID string, contentIndex int) string {
	return fmt.Sprintf("f:%s:%d", itemID, contentIndex)
}

func respSummaryKey(itemID string, summaryIndex int) string {
	return fmt.Sprintf("s:%s:%d", itemID, summaryIndex)
}

func respArgsKey(itemID string) string { return "a:" + itemID }

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// chatDeltaFields returns the text fields of a Chat Completions delta.
func chatDeltaFields(choice int, d *schemas.ChatStreamResponseChoiceDelta) []textField {
	fields := []textField{
		{key: chatTextKey(choice), role: RoleAssistant, text: d.Content, delta: true},
		{key: chatFieldKey(choice, "refusal"), role: RoleAssistant, text: d.Refusal, delta: true},
		{key: chatFieldKey(choice, "reasoning"), role: RoleAssistant, text: d.Reasoning, delta: true},
	}
	for i := range d.ReasoningDetails {
		rd := &d.ReasoningDetails[i]
		fields = append(fields,
			textField{key: chatFieldKey(choice, fmt.Sprintf("rd%d:text", rd.Index)), role: RoleAssistant, text: rd.Text, delta: true},
			textField{key: chatFieldKey(choice, fmt.Sprintf("rd%d:summary", rd.Index)), role: RoleAssistant, text: rd.Summary, delta: true},
		)
	}
	for i := range d.ToolCalls {
		tc := &d.ToolCalls[i]
		fields = append(fields, textField{key: chatToolKey(choice, tc.Index), role: RoleToolCall, name: derefString(tc.Function.Name), text: &tc.Function.Arguments, delta: true})
	}
	return fields
}

// responsesEventFields returns the text fields of a Responses stream event, other than
// those in its output items.
func (s *streamState) responsesEventFields(ev *schemas.BifrostResponsesStreamResponse) []textField {
	itemID := derefString(ev.ItemID)
	ci := derefInt(ev.ContentIndex)
	si := derefInt(ev.SummaryIndex)
	switch ev.Type {
	case schemas.ResponsesStreamResponseTypeOutputTextDelta:
		return []textField{{key: respTextKey(itemID, ci), role: RoleAssistant, text: ev.Delta, delta: true}}
	case schemas.ResponsesStreamResponseTypeOutputTextDone:
		return []textField{{key: respTextKey(itemID, ci), role: RoleAssistant, text: ev.Text}}
	case schemas.ResponsesStreamResponseTypeRefusalDelta:
		return []textField{{key: respRefusalKey(itemID, ci), role: RoleAssistant, text: ev.Delta, delta: true}}
	case schemas.ResponsesStreamResponseTypeRefusalDone:
		return []textField{{key: respRefusalKey(itemID, ci), role: RoleAssistant, text: ev.Refusal}}
	case schemas.ResponsesStreamResponseTypeReasoningSummaryTextDelta:
		return []textField{{key: respSummaryKey(itemID, si), role: RoleAssistant, text: ev.Delta, delta: true}}
	case schemas.ResponsesStreamResponseTypeReasoningSummaryTextDone:
		return []textField{{key: respSummaryKey(itemID, si), role: RoleAssistant, text: ev.Text}}
	case schemas.ResponsesStreamResponseTypeFunctionCallArgumentsDelta, schemas.ResponsesStreamResponseTypeCustomToolCallInputDelta:
		return []textField{{key: respArgsKey(itemID), role: RoleToolCall, name: s.names[itemID], text: ev.Delta, delta: true}}
	case schemas.ResponsesStreamResponseTypeFunctionCallArgumentsDone:
		return []textField{{key: respArgsKey(itemID), role: RoleToolCall, name: s.names[itemID], text: ev.Arguments}}
	case schemas.ResponsesStreamResponseTypeCustomToolCallInputDone:
		return []textField{{key: respArgsKey(itemID), role: RoleToolCall, name: s.names[itemID], text: ev.Input}}
	case schemas.ResponsesStreamResponseTypeContentPartAdded, schemas.ResponsesStreamResponseTypeContentPartDone:
		if ev.Part != nil {
			return contentBlockFields(itemID, ci, ev.Part)
		}
	case schemas.ResponsesStreamResponseTypeReasoningSummaryPartAdded, schemas.ResponsesStreamResponseTypeReasoningSummaryPartDone:
		if ev.Part != nil {
			return []textField{{key: respSummaryKey(itemID, si), role: RoleAssistant, text: ev.Part.Text}}
		}
	}
	return nil
}

func contentBlockFields(itemID string, index int, b *schemas.ResponsesMessageContentBlock) []textField {
	fields := []textField{{key: respTextKey(itemID, index), role: RoleAssistant, text: b.Text}}
	if b.ResponsesOutputMessageContentRefusal != nil {
		fields = append(fields, textField{key: respRefusalKey(itemID, index), role: RoleAssistant, text: &b.Refusal})
	}
	return fields
}

// itemFields returns the complete-text fields of a Responses output item.
func itemFields(item *schemas.ResponsesMessage) []textField {
	id := derefString(item.ID)
	var fields []textField
	if item.ResponsesToolMessage != nil {
		name := derefString(item.Name)
		if item.Arguments != nil {
			fields = append(fields, textField{key: respArgsKey(id), role: RoleToolCall, name: name, text: item.Arguments})
		}
		if item.ResponsesCustomToolCall != nil {
			fields = append(fields, textField{key: respArgsKey(id), role: RoleToolCall, name: name, text: &item.ResponsesCustomToolCall.Input})
		}
	}
	if item.Content != nil {
		fields = append(fields, textField{key: respTextKey(id, 0), role: RoleAssistant, text: item.Content.ContentStr})
		for i := range item.Content.ContentBlocks {
			fields = append(fields, contentBlockFields(id, i, &item.Content.ContentBlocks[i])...)
		}
	}
	if item.ResponsesReasoning != nil {
		for i := range item.Summary {
			fields = append(fields, textField{key: respSummaryKey(id, i), role: RoleAssistant, text: &item.Summary[i].Text})
		}
	}
	return fields
}

// cloneItem copies an output item deeply enough that its text fields can be rewritten
// without changing the original.
func cloneItem(item schemas.ResponsesMessage) schemas.ResponsesMessage {
	if item.ResponsesToolMessage != nil {
		tm := *item.ResponsesToolMessage
		if tm.Arguments != nil {
			args := *tm.Arguments
			tm.Arguments = &args
		}
		if tm.ResponsesCustomToolCall != nil {
			ct := *tm.ResponsesCustomToolCall
			tm.ResponsesCustomToolCall = &ct
		}
		item.ResponsesToolMessage = &tm
	}
	if item.Content != nil {
		content := *item.Content
		if content.ContentStr != nil {
			str := *content.ContentStr
			content.ContentStr = &str
		}
		if content.ContentBlocks != nil {
			content.ContentBlocks = make([]schemas.ResponsesMessageContentBlock, len(item.Content.ContentBlocks))
			for i, b := range item.Content.ContentBlocks {
				if b.Text != nil {
					text := *b.Text
					b.Text = &text
				}
				if b.ResponsesOutputMessageContentRefusal != nil {
					refusal := *b.ResponsesOutputMessageContentRefusal
					b.ResponsesOutputMessageContentRefusal = &refusal
				}
				content.ContentBlocks[i] = b
			}
		}
		item.Content = &content
	}
	if item.ResponsesReasoning != nil {
		reasoning := *item.ResponsesReasoning
		reasoning.Summary = append([]schemas.ResponsesReasoningSummary(nil), item.Summary...)
		item.ResponsesReasoning = &reasoning
	}
	return item
}

// record adds observed text to its part. A complete-text field only counts when no deltas
// were seen for the part, since it repeats them.
func (s *streamState) record(f textField) {
	if f.text == nil || *f.text == "" {
		return
	}
	p := s.part(f.key, f.role, f.name)
	if f.delta {
		p.text.WriteString(*f.text)
	} else if p.text.Len() == 0 {
		p.text.WriteString(*f.text)
	}
}

// observe records a chunk's original content.
func (s *streamState) observe(resp *schemas.BifrostResponse) {
	switch {
	case resp.ChatResponse != nil:
		for i := range resp.ChatResponse.Choices {
			c := &resp.ChatResponse.Choices[i]
			if c.ChatStreamResponseChoice == nil || c.Delta == nil {
				continue
			}
			for _, f := range chatDeltaFields(c.Index, c.Delta) {
				s.record(f)
			}
		}
	case resp.ResponsesStreamResponse != nil:
		ev := resp.ResponsesStreamResponse
		if ev.Type == schemas.ResponsesStreamResponseTypeOutputItemAdded && ev.Item != nil && ev.Item.ID != nil && ev.Item.ResponsesToolMessage != nil && ev.Item.Name != nil {
			s.names[*ev.Item.ID] = *ev.Item.Name
		}
		for _, f := range s.responsesEventFields(ev) {
			s.record(f)
		}
		if ev.Item != nil {
			for _, f := range itemFields(ev.Item) {
				s.record(f)
			}
		}
		if ev.Response != nil {
			for i := range ev.Response.Output {
				for _, f := range itemFields(&ev.Response.Output[i]) {
					s.record(f)
				}
			}
		}
	}
}

func (s *streamState) segments() []Segment {
	out := make([]Segment, 0, len(s.order))
	for _, key := range s.order {
		p := s.parts[key]
		if p.text.Len() == 0 {
			continue
		}
		out = append(out, Segment{Role: p.role, Name: p.name, Text: p.text.String()})
	}
	return out
}

// finalText maps each part to the text clients receive.
func (s *streamState) finalText(d Decision) map[string]string {
	out := map[string]string{}
	i := 0
	for _, key := range s.order {
		p := s.parts[key]
		if p.text.Len() == 0 {
			continue
		}
		text := p.text.String()
		if d.Redacted && i < len(d.Segments) {
			text = d.Segments[i].Text
		}
		out[key] = text
		i++
	}
	return out
}

// rewriteField sets a field to what clients receive: nothing while blanking (final == nil);
// for a delta, the part's complete text on its first delta and nothing on later ones; for a
// complete-text field, the part's complete text. An empty complete-text field stays empty: it
// starts a part (an output item before its deltas) rather than repeating it, and filling it
// would deliver the text twice, since some clients' converters stream a non-empty starting
// item's text before its deltas.
func rewriteField(f textField, final map[string]string, emitted map[string]bool) {
	if f.text == nil || (final != nil && !f.delta && *f.text == "") {
		return
	}
	if final == nil {
		*f.text = ""
		return
	}
	text, ok := final[f.key]
	if !ok {
		return
	}
	if !f.delta {
		*f.text = text
		return
	}
	if emitted[f.key] {
		*f.text = ""
		return
	}
	emitted[f.key] = true
	*f.text = text
}

// blank returns a copy of a chunk with its content removed.
func (s *streamState) blank(resp *schemas.BifrostResponse) *schemas.BifrostResponse {
	out := *resp
	out.ChatResponse = s.rewriteChat(resp.ChatResponse, nil, nil)
	out.ResponsesStreamResponse = s.rewriteResponsesEvent(resp.ResponsesStreamResponse, nil, nil)
	return &out
}

func (s *streamState) rewriteChunk(c *schemas.BifrostStreamChunk, final map[string]string, emitted map[string]bool) *schemas.BifrostStreamChunk {
	if c == nil {
		return nil
	}
	out := *c
	out.BifrostChatResponse = s.rewriteChat(c.BifrostChatResponse, final, emitted)
	out.BifrostResponsesStreamResponse = s.rewriteResponsesEvent(c.BifrostResponsesStreamResponse, final, emitted)
	return &out
}

func (s *streamState) rewriteResponse(resp *schemas.BifrostResponse, final map[string]string, emitted map[string]bool) *schemas.BifrostResponse {
	out := *resp
	out.ChatResponse = s.rewriteChat(resp.ChatResponse, final, emitted)
	out.ResponsesStreamResponse = s.rewriteResponsesEvent(resp.ResponsesStreamResponse, final, emitted)
	return &out
}

func copyString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func (s *streamState) rewriteChat(r *schemas.BifrostChatResponse, final map[string]string, emitted map[string]bool) *schemas.BifrostChatResponse {
	if r == nil {
		return nil
	}
	out := *r
	out.Choices = make([]schemas.BifrostResponseChoice, len(r.Choices))
	for i, c := range r.Choices {
		out.Choices[i] = c
		if c.ChatStreamResponseChoice == nil || c.Delta == nil {
			continue
		}
		delta := *c.Delta
		delta.Content = copyString(delta.Content)
		delta.Refusal = copyString(delta.Refusal)
		delta.Reasoning = copyString(delta.Reasoning)
		if c.Delta.ReasoningDetails != nil {
			delta.ReasoningDetails = make([]schemas.ChatReasoningDetails, len(c.Delta.ReasoningDetails))
			for j, rd := range c.Delta.ReasoningDetails {
				rd.Text = copyString(rd.Text)
				rd.Summary = copyString(rd.Summary)
				delta.ReasoningDetails[j] = rd
			}
		}
		if c.Delta.ToolCalls != nil {
			delta.ToolCalls = append([]schemas.ChatAssistantMessageToolCall(nil), c.Delta.ToolCalls...)
		}
		for _, f := range chatDeltaFields(c.Index, &delta) {
			rewriteField(f, final, emitted)
		}
		choice := *c.ChatStreamResponseChoice
		choice.Delta = &delta
		out.Choices[i].ChatStreamResponseChoice = &choice
	}
	return &out
}

func (s *streamState) rewriteResponsesEvent(ev *schemas.BifrostResponsesStreamResponse, final map[string]string, emitted map[string]bool) *schemas.BifrostResponsesStreamResponse {
	if ev == nil {
		return nil
	}
	out := *ev
	out.Delta = copyString(ev.Delta)
	out.Text = copyString(ev.Text)
	out.Refusal = copyString(ev.Refusal)
	out.Arguments = copyString(ev.Arguments)
	out.Input = copyString(ev.Input)
	if ev.Part != nil {
		part := *ev.Part
		part.Text = copyString(part.Text)
		if part.ResponsesOutputMessageContentRefusal != nil {
			refusal := *part.ResponsesOutputMessageContentRefusal
			part.ResponsesOutputMessageContentRefusal = &refusal
		}
		out.Part = &part
	}
	for _, f := range s.responsesEventFields(&out) {
		rewriteField(f, final, emitted)
	}
	if ev.Item != nil {
		item := cloneItem(*ev.Item)
		for _, f := range itemFields(&item) {
			rewriteField(f, final, emitted)
		}
		out.Item = &item
	}
	if ev.Response != nil {
		resp := *ev.Response
		resp.Output = make([]schemas.ResponsesMessage, len(ev.Response.Output))
		for i, item := range ev.Response.Output {
			item = cloneItem(item)
			for _, f := range itemFields(&item) {
				rewriteField(f, final, emitted)
			}
			resp.Output[i] = item
		}
		out.Response = &resp
	}
	return &out
}
