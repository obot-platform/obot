package mcpgateway

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strconv"
	"sync"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
)

var (
	// unavailableBuiltin serves a built-in server that isn't registered: an MCP server with no tools.
	unavailableBuiltin = func() BuiltinServer {
		empty := gomcp.NewServer(&gomcp.Implementation{Name: "Unavailable built-in MCP server", Version: "0"}, nil)
		handler := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return empty }, &gomcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			// Like every built-in server, it's only called in process; see adminmcp.NewHandler.
			DisableLocalhostProtection: true,
		})
		return func(api.Context) http.Handler { return handler }
	}()
)

// BuiltinServer returns the handler of an MCP server built into Obot, acting for req's user.
type BuiltinServer func(req api.Context) http.Handler

// builtinTransport serves requests with a built-in MCP server's handler instead of the network.
type builtinTransport struct {
	// req is the component request, which carries the user and Obot's storage and clients.
	req    api.Context
	server BuiltinServer
}

// streamingResponse is an http.ResponseWriter whose headers are delivered once written and whose
// body is streamed through a pipe.
type streamingResponse struct {
	header     http.Header
	sentHeader http.Header
	status     int
	once       sync.Once
	ready      chan struct{}
	body       *io.PipeWriter
}

// cancelingBody cancels the handler's request when the caller closes the response body.
type cancelingBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

// RegisterBuiltin makes a built-in MCP server available to vMCPs under name. It must be called
// before the gateway serves requests.
func (h *Handler) RegisterBuiltin(name string, server BuiltinServer) {
	if h.builtins == nil {
		h.builtins = map[string]BuiltinServer{}
	}
	h.builtins[name] = server
}

// builtinClient returns a client that serves a built-in MCP server in this process, and the URL to
// address it by. Built-in servers are only reachable as vMCP components: the request must be a
// vMCP's own loopback, whose token names the user who connected to the vMCP.
func (h *Handler) builtinClient(req api.Context, serverConfig mcp.ServerConfig) (*http.Client, string, error) {
	if !slices.Contains(req.User.GetGroups(), types.GroupCompositeMCP) {
		return nil, "", types.NewErrHTTP(http.StatusForbidden, "built-in MCP servers can only be used through a vMCP")
	}
	server, ok := h.builtins[serverConfig.BuiltinName]
	if !ok {
		// The server is turned off, for example by --disable-admin-mcp, but vMCPs that include it keep
		// their component. An empty server keeps those vMCPs working with their other components.
		server = unavailableBuiltin
	}
	return &http.Client{Transport: builtinTransport{req: req, server: server}}, "http://builtin.obot.internal/" + serverConfig.BuiltinName, nil
}

// RoundTrip runs the handler in its own goroutine and returns the response as soon as the handler
// writes its headers, streaming the body. MCP responses can stay open indefinitely, for example a
// subscriptions/listen stream, so waiting for the handler to finish would never return.
func (t builtinTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	bodyReader, bodyWriter := io.Pipe()
	response := &streamingResponse{header: http.Header{}, ready: make(chan struct{}), body: bodyWriter}

	inner := t.req
	inner.Request = r
	inner.ResponseWriter = response
	go func() {
		defer func() {
			// A handler that returns without writing still answers 200 with an empty body.
			response.WriteHeader(http.StatusOK)
			_ = bodyWriter.Close()
		}()
		t.server(inner).ServeHTTP(response, r)
	}()

	select {
	case <-response.ready:
	case <-ctx.Done():
		cancel()
		// Nothing will read the body, so make the handler's writes fail instead of blocking forever.
		_ = bodyReader.CloseWithError(ctx.Err())
		return nil, ctx.Err()
	}
	return &http.Response{
		Status:        strconv.Itoa(response.status) + " " + http.StatusText(response.status),
		StatusCode:    response.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        response.sentHeader,
		Body:          cancelingBody{ReadCloser: bodyReader, cancel: cancel},
		ContentLength: -1,
		Request:       r,
	}, nil
}

func (s *streamingResponse) Header() http.Header {
	return s.header
}

// WriteHeader delivers the status and headers. Like a real response, headers changed afterwards aren't sent.
func (s *streamingResponse) WriteHeader(status int) {
	s.once.Do(func() {
		s.status = status
		s.sentHeader = s.header.Clone()
		close(s.ready)
	})
}

func (s *streamingResponse) Write(data []byte) (int, error) {
	s.WriteHeader(http.StatusOK)
	return s.body.Write(data)
}

// Flush delivers the headers. The body is a pipe, so what was written has already been delivered.
func (s *streamingResponse) Flush() {
	s.WriteHeader(http.StatusOK)
}

func (c cancelingBody) Close() error {
	c.cancel()
	return c.ReadCloser.Close()
}
