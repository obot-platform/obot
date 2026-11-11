package mcpgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/openapiworker"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const maxOpenAPIBridgeJSONBytes = 1 << 20

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (h *Handler) handleOpenAPIWorkerBridge(req api.Context, server v1.MCPServer, config mcp.ServerConfig) (bool, error) {
	if h.openAPIWorkerExecutor == nil || config.Runtime != types.RuntimeOpenAPI {
		return false, nil
	}
	reference, err := openAPIWorkerReference(req, server, config)
	if err != nil {
		writeOpenAPIBridgeError(req, nil, err)
		return true, nil
	}
	if req.Method != http.MethodPost {
		writeOpenAPIBridgeError(req, nil, openapiworker.NewError(openapiworker.ErrorCodeInvalidRequest, "OpenAPI MCP bridge accepts only JSON POST requests"))
		return true, nil
	}
	if !acceptsJSON(req.Request.Header.Get("Accept")) {
		writeOpenAPIBridgeError(req, nil, openapiworker.NewError(openapiworker.ErrorCodeUpstreamProtocolUnsupported, "OpenAPI MCP bridge requires JSON"))
		return true, nil
	}
	body, err := io.ReadAll(io.LimitReader(req.Request.Body, maxOpenAPIBridgeJSONBytes+1))
	if err != nil || len(body) > maxOpenAPIBridgeJSONBytes {
		writeOpenAPIBridgeError(req, nil, openapiworker.NewError(openapiworker.ErrorCodeRequestTooLarge, "OpenAPI MCP request exceeds 1 MiB"))
		return true, nil
	}
	var request jsonRPCRequest
	if err := json.Unmarshal(body, &request); err != nil || request.JSONRPC != "2.0" || request.Method == "" {
		writeOpenAPIBridgeError(req, nil, openapiworker.NewError(openapiworker.ErrorCodeInvalidRequest, "invalid JSON-RPC request"))
		return true, nil
	}
	if len(request.ID) == 0 {
		req.WriteHeader(http.StatusAccepted)
		return true, nil
	}

	principalContext, principal, err := openapiworker.NewPrincipalContext(req.Context(), req.User)
	if err != nil {
		writeOpenAPIBridgeError(req, request.ID, err)
		return true, nil
	}
	switch request.Method {
	case "initialize":
		writeOpenAPIBridgeResult(req, request.ID, map[string]any{
			"protocolVersion": requestedProtocolVersion(request.Params),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "Obot OpenAPI Worker Bridge",
				"version": "1.0",
			},
		})
	case "tools/list":
		description, err := h.openAPIWorkerExecutor.Describe(principalContext, principal, reference)
		if err != nil {
			writeOpenAPIBridgeError(req, request.ID, err)
			return true, nil
		}
		tools := make([]map[string]any, 0, len(description.Operations))
		for _, operation := range description.Operations {
			tools = append(tools, map[string]any{
				"name":        openAPIToolName(operation),
				"description": operation.Description,
				"inputSchema": json.RawMessage(operation.InputSchema),
				"_meta": map[string]string{
					"operationKey":   operation.OperationKey,
					"legacyToolName": operation.LegacyToolName,
				},
			})
		}
		writeOpenAPIBridgeResult(req, request.ID, map[string]any{"tools": tools})
	case "tools/call":
		started := time.Now()
		result, err := h.callOpenAPIWorker(principal, principalContext, reference, request.Params)
		h.recordOpenAPIBridgeAudit(req, server, config, request, started, result, err)
		if err != nil {
			writeOpenAPIBridgeError(req, request.ID, err)
			return true, nil
		}
		writeOpenAPIBridgeResult(req, request.ID, result)
	default:
		writeOpenAPIBridgeError(req, request.ID, openapiworker.NewError(openapiworker.ErrorCodeInvalidRequest, "unsupported JSON-RPC method"))
	}
	return true, nil
}

