package mcpgateway

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/mcp"
)

const (
	maxMCPProxyHookBodySize = 10 << 20
)

// pendingRequest is the request context needed when its JSON-RPC response
// arrives, potentially through another HTTP request or Obot replica.
type pendingRequest struct {
	message   mcp.Message
	name      string
	mutations map[string]mcp.HookMutation
}

// hookProcessor filters MCP messages for one proxied HTTP exchange.
type hookProcessor struct {
	ctx       context.Context
	runner    mcp.HookRunner
	hooks     mcp.Hooks
	servers   mcp.HookServerConfigs
	audit     *proxyAudit
	store     *hookCorrelationStore
	sessionID string
	request   *pendingRequest
	requestID string
	disabled  bool

	requestError    error
	requestResponse []byte
}

type gzipHookBody struct {
	*gzip.Reader
	source io.Closer
}

// filteredRequest contains the wire request plus the context needed to filter
// its eventual response.
type filteredRequest struct {
	body      []byte
	request   pendingRequest
	requestID string
	hooks     hookResult
}

// hookResult is the complete outcome of running the matching hook chain. The
// audit recorder consumes the same result, so hook execution has one result
// type instead of a second, nearly identical audit type.
type hookResult struct {
	message         mcp.Message
	mutations       map[string]mcp.HookMutation
	statuses        []hookStatus
	mutated         bool
	err             error
	originalBody    json.RawMessage
	mutatedBody     json.RawMessage
	responseChanged bool
}

type hookStatus struct {
	typeName, method, name, tool, status, message string
}

type hookBlockedError struct {
	direction string
	reasons   []string
}

type hookSSEBody struct {
	source      io.ReadCloser
	reader      *bufio.Reader
	hooks       *hookProcessor
	output      []byte
	terminalErr error
}

type hookSSELine struct {
	value, ending string
}

func newHookProcessor(req *http.Request, runner mcp.HookRunner, hooks mcp.Hooks, servers mcp.HookServerConfigs, audit *proxyAudit, store *hookCorrelationStore) (*hookProcessor, error) {
	processor := &hookProcessor{
		ctx:       req.Context(),
		runner:    runner,
		hooks:     hooks,
		servers:   servers,
		audit:     audit,
		store:     store,
		sessionID: mcpSessionID(req.Header, req.URL),
		disabled:  runner == nil || len(hooks) == 0,
	}
	if req.Method != http.MethodPost || req.Body == nil || (processor.disabled && processor.audit == nil) {
		return processor, nil
	}

	requestBody, decoded, err := decodeMCPHookBody(req.Body, req.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, fmt.Errorf("failed to decode MCP request for hooks: %w", err)
	}
	body, err := readMCPHookBody(requestBody)
	if err != nil {
		_ = requestBody.Close()
		return nil, fmt.Errorf("failed to read MCP request for hooks: %w", err)
	}
	if err := requestBody.Close(); err != nil {
		return nil, fmt.Errorf("failed to close MCP request before hooks: %w", err)
	}
	if decoded {
		clearMCPHookRequestHeaders(req)
	}
	setMCPRequestBody(req, body)

	var message mcp.Message
	if decodeMCPHookMessage(body, &message) != nil {
		return processor, nil
	}

	if message.Method == "" {
		// If there is no method on this message, it's a protocol response
		if !processor.disabled {
			body = processor.filterResponseMessage(body, hookOriginServer)
			clearMCPHookRequestHeaders(req)
			setMCPRequestBody(req, body)
		}
		processor.audit.recordClientResponse(body)
		return processor, nil
	}
	if processor.disabled {
		return processor, nil
	}

	filtered, err := processor.filterRequest(body, message, hookOriginClient)
	filtered.hooks.captureBody(body)
	processor.audit.recordRequestHooks(filtered.hooks)
	if filtered.hooks.err != nil {
		processor.requestError = fmt.Errorf("failed to call request hooks: %w", filtered.hooks.err)
		processor.requestResponse = filtered.body
		return processor, nil
	}
	if err != nil {
		return nil, err
	}
	if filtered.hooks.mutated {
		clearMCPHookRequestHeaders(req)
		setMCPRequestBody(req, filtered.body)
	}

	processor.requestID = filtered.requestID
	processor.request = &filtered.request
	if err := processor.store.save(processor.ctx, processor.sessionID, processor.requestID, hookOriginClient, *processor.request); err != nil {
		return nil, err
	}
	return processor, nil
}

