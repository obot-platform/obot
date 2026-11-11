package mcpgateway

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	obotmcp "github.com/obot-platform/obot/pkg/mcp"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/auditlogs"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// openAPIAuditBodyLimit bounds the request/response excerpts written into MCP
// audit logs for OpenAPI worker calls. The contract caps audit bodies at
// 32 KiB with an explicit truncation marker.
const openAPIAuditBodyLimit = 32 * 1024

// recordOpenAPIBridgeAudit emits one MCP audit entry per OpenAPI tools/call.
// It mirrors the proxy audit semantics: identity comes from authenticated
// request context and server metadata, never client input; bodies are bounded
// and marked when truncated; credentials never appear in the entry.
func (h *Handler) recordOpenAPIBridgeAudit(req api.Context, server v1.MCPServer, config mcp.ServerConfig, request jsonRPCRequest, started time.Time, result map[string]any, callErr error) {
	if h.auditLogCollector == nil {
		println("BRIDGE-AUDIT: collector nil")
		return
	}

	entry := auditlogs.MCPAuditLog{
		Metadata:       bridgeAuditMetadata(server, config),
		CreatedAt:      started,
		Subject:        req.User.GetUID(),
		ClientIP:       clientIPForAudit(req),
		UserAgent:      req.Request.UserAgent(),
		CallType:       "tools/call",
		RequestID:      jsonIDString(request.ID),
		SessionID:      req.Request.Header.Get(mcpSessionHeader),
		RequestHeaders: sanitizedRequestHeaders(req),
	}
	if auth := strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "); auth != "" && strings.Count(auth, ".") != 2 {
		entry.APIKey = auditlogs.RedactAPIKey(auth)
	}

	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(request.Params, &params); err == nil && params.Name != "" {
		entry.CallIdentifier = params.Name
		entry.RequestBody = boundedAuditJSON(json.RawMessage(request.Params), openAPIAuditBodyLimit)
	}

	if callErr != nil {
		entry.Error = boundedAuditString(callErr.Error(), 256)
		entry.ResponseStatus = 0
	} else {
		entry.ResponseStatus = openAPIAuditStatus(result)
		if body := openAPIAuditResponseBody(result); body != nil {
			entry.ResponseBody = boundedAuditJSON(body, openAPIAuditBodyLimit)
		}
	}
	entry.SetProcessingTime()
	h.auditLogCollector.CollectMCPAuditEntry(entry)
}

// bridgeAuditMetadata identifies the component server being executed. The
// vMCP-level proxy audit deliberately skips component entries via the ignore
// flag; this bridge entry drops that flag and records the business execution
// (operation, business status, bounded bodies) the aggregate layer cannot see.
func bridgeAuditMetadata(server v1.MCPServer, config mcp.ServerConfig) map[string]string {
	metadata := make(map[string]string, len(config.AuditLogMetadata)+3)
	for key, value := range config.AuditLogMetadata {
		metadata[key] = value
	}
	delete(metadata, obotmcp.AuditLogIgnore)
	if metadata["mcpID"] == "" && server.Spec.VMCPID != "" {
		metadata["mcpID"] = server.Spec.VMCPID
	}
	if metadata["mcpServerDisplayName"] == "" {
		metadata["mcpServerDisplayName"] = server.Spec.Manifest.Name
	}
	if metadata["userID"] == "" {
		metadata["userID"] = server.Spec.UserID
	}
	metadata["vmcpComponentID"] = server.Spec.VMCPComponentID
	return metadata
}

func jsonIDString(id json.RawMessage) string {
	return strings.Trim(string(id), `"`)
}

func clientIPForAudit(req api.Context) string {
	if forwarded := req.Request.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host := req.Request.RemoteAddr
	if index := strings.LastIndex(host, ":"); index > 0 {
		host = host[:index]
	}
	return strings.TrimSpace(host)
}

func sanitizedRequestHeaders(req api.Context) json.RawMessage {
	headers, _ := json.Marshal(sanitizedMCPHeaders(req.Request.Header))
	return headers
}

func openAPIAuditStatus(result map[string]any) int {
	if result == nil {
		return 0
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured == nil {
		return 0
	}
	switch status := structured["status"].(type) {
	case int:
		return status
	case float64:
		return int(status)
	default:
		return 0
	}
}

func openAPIAuditResponseBody(result map[string]any) json.RawMessage {
	if result == nil {
		return nil
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured == nil {
		return nil
	}
	encoded, ok := structured["bodyBase64"].(string)
	if !ok || encoded == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return nil
	}
	if len(decoded) > openAPIAuditBodyLimit {
		decoded = decoded[:openAPIAuditBodyLimit]
	}
	if json.Valid(decoded) {
		return boundedAuditJSON(decoded, openAPIAuditBodyLimit)
	}
	return nil
}

// boundedAuditJSON truncates a JSON excerpt at limit bytes and marks it, so a
// truncated entry is never mistaken for the complete business object.
func boundedAuditJSON(raw json.RawMessage, limit int) json.RawMessage {
	if len(raw) <= limit {
		return raw
	}
	truncationMarker, _ := json.Marshal(map[string]any{
		"truncated":     true,
		"originalBytes": len(raw),
	})
	return truncationMarker
}

func boundedAuditString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…(truncated)"
}
