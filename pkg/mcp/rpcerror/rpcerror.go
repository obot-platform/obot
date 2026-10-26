// Package rpcerror answers MCP clients' calls with JSON-RPC errors. Clients show an error's message to their user.
package rpcerror

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

const (
	// maxCallBytes is the largest call read to answer it.
	maxCallBytes = 1 << 20
)

// Answer returns the JSON-RPC error that answers the call req posts with message. It reads req's body, and reports
// false when there's no call to answer, like for a notification or a body it can't read.
func Answer(req *http.Request, message string) ([]byte, bool) {
	if req.Method != http.MethodPost || req.Body == nil {
		return nil, false
	}

	var body io.Reader = req.Body
	switch strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		decoded, err := gzip.NewReader(req.Body)
		if err != nil {
			return nil, false
		}
		defer decoded.Close()
		body = decoded
	default:
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(body, maxCallBytes+1))
	if err != nil || len(data) > maxCallBytes {
		return nil, false
	}

	msg, err := jsonrpc.DecodeMessage(data)
	if err != nil {
		return nil, false
	}
	call, ok := msg.(*jsonrpc.Request)
	if !ok || call == nil || !call.IsCall() {
		return nil, false
	}
	response, err := jsonrpc.EncodeMessage(&jsonrpc.Response{
		ID: call.ID,
		Error: &jsonrpc.Error{
			Code:    jsonrpc.CodeInternalError,
			Message: message,
		},
	})
	if err != nil {
		return nil, false
	}
	return response, true
}
