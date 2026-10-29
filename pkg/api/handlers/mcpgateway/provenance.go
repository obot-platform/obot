package mcpgateway

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/provenance"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	// provenanceHookName is the provenance filter's display name in MCP audit logs.
	provenanceHookName = "Session policy"
	// provenanceDecisionBudget bounds how long a tool call waits for Obot Policy.
	provenanceDecisionBudget = 10 * time.Second
	// maxProvenanceContentBytes caps how much of a result is scanned for labels.
	maxProvenanceContentBytes = 1 << 20
	// defaultMCPAgent names the agent when the MCP client did not identify itself.
	defaultMCPAgent = "mcp-client"
)

// mcpProvenance is the MCP gateway's provenance adapter for one proxied exchange: every
// tools/call is decided by the provenance engine before it is proxied, and every result is
// ingested into the session graph with the tool's capability labels.
type mcpProvenance struct {
	engine  *provenance.Engine
	subject provenance.Subject
	session provenance.SessionKey
	// server is the server's display name and serverID its MCPServer ID.
	server, serverID string
	// descriptions caches tool descriptions seen in tools/list results, keyed by server and
	// tool, so capability labelling of a call can use the tool's description.
	descriptions *sync.Map
}

// provenanceHooks returns the hook mappings that route tool calls, tool listings and
// resource reads through the provenance filter. They are prepended to a server's configured
// filters so the session graph sees messages as the client and the server sent them.
func provenanceHooks() mcp.Hooks {
	target := mcp.HookTarget{Target: mcp.HookNativeProvenance, Name: provenanceHookName, Native: mcp.HookNativeProvenance}
	return mcp.Hooks{
		{Name: "tools/call", Targets: []mcp.HookTarget{target}},
		{Name: "tools/list", Targets: []mcp.HookTarget{target}},
		{Name: "resources/read", Targets: []mcp.HookTarget{target}},
	}
}

// newMCPProvenance builds the adapter for a request. The session key is the client's MCP
// session, which the audit entry holds authoritatively (a virtual session's header is
// removed from the request before hooks run).
func newMCPProvenance(engine *provenance.Engine, caller user.Info, serverConfig mcp.ServerConfig, req *http.Request, audit *proxyAudit, descriptions *sync.Map) *mcpProvenance {
	var clientName, clientVersion, sessionID string
	if audit != nil {
		clientName, clientVersion, sessionID = audit.entry.ClientName, audit.entry.ClientVersion, audit.entry.SessionID
	} else if req != nil {
		sessionID = mcpSessionID(req.Header, req.URL)
	}
	agent := cmp.Or(clientName, defaultMCPAgent)

	var apiKeyID string
	if attribution, ok := principal.APIKeyAttributionFromUser(caller); ok {
		apiKeyID = strconv.FormatUint(uint64(attribution.ID), 10)
	}
	var email string
	var groups []string
	userID := serverConfig.AuditLogMetadata["userID"]
	if caller != nil {
		extra := caller.GetExtra()
		email = cmp.Or(extra["email"]...)
		groups = extra["auth_provider_groups"]
		userID = cmp.Or(userID, caller.GetUID())
	}

	// A harness that already knows its provenance session (the LLM gateway returns it as
	// X-Obot-Session-ID) names it explicitly, so its tool calls land in the same session
	// as its model turns.
	var explicit string
	if req != nil {
		explicit = req.Header.Get("X-Obot-Session-ID")
	}

	actor := provenance.Actor{Agent: agent, AgentVersion: clientVersion, APIKeyID: apiKeyID, ClientSessionID: sessionID}
	return &mcpProvenance{
		engine:       engine,
		subject:      provenance.Subject{UserID: userID, Email: email, Groups: groups, Actor: actor},
		session:      provenance.SessionKey{Explicit: explicit, ClientSessionID: sessionID, Agent: agent, UserID: userID, APIKeyID: apiKeyID},
		server:       cmp.Or(serverConfig.MCPServerDisplayName, serverConfig.MCPServerName),
		serverID:     cmp.Or(serverConfig.AuditLogMetadata["mcpID"], serverConfig.MCPServerName),
		descriptions: descriptions,
	}
}

// run filters one message with the provenance engine and answers like a filter server
// would: a denied call rejects the message and a redact effect mutates its arguments. It
// returns nil when nothing applies. For a request it returns the ID of the event the call
// was recorded as; for a response, callEventID is that event and the result is ingested as
// its child. Errors fail closed, like every other filter.
func (p *mcpProvenance) run(ctx context.Context, message mcp.Message, method, name, direction, callEventID string) (*mcp.SessionMessageHook, string, error) {
	if p == nil || p.engine == nil {
		return nil, "", errors.New("session policy is not available")
	}
	switch direction {
	case "request":
		if method == "tools/call" {
			return p.decideToolCall(ctx, message, name)
		}
	case "response":
		if message.Error != nil || len(message.Result) == 0 {
			return nil, "", nil
		}
		// Ingesting a result only updates the session graph; a storage error
		// must not withhold a result the server already produced, so the
		// response side fails open and logs.
		var err error
		switch method {
		case "tools/call":
			err = p.ingestToolResult(ctx, message.Result, name, callEventID)
		case "tools/list":
			p.cacheToolList(ctx, message.Result)
		case "resources/read":
			err = p.ingestResource(ctx, message.Result, name, callEventID)
		}
		if err != nil {
			slog.Warn("Session policy could not record an MCP result", "server", p.server, "method", method, "error", err)
		}
	}
	return nil, "", nil
}