func openAPIWorkerReference(req api.Context, server v1.MCPServer, config mcp.ServerConfig) (openapiworker.Reference, error) {
	vmcpID := server.Spec.VMCPID
	if vmcpID == "" && server.Spec.VMCPInstanceID != "" {
		var instance v1.VMCPInstance
		if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: server.Spec.VMCPInstanceID}, &instance); err != nil {
			return openapiworker.Reference{}, openapiworker.NewError(openapiworker.ErrorCodeResourceHidden, "OpenAPI component is unavailable")
		}
		vmcpID = instance.Spec.Manifest.VMCPID
	}
	instanceName := config.MCPServerInstanceID
	if instanceName == "" {
		instanceName = server.Name
	}
	if vmcpID == "" || server.Spec.VMCPComponentID == "" || instanceName == "" {
		return openapiworker.Reference{}, openapiworker.NewError(openapiworker.ErrorCodeResourceHidden, "OpenAPI runtime requires a VMCP component reference")
	}
	return openapiworker.Reference{
		VMCPID:       vmcpID,
		ComponentID:  server.Spec.VMCPComponentID,
		InstanceName: instanceName,
	}, nil
}

func (h *Handler) callOpenAPIWorker(principal openapiworker.PrincipalContext, principalContext context.Context, reference openapiworker.Reference, params json.RawMessage) (map[string]any, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(params, &call); err != nil || call.Name == "" {
		return nil, openapiworker.NewError(openapiworker.ErrorCodeInvalidRequest, "invalid tools/call parameters")
	}
	description, err := h.openAPIWorkerExecutor.Describe(principalContext, principal, reference)
	if err != nil {
		return nil, err
	}
	var operationKey string
	for _, operation := range description.Operations {
		if call.Name == openAPIToolName(operation) || call.Name == operation.LegacyToolName || call.Name == operation.DisplayName {
			if operationKey != "" {
				return nil, openapiworker.NewError(openapiworker.ErrorCodeAmbiguousOperation, "tool name matches multiple operations")
			}
			operationKey = operation.OperationKey
		}
	}
	if operationKey == "" {
		return nil, openapiworker.NewError(openapiworker.ErrorCodeOperationNotFound, "tool is not authorized")
	}
	result, err := h.openAPIWorkerExecutor.Execute(principalContext, principal, openapiworker.Invocation{
		Reference:    reference,
		OperationKey: operationKey,
		InputKind:    openapiworker.InputKindTool,
		ToolInput:    call.Arguments,
		Deadline:     time.Now().Add(30 * time.Second),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]string{{
			"type": "text",
			"text": fmt.Sprintf("HTTP %d", result.Status),
		}},
		"structuredContent": map[string]any{
			"status":     result.Status,
			"headers":    result.Headers,
			"bodyBase64": result.BodyBase64,
		},
	}, nil
}

func openAPIToolName(operation openapiworker.OperationDescription) string {
	candidate := operation.LegacyToolName
	if candidate == "" {
		candidate = operation.DisplayName
	}
	if candidate == "" {
		candidate = operation.OperationKey
	}

	var result strings.Builder
	lastInvalid := false
	for _, char := range candidate {
		valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' || char == '.'
		if valid {
			result.WriteRune(char)
			lastInvalid = false
			continue
		}
		if result.Len() > 0 && !lastInvalid {
			result.WriteByte('_')
			lastInvalid = true
		}
	}
	return strings.Trim(result.String(), "_")
}

func requestedProtocolVersion(params json.RawMessage) string {
	var decoded struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(params, &decoded) == nil && decoded.ProtocolVersion != "" {
		return decoded.ProtocolVersion
	}
	return "2025-06-18"
}

func acceptsJSON(accept string) bool {
	for _, value := range strings.Split(accept, ",") {
		mediaType, _, _ := strings.Cut(value, ";")
		if strings.TrimSpace(mediaType) == "application/json" {
			return true
		}
	}
	return false
}

func writeOpenAPIBridgeResult(req api.Context, id json.RawMessage, result any) {
	encoded, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	if err != nil {
		writeOpenAPIBridgeError(req, id, openapiworker.NewError(openapiworker.ErrorCodeProtocolError, "cannot encode OpenAPI MCP response"))
		return
	}
	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	req.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = req.ResponseWriter.Write(encoded)
}

func writeOpenAPIBridgeError(req api.Context, id json.RawMessage, err error) {
	code := -32000
	message := "OpenAPI MCP bridge failed"
	if workerErr, ok := openapiworker.AsError(err); ok {
		message = workerErr.Error()
	}
	encoded, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	})
	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	req.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = req.ResponseWriter.Write(encoded)
}