func readMCPHookBody(reader io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: maxMCPProxyHookBodySize + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maxMCPProxyHookBodySize {
		return nil, fmt.Errorf("MCP message exceeds %d byte hook limit", maxMCPProxyHookBodySize)
	}
	return body, nil
}

func decodeMCPHookMessage(body []byte, message *mcp.Message) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(message); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("MCP body contains multiple JSON values")
		}
		return err
	}
	return nil
}

func (b *gzipHookBody) Close() error {
	return errors.Join(b.Reader.Close(), b.source.Close())
}

func decodeMCPHookBody(body io.ReadCloser, contentEncoding string) (io.ReadCloser, bool, error) {
	switch strings.ToLower(strings.TrimSpace(contentEncoding)) {
	case "", "identity":
		return body, false, nil
	case "gzip":
		reader, err := gzip.NewReader(body)
		if err != nil {
			_ = body.Close()
			return nil, false, err
		}
		return &gzipHookBody{Reader: reader, source: body}, true, nil
	default:
		_ = body.Close()
		return nil, false, fmt.Errorf("unsupported Content-Encoding %q", contentEncoding)
	}
}

func setMCPRequestBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func clearMCPHookRequestHeaders(req *http.Request) {
	req.Header.Del("Content-Encoding")
	req.Header.Del("Digest")
	req.Header.Del("Content-MD5")
	req.Trailer = nil
}

func (h *hookProcessor) blockedRequest() ([]byte, bool, error) {
	if h == nil || h.requestError == nil {
		return nil, false, nil
	}
	return h.requestResponse, true, h.requestError
}