// decideToolCall asks Obot Policy whether the call may proceed.
func (p *mcpProvenance) decideToolCall(ctx context.Context, message mcp.Message, tool string) (*mcp.SessionMessageHook, string, error) {
	var params struct {
		Arguments map[string]any `json:"arguments"`
	}
	if len(message.Params) > 0 {
		if err := decodeJSONNumbers(message.Params, &params); err != nil {
			return nil, "", fmt.Errorf("decode tool call arguments: %w", err)
		}
	}

	caps := p.capabilities(ctx, tool)
	action := provenance.Action{
		Surface:      provenance.SurfaceMCP,
		Kind:         provenance.KindToolCall,
		Tool:         tool,
		Server:       p.server,
		ServerID:     p.serverID,
		Capabilities: caps,
		Sink:         provenance.SinkFromCapabilities(caps),
		Source:       provenance.SourceFromCapabilities(caps),
		Arguments:    params.Arguments,
	}
	event, decision, err := p.engine.Decide(ctx, provenance.DecideRequest{
		Subject: p.subject, Session: p.session, Action: action, Budget: provenanceDecisionBudget,
	})
	if err != nil {
		return nil, "", fmt.Errorf("decide tool call: %w", err)
	}

	if !decision.Allow {
		reason := decision.Reason
		if decision.ApprovalID != "" {
			reason += " Approval " + decision.ApprovalID + " is pending."
		}
		return &mcp.SessionMessageHook{Accept: false, Message: &message, Reason: reason}, event.ID, nil
	}

	output := &mcp.SessionMessageHook{Accept: true, Message: &message}
	if decision.Effect != provenance.EffectAllow {
		// A shadow match ("would deny (shadow): ...") or a non-blocking effect is
		// recorded as the filter's status message in the MCP audit log.
		output.Reason = decision.Reason
	}
	if decision.Effect == provenance.EffectRedact && !decision.Shadow {
		redacted, changed, err := redactToolArguments(message.Params)
		if err != nil {
			return nil, event.ID, fmt.Errorf("redact tool call arguments: %w", err)
		}
		if changed {
			mutated := message
			mutated.Params = redacted
			output.Message = &mutated
			output.Mutated = true
			output.Reason = "redacted by Obot Policy " + cmp.Or(decision.PolicyName, decision.PolicyID)
		}
	}
	return output, event.ID, nil
}

// ingestToolResult records a tool's result as content that entered the session.
func (p *mcpProvenance) ingestToolResult(ctx context.Context, result json.RawMessage, tool, callEventID string) error {
	caps := p.capabilities(ctx, tool)
	_, err := p.engine.Ingest(ctx, provenance.IngestRequest{
		Subject:      p.subject,
		Session:      p.session,
		Surface:      provenance.SurfaceMCP,
		Kind:         provenance.KindToolResult,
		ParentID:     callEventID,
		Tool:         tool,
		Server:       p.server,
		ServerID:     p.serverID,
		Capabilities: caps,
		Content:      toolResultContent(result),
		Summary:      p.server + "." + tool + " result",
		Bytes:        len(result),
	})
	if err != nil {
		return fmt.Errorf("record tool result: %w", err)
	}
	return nil
}

// ingestResource records a resource's contents as content that entered the session. A
// resource is the server's own data (reads-private) unless its URI is a web address, which
// anyone could have authored (reads-untrusted).
func (p *mcpProvenance) ingestResource(ctx context.Context, result json.RawMessage, uri, callEventID string) error {
	caps := provenance.NewLabels(provenance.CapReadsPrivate)
	var host string
	if u, err := url.Parse(uri); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		caps = provenance.NewLabels(provenance.CapReadsUntrusted)
		host = u.Hostname()
	}
	_, err := p.engine.Ingest(ctx, provenance.IngestRequest{
		Subject:      p.subject,
		Session:      p.session,
		Surface:      provenance.SurfaceMCP,
		Kind:         provenance.KindToolResult,
		ParentID:     callEventID,
		Tool:         "resources/read",
		Server:       p.server,
		ServerID:     p.serverID,
		Capabilities: caps,
		Host:         host,
		Content:      resourceContent(result),
		Summary:      p.server + " resource " + resourceSummary(uri),
		Bytes:        len(result),
	})
	if err != nil {
		return fmt.Errorf("record resource read: %w", err)
	}
	return nil
}

// cacheToolList remembers the tools' descriptions and labels them ahead of their first call.
func (p *mcpProvenance) cacheToolList(ctx context.Context, result json.RawMessage) {
	var list struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	if json.Unmarshal(result, &list) != nil {
		return
	}
	for _, tool := range list.Tools {
		if tool.Name == "" {
			continue
		}
		if p.descriptions != nil {
			p.descriptions.Store(p.descriptionKey(tool.Name), tool.Description)
		}
		p.engine.ToolCapabilities(ctx, p.server, tool.Name, tool.Description)
	}
}

