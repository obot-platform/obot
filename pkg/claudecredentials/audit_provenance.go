package claudecredentials

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	ext "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	// provenanceBodyLimit bounds how much of a request or response body is
	// scanned by Obot Policy. A request larger than this is decided on its
	// first provenanceBodyLimit bytes, before the rest is forwarded.
	provenanceBodyLimit = 256 << 10
	// provenanceBudget bounds a decision so it fits inside Envoy's 5s
	// ext_proc message timeout with room for the rest of the hop.
	provenanceBudget = 3 * time.Second
	// provenanceAgent is the actor reported for every Claude Environment.
	provenanceAgent = "claude_environment"
	// provenanceDeniedDetails is the Envoy response code detail for a denial.
	provenanceDeniedDetails = "obot_provenance_denied"
	anthropicAPIHost        = "api.anthropic.com"
)

// egressProvenance follows one intercepted request through Obot Policy: it
// decides the request once it has seen enough of it, and ingests what the
// response brought back into the sandbox's session.
type egressProvenance struct {
	engine  *provenance.Engine
	entry   *types.EgressAuditLog
	subject provenance.Subject
	session provenance.SessionKey
	action  provenance.Action
	// model reports model or Obot-internal traffic: decided on headers,
	// bodies never scanned, responses never ingested (the model is not a
	// source and the LLM gateway has its own surface).
	model    bool
	decided  bool
	eventID  string
	request  []byte
	response []byte
	// responseText reports that the response body is scannable text.
	responseText bool
	ingested     bool
}

// newEgressProvenance builds the action for an attributed request. It returns
// nil when no engine is configured.
func (p *AuditProcessor) newEgressProvenance(ctx context.Context, entry *types.EgressAuditLog) *egressProvenance {
	engine := p.Provider.Provenance
	if engine == nil {
		return nil
	}
	host := hostname(entry.Host)
	model := strings.EqualFold(host, anthropicAPIHost) || sameHost(p.Provider.ObotURL, host)
	writes := false
	switch entry.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		writes = true
	}
	external := !model && engine.IsExternal(ctx, host)
	var caps provenance.Labels
	if writes {
		if external {
			caps = caps.Add(provenance.CapWritesExternal)
		} else {
			caps = caps.Add(provenance.CapWritesInternal)
		}
	}
	if !model {
		if engine.IsTrustedHost(ctx, host) {
			caps = caps.Add(provenance.CapReadsPrivate)
		} else {
			caps = caps.Add(provenance.CapReadsUntrusted)
		}
	}
	actor := provenance.Actor{Agent: provenanceAgent, EnvironmentID: entry.EnvironmentID, ClientSessionID: entry.ClaudeSession}
	return &egressProvenance{
		engine:  engine,
		entry:   entry,
		subject: provenance.Subject{UserID: entry.UserID, Actor: actor},
		session: provenance.SessionKey{ClientSessionID: entry.ClaudeSession, Agent: provenanceAgent, EnvironmentID: entry.EnvironmentID},
		action: provenance.Action{
			Surface:      provenance.SurfaceEgress,
			Kind:         provenance.KindNetEgress,
			Host:         host,
			Method:       entry.Method,
			Path:         entry.Path,
			Capabilities: caps,
			Sink:         provenance.Sink{External: external, Destination: host, Writes: writes},
			Source:       provenance.SourceFromCapabilities(caps),
		},
		model: model,
	}
}

// decideOnHeaders reports whether the request can be decided before its body:
// there is no body, Envoy will not stream one (upgrades), or the body is
// model traffic that is never scanned.
func (e *egressProvenance) decideOnHeaders(headers map[string][]string, endOfStream, upgrade bool) bool {
	if e.model || endOfStream || upgrade {
		return true
	}
	switch e.entry.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	}
	return firstHeader(headers, "content-length") == "0"
}

// addRequest buffers a request body chunk and reports whether the request is
// ready to decide: the body ended or the scan limit was reached.
func (e *egressProvenance) addRequest(chunk []byte, endOfStream bool) bool {
	if e.decided || e.model {
		return false
	}
	e.request = append(e.request, chunk[:min(len(chunk), provenanceBodyLimit-len(e.request))]...)
	return endOfStream || len(e.request) >= provenanceBodyLimit
}