func (h *hookProcessor) filterResponse(resp *http.Response) error {
	if h == nil || h.disabled {
		return nil
	}
	if resp.Body != nil {
		body, decoded, err := decodeMCPHookBody(resp.Body, resp.Header.Get("Content-Encoding"))
		if err != nil {
			return fmt.Errorf("failed to decode MCP response for hooks: %w", err)
		}
		resp.Body = body
		if decoded {
			resp.ContentLength = -1
			clearMCPHookResponseHeaders(resp.Header)
		}
	}
	if sessionID := resp.Header.Get(mcpSessionHeader); sessionID != "" {
		previousSessionID := h.sessionID
		h.sessionID = sessionID
		if h.request != nil && sessionID != previousSessionID {
			if err := h.store.save(h.ctx, h.sessionID, h.requestID, hookOriginClient, *h.request); err != nil {
				return err
			}
			h.store.delete(h.ctx, previousSessionID, h.requestID, hookOriginClient)
		}
	}

	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" {
		resp.Body = newHookSSEBody(resp.Body, h)
		resp.ContentLength = -1
		clearMCPHookResponseHeaders(resp.Header)
		return nil
	}
	if resp.Body == nil {
		return nil
	}

	body, err := readMCPHookBody(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read MCP response for hooks: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("failed to close MCP response before hooks: %w", err)
	}
	originalBody := body
	if len(body) > 0 {
		body = h.filterResponseMessage(body, hookOriginClient)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if !bytes.Equal(body, originalBody) {
		resp.ContentLength = int64(len(body))
		clearMCPHookResponseHeaders(resp.Header)
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	return nil
}

func clearMCPHookResponseHeaders(header http.Header) {
	header.Del("Content-Encoding")
	header.Del("Content-Length")
	header.Del("Digest")
	header.Del("Content-MD5")
	header.Del("ETag")
}

func (h *hookProcessor) filterResponseMessage(body []byte, origin hookOrigin) []byte {
	var wireMessage mcp.Message
	if decodeMCPHookMessage(body, &wireMessage) != nil || wireMessage.Method != "" {
		return body
	}

	request, ok, err := h.takePendingRequest(wireMessage.ID, origin)
	if err != nil {
		return mcpHookErrorResponse(wireMessage, "response", err)
	}
	if !ok {
		return body
	}

	hookMessage := wireMessage
	hookMessage.Method = request.message.Method
	hookMessage.HookMutations = cloneMCPHookMutations(request.mutations)
	result := h.run(hookMessage, request.message.Method, request.name, "response", request.mutations)
	result.captureBody(body)

	if result.err != nil {
		hookErr := result.err
		result.responseChanged = true
		result.err = fmt.Errorf("failed to call response hooks: %w", hookErr)
		body = mcpHookErrorResponse(wireMessage, "response", hookErr)
		h.audit.recordResponseHooks(mcp.MessageIDString(wireMessage.ID), result)
		return body
	}
	if !result.mutated && len(result.mutations) == 0 {
		h.audit.recordResponseHooks(mcp.MessageIDString(wireMessage.ID), result)
		return body
	}

	message := result.message
	message.JSONRPC = wireMessage.JSONRPC
	message.ID = wireMessage.ID
	message.Method = wireMessage.Method
	message.HookMutations = cloneMCPHookMutations(result.mutations)
	if err := addMCPHookMutationsMeta(&message); err != nil {
		result.responseChanged = true
		result.err = err
		body = mcpHookErrorResponse(wireMessage, "response", err)
		h.audit.recordResponseHooks(mcp.MessageIDString(wireMessage.ID), result)
		return body
	}

	filtered, err := json.Marshal(message)
	if err != nil {
		result.responseChanged = true
		result.err = fmt.Errorf("failed to marshal response mutated by MCP hook: %w", err)
		body = mcpHookErrorResponse(wireMessage, "response", result.err)
		h.audit.recordResponseHooks(mcp.MessageIDString(wireMessage.ID), result)
		return body
	}
	// Mutation metadata can change the wire response when only the request was
	// mutated. That does not mean a response hook mutated the response body.
	result.responseChanged = result.mutated
	h.audit.recordResponseHooks(mcp.MessageIDString(wireMessage.ID), result)
	return filtered
}

func (h *hookProcessor) filterRequestMessage(body []byte, wireMessage mcp.Message) []byte {
	filtered, err := h.filterRequest(body, wireMessage, hookOriginServer)
	filtered.hooks.captureBody(body)
	auditID := mcp.MessageIDString(wireMessage.ID)

	if filtered.hooks.err != nil {
		filtered.hooks.err = fmt.Errorf("failed to call request hooks: %w", filtered.hooks.err)
		h.audit.recordStreamRequestHooks(auditID, filtered.hooks)
		return filtered.body
	}
	if err != nil {
		filtered.hooks.err = err
		h.audit.recordStreamRequestHooks(auditID, filtered.hooks)
		return mcpHookErrorResponse(wireMessage, "request", err)
	}

	if err := h.store.save(h.ctx, h.sessionID, filtered.requestID, hookOriginServer, filtered.request); err != nil {
		filtered.hooks.err = err
		h.audit.recordStreamRequestHooks(auditID, filtered.hooks)
		return mcpHookErrorResponse(wireMessage, "request", err)
	}
	h.audit.recordStreamRequestHooks(auditID, filtered.hooks)
	return filtered.body
}

func (h *hookProcessor) filterRequest(body []byte, wireMessage mcp.Message, origin hookOrigin) (filteredRequest, error) {
	result := h.run(wireMessage, wireMessage.Method, mcpHookMessageName(wireMessage), "request", nil)
	if result.err != nil {
		return filteredRequest{body: mcpHookErrorResponse(wireMessage, "request", result.err), hooks: result}, nil
	}

	message := result.message
	if result.mutated {
		if origin == hookOriginServer {
			message.JSONRPC = wireMessage.JSONRPC
			message.ID = wireMessage.ID
			message.Method = wireMessage.Method
		}
		var err error
		body, err = json.Marshal(message)
		if err != nil {
			return filteredRequest{hooks: result}, fmt.Errorf("failed to marshal request mutated by MCP hook: %w", err)
		}
	}

	requestID, _ := mcpHookMessageID(message.ID)
	return filteredRequest{
		body: body,
		request: pendingRequest{
			message: message, name: mcpHookMessageName(message), mutations: cloneMCPHookMutations(result.mutations),
		},
		requestID: requestID,
		hooks:     result,
	}, nil
}

func (h *hookProcessor) takePendingRequest(wireID any, origin hookOrigin) (pendingRequest, bool, error) {
	requestID, ok := mcpHookMessageID(wireID)
	if !ok {
		return pendingRequest{}, false, nil
	}
	if origin == hookOriginClient && h.request != nil && requestID == h.requestID {
		request := *h.request
		h.request = nil
		h.store.delete(h.ctx, h.sessionID, requestID, origin)
		return request, true, nil
	}
	request, found, err := h.store.loadAndDelete(h.ctx, h.sessionID, requestID, origin)
	if err != nil || !found {
		return pendingRequest{}, found, err
	}
	return request, true, nil
}

func (e *hookBlockedError) Error() string {
	return fmt.Sprintf("MCP %s blocked by hook: %s", e.direction, strings.Join(e.reasons, "; "))
}

func (h *hookProcessor) run(message mcp.Message, method, name, direction string, priorMutations map[string]mcp.HookMutation) hookResult {
	if len(h.hooks) == 0 {
		return hookResult{message: message, mutations: cloneMCPHookMutations(priorMutations)}
	}

	var (
		errs       []error
		statuses   []hookStatus
		mutations  = cloneMCPHookMutations(priorMutations)
		mutated    bool
		current    = message
		rejections []string
	)
	message.HookMutations = cloneMCPHookMutations(mutations)
	hookResponse := mcp.SessionMessageHook{Accept: true, Message: &message}
	params := map[string]string{
		"name": name, "direction": direction, "callOnError": strconv.FormatBool(message.Error != nil), "method": method,
	}
	for _, hook := range h.hooks {
		if !hook.Matches(method, params) {
			continue
		}
		for _, target := range hook.Targets {
			output, hookErr := h.runner.RunHook(h.ctx, h.servers, hookResponse, target.Target)
			if hookErr != nil {
				errs = append(errs, fmt.Errorf("failed to run hook %s: %w", hook.Name, hookErr))
				continue
			}
			if output == nil {
				continue
			}
			response := *output

			if response.Mutated && target.MutateDisallowed {
				if response.Reason != "" {
					response.Reason += "; "
				}
				response.Reason += "mutation not allowed by hook configuration, implicit rejection"
				response.Accept = false
				response.Mutated = false
			}

			status := "ok"
			if !response.Accept {
				status = "rejected"
			} else if response.Mutated {
				status = "mutated"
			}
			statuses = append(statuses, hookStatus{
				typeName: direction, method: method, name: hook.Name, tool: target.Target, status: status, message: response.Reason,
			})

			if !response.Accept {
				reason := strings.TrimSpace(response.Reason)
				if reason == "" {
					reason = fmt.Sprintf("hook %q did not provide a reason", target.Target)
				}
				rejections = append(rejections, reason)
			}
			if response.Mutated && response.Message != nil {
				if string(response.Message.Result) == "null" {
					response.Message.Result = nil
				}
				if string(response.Message.Params) == "null" {
					response.Message.Params = nil
				}
				if mutations == nil {
					mutations = make(map[string]mcp.HookMutation)
				}
				mutation := mutations[direction]
				mutation.Mutated = true
				if response.Reason != "" {
					mutation.Reasons = append(mutation.Reasons, response.Reason)
				}
				mutations[direction] = mutation
				response.Message.HookMutations = cloneMCPHookMutations(mutations)
				current = *response.Message
				mutated = true
			} else {
				response.Message = &current
			}
			hookResponse = response
		}
	}

	if hookResponse.Message == nil {
		hookResponse.Message = &message
	}
	if len(rejections) > 0 {
		errs = append(errs, &hookBlockedError{direction: direction, reasons: rejections})
	}
	return hookResult{
		message: *hookResponse.Message, mutations: mutations, statuses: statuses, mutated: mutated, err: errors.Join(errs...),
	}
}

func (r *hookResult) captureBody(originalBody []byte) {
	r.originalBody = jsonBody(originalBody)
	if r.mutated {
		if body, err := json.Marshal(r.message); err == nil {
			r.mutatedBody = jsonBody(body)
		}
	}
}

func cloneMCPHookMutations(mutations map[string]mcp.HookMutation) map[string]mcp.HookMutation {
	if len(mutations) == 0 {
		return nil
	}
	result := make(map[string]mcp.HookMutation, len(mutations))
	for direction, mutation := range mutations {
		mutation.Reasons = slices.Clone(mutation.Reasons)
		result[direction] = mutation
	}
	return result
}

func mcpHookMessageID(id any) (string, bool) {
	if id == nil {
		return "", false
	}
	data, err := json.Marshal(id)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func mcpHookMessageName(message mcp.Message) string {
	var params struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}
	if json.Unmarshal(message.Params, &params) != nil {
		return ""
	}
	switch message.Method {
	case "tools/call", "prompts/get":
		return params.Name
	case "resources/read", "resources/subscribe", "resources/unsubscribe":
		return params.URI
	default:
		return ""
	}
}

func addMCPHookMutationsMeta(message *mcp.Message) error {
	if len(message.HookMutations) == 0 || len(message.Result) == 0 {
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal(message.Result, &result); err != nil {
		return fmt.Errorf("failed to unmarshal response result to add hook mutation metadata: %w", err)
	}
	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		meta = make(map[string]any)
	}
	meta[mcp.HookMutationsMetaKey] = message.HookMutations
	result["_meta"] = meta

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal response result with hook mutation metadata: %w", err)
	}
	message.Result = data
	return nil
}