func (p *mcpProvenance) capabilities(ctx context.Context, tool string) provenance.Labels {
	return p.engine.ToolCapabilities(ctx, p.server, tool, p.description(tool))
}

func (p *mcpProvenance) description(tool string) string {
	if p.descriptions == nil {
		return ""
	}
	if description, ok := p.descriptions.Load(p.descriptionKey(tool)); ok {
		return description.(string)
	}
	return ""
}

func (p *mcpProvenance) descriptionKey(tool string) string {
	return p.serverID + "\x00" + tool
}

// toolResultContent extracts the text a tools/call result carries: every text content
// item, embedded resource text and the structured content. It falls back to the raw result
// when the result has none of those. The text is capped at maxProvenanceContentBytes.
func toolResultContent(result json.RawMessage) string {
	var parsed struct {
		Content []struct {
			Text     string `json:"text"`
			Resource *struct {
				Text string `json:"text"`
			} `json:"resource"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if json.Unmarshal(result, &parsed) != nil {
		return capContent(string(result))
	}
	var b strings.Builder
	for _, item := range parsed.Content {
		if item.Text != "" {
			b.WriteString(item.Text)
			b.WriteByte('\n')
		}
		if item.Resource != nil && item.Resource.Text != "" {
			b.WriteString(item.Resource.Text)
			b.WriteByte('\n')
		}
		if b.Len() > maxProvenanceContentBytes {
			break
		}
	}
	if len(parsed.StructuredContent) > 0 && !bytes.Equal(parsed.StructuredContent, []byte("null")) {
		b.Write(parsed.StructuredContent)
	}
	if b.Len() == 0 {
		return capContent(string(result))
	}
	return capContent(b.String())
}

// resourceContent extracts the text contents of a resources/read result.
func resourceContent(result json.RawMessage) string {
	var parsed struct {
		Contents []struct {
			Text string `json:"text"`
		} `json:"contents"`
	}
	if json.Unmarshal(result, &parsed) != nil {
		return capContent(string(result))
	}
	var b strings.Builder
	for _, item := range parsed.Contents {
		if item.Text != "" {
			b.WriteString(item.Text)
			b.WriteByte('\n')
		}
		if b.Len() > maxProvenanceContentBytes {
			break
		}
	}
	if b.Len() == 0 {
		return capContent(string(result))
	}
	return capContent(b.String())
}

func capContent(s string) string {
	if len(s) > maxProvenanceContentBytes {
		return s[:maxProvenanceContentBytes]
	}
	return s
}

// resourceSummary renders a resource URI for an event summary without its query or
// credentials, which can carry content.
func resourceSummary(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" {
		return uri
	}
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return u.String()
}

// redactToolArguments replaces every sensitive identifier in the string values of a tool
// call's arguments with a placeholder, leaving the other params (_meta, name) as they are.
func redactToolArguments(params json.RawMessage) (json.RawMessage, bool, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(params, &object); err != nil {
		return nil, false, err
	}
	raw, ok := object["arguments"]
	if !ok || len(raw) == 0 {
		return params, false, nil
	}
	var arguments any
	if err := decodeJSONNumbers(raw, &arguments); err != nil {
		return nil, false, err
	}
	redacted, changed := redactValue(arguments)
	if !changed {
		return params, false, nil
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return nil, false, err
	}
	object["arguments"] = encoded
	out, err := json.Marshal(object)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func redactValue(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		return redactString(t)
	case map[string]any:
		changed := false
		for k, val := range t {
			redacted, ok := redactValue(val)
			if ok {
				t[k] = redacted
				changed = true
			}
		}
		return t, changed
	case []any:
		changed := false
		for i, val := range t {
			redacted, ok := redactValue(val)
			if ok {
				t[i] = redacted
				changed = true
			}
		}
		return t, changed
	}
	return v, false
}

// redactString replaces the classified spans of s with "[REDACTED:<label>]". Overlapping
// spans (one identifier matched by two patterns) are merged and labelled by the first.
func redactString(s string) (string, bool) {
	spans := provenance.Classify(s).Spans
	if len(spans) == 0 {
		return s, false
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End > spans[j].End
	})
	merged := []provenance.LabeledSpan{spans[0]}
	for _, sp := range spans[1:] {
		last := &merged[len(merged)-1]
		if sp.Start < last.End {
			last.End = max(last.End, sp.End)
			continue
		}
		merged = append(merged, sp)
	}
	var b strings.Builder
	last := 0
	for _, sp := range merged {
		b.WriteString(s[last:sp.Start])
		b.WriteString("[REDACTED:")
		b.WriteString(string(sp.Label))
		b.WriteByte(']')
		last = sp.End
	}
	b.WriteString(s[last:])
	return b.String(), true
}

// decodeJSONNumbers decodes JSON keeping numbers as json.Number, so re-encoding an
// argument object after redaction does not change its numbers.
func decodeJSONNumbers(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(v)
}