// decide runs Obot Policy once for the request. It returns the denial
// response to send, or nil to continue. Engine failures allow the request and
// are logged: the audit record is still written, and the egress gateway's
// own policy still applies.
func (e *egressProvenance) decide(ctx context.Context) *ext.ProcessingResponse {
	if e.decided {
		return nil
	}
	e.decided = true
	action := e.action
	action.Text = string(e.request)
	e.request = nil
	ev, decision, err := e.engine.Decide(ctx, provenance.DecideRequest{
		Subject: e.subject,
		Session: e.session,
		Action:  action,
		Ref:     provenance.SurfaceRef{EgressAuditLogUUID: e.entry.ID},
		Budget:  provenanceBudget,
	})
	if err != nil {
		slog.WarnContext(ctx, "Obot Policy could not decide a Claude egress request", "host", e.action.Host, "error", err)
		return nil
	}
	e.eventID = ev.ID
	if decision.Allow {
		return nil
	}
	e.entry.ResponseStatus = http.StatusForbidden
	e.entry.Outcome = "denied"
	body := "Blocked by Obot Policy: " + decision.Reason
	if decision.ApprovalID != "" {
		body += " Approval " + decision.ApprovalID + " is pending."
	}
	return &ext.ProcessingResponse{
		Response: &ext.ProcessingResponse_ImmediateResponse{ImmediateResponse: &ext.ImmediateResponse{
			Status:  &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
			Body:    []byte(body),
			Details: provenanceDeniedDetails,
		}},
	}
}

// responseHeaders records whether the response body will be scannable.
func (e *egressProvenance) responseHeaders(headers map[string][]string) {
	encoding := strings.ToLower(firstHeader(headers, "content-encoding"))
	e.responseText = (encoding == "" || encoding == "identity") && textualContent(firstHeader(headers, "content-type"))
}

// addResponse buffers a response body chunk.
func (e *egressProvenance) addResponse(chunk []byte) {
	if e.model || e.ingested {
		return
	}
	e.response = append(e.response, chunk[:min(len(chunk), provenanceBodyLimit-len(e.response))]...)
}

// ingest feeds the response into the session as a tool result: content from
// an untrusted host taints the session; content from a trusted host is
// first-party and private. Model traffic is skipped.
func (e *egressProvenance) ingest(ctx context.Context) {
	if e.model || e.ingested || len(e.response) == 0 {
		return
	}
	e.ingested = true
	content := ""
	if e.responseText {
		content = string(e.response)
	}
	e.response = nil
	_, err := e.engine.Ingest(ctx, provenance.IngestRequest{
		Subject:      e.subject,
		Session:      e.session,
		Surface:      provenance.SurfaceEgress,
		Kind:         provenance.KindToolResult,
		ParentID:     e.eventID,
		Host:         e.action.Host,
		Capabilities: e.action.Capabilities,
		Content:      content,
		Ref:          provenance.SurfaceRef{EgressAuditLogUUID: e.entry.ID},
		Summary:      strings.TrimSpace(e.action.Method + " " + e.action.Host + e.action.Path),
		Bytes:        int(e.entry.ResponseBytes),
	})
	if err != nil {
		slog.WarnContext(ctx, "Obot Policy could not ingest a Claude egress response", "host", e.action.Host, "error", err)
	}
}

// hostname strips the port and trailing dot from an :authority value.
func hostname(authority string) string {
	parsed, err := url.Parse("https://" + authority)
	if err != nil || parsed.Hostname() == "" {
		host, _, _ := strings.Cut(authority, ":")
		return strings.ToLower(strings.TrimSuffix(host, "."))
	}
	return strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
}

// sameHost reports whether host is the host Obot itself is reached at.
func sameHost(obotURL, host string) bool {
	obot, err := url.Parse(obotURL)
	if err != nil || obot.Hostname() == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(obot.Hostname(), "."), host)
}

// textualContent reports whether a content type is worth scanning as text.
// Unknown types are scanned; binary media are not.
func textualContent(contentType string) bool {
	mediaType, _, _ := strings.Cut(strings.ToLower(contentType), ";")
	mediaType = strings.TrimSpace(mediaType)
	switch {
	case mediaType == "":
		return true
	case strings.HasPrefix(mediaType, "text/"):
		return true
	case strings.HasPrefix(mediaType, "image/"), strings.HasPrefix(mediaType, "audio/"), strings.HasPrefix(mediaType, "video/"), strings.HasPrefix(mediaType, "font/"):
		return false
	case strings.HasSuffix(mediaType, "+json"), strings.HasSuffix(mediaType, "+xml"):
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/javascript", "application/x-www-form-urlencoded",
		"application/x-ndjson", "application/graphql", "application/yaml", "application/x-yaml", "application/toml",
		"application/sql", "application/x-sh", "application/x-httpd-php", "application/ld+json", "message/rfc822":
		return true
	case "application/octet-stream", "application/zip", "application/gzip", "application/x-gzip", "application/x-tar",
		"application/pdf", "application/x-bzip2", "application/x-xz", "application/zstd", "application/wasm",
		"application/x-7z-compressed", "application/vnd.ms-excel", "application/msword", "application/x-protobuf", "application/grpc":
		return false
	}
	return !strings.HasPrefix(mediaType, "application/vnd.")
}