func mcpHookErrorResponse(request mcp.Message, direction string, hookErr error) []byte {
	jsonRPC := request.JSONRPC
	if jsonRPC == "" {
		jsonRPC = "2.0"
	}
	rpcError := mcp.ErrRPCUnknown.WithMessage("failed to call %q hooks: %v", direction, hookErr)
	if blockedErr, ok := errors.AsType[*hookBlockedError](hookErr); ok {
		rpcError = mcp.NewRPCError(mcp.ErrRPCUnknown.Code, blockedErr.Error())
	}
	data, err := json.Marshal(mcp.Message{
		JSONRPC: jsonRPC,
		ID:      request.ID,
		Error:   rpcError,
	})
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32001,"message":"JSON RPC unknown error: hook rejected message"}}`)
	}
	return data
}

func newHookSSEBody(source io.ReadCloser, hooks *hookProcessor) *hookSSEBody {
	return &hookSSEBody{source: source, reader: bufio.NewReader(source), hooks: hooks}
}

func (b *hookSSEBody) Read(p []byte) (int, error) {
	for len(b.output) == 0 {
		if b.terminalErr != nil {
			return 0, b.terminalErr
		}
		rawEvent, lines, err := readMCPHookSSEEvent(b.reader)
		if len(rawEvent) > 0 {
			b.output = b.transformEvent(rawEvent, lines)
		}
		if err != nil {
			b.terminalErr = err
		}
	}

	n := copy(p, b.output)
	b.output = b.output[n:]
	return n, nil
}

func (b *hookSSEBody) Close() error {
	return b.source.Close()
}

func (b *hookSSEBody) transformEvent(rawEvent []byte, lines []hookSSELine) []byte {
	var event string
	var data []string
	for _, line := range lines {
		if value, ok := strings.CutPrefix(line.value, "event:"); ok {
			event = strings.TrimSpace(value)
		} else if value, ok := strings.CutPrefix(line.value, "data:"); ok {
			data = append(data, strings.TrimPrefix(value, " "))
		}
	}
	if len(data) == 0 {
		return rawEvent
	}

	joined := []byte(strings.Join(data, "\n"))
	if event == "endpoint" {
		if endpoint, err := url.Parse(string(joined)); err == nil {
			if sessionID := mcpSessionID(nil, endpoint); sessionID != "" {
				b.hooks.sessionID = sessionID
			}
		}
		return rawEvent
	}

	var message mcp.Message
	if decodeMCPHookMessage(joined, &message) != nil {
		return rawEvent
	}
	var filtered []byte
	if message.Method != "" {
		filtered = b.hooks.filterRequestMessage(joined, message)
	} else {
		filtered = b.hooks.filterResponseMessage(joined, hookOriginClient)
	}
	if bytes.Equal(filtered, joined) {
		return rawEvent
	}

	var output bytes.Buffer
	wroteData := false
	for _, line := range lines {
		if _, ok := strings.CutPrefix(line.value, "data:"); ok {
			if !wroteData {
				output.WriteString("data: ")
				output.Write(filtered)
				output.WriteString(line.ending)
				wroteData = true
			}
			continue
		}
		output.WriteString(line.value)
		output.WriteString(line.ending)
	}
	return output.Bytes()
}

func readMCPHookSSEEvent(reader *bufio.Reader) ([]byte, []hookSSELine, error) {
	var (
		raw   bytes.Buffer
		lines []hookSSELine
	)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			raw.WriteString(line)
			value := strings.TrimSuffix(line, "\n")
			ending := line[len(value):]
			if trimmed, ok := strings.CutSuffix(value, "\r"); ok {
				value = trimmed
				ending = "\r" + ending
			}
			lines = append(lines, hookSSELine{value: value, ending: ending})
			if value == "" {
				return raw.Bytes(), lines, err
			}
		}
		if err != nil {
			return raw.Bytes(), lines, err
		}
	}
}
