package mcpserver

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	omcp "github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/listing"
	"github.com/obot-platform/obot/pkg/storage"
)

// Server is the integrated MCP server that exposes Obot MCP server discovery tools.
type Server struct {
	mcpServer         *mcp.Server
	gatewayClient     *gclient.Client
	storageClient     storage.Client
	lister            *listing.Lister
	serverURL         string
	internalServerURL string
}

// NewServer creates a new integrated MCP server.
func NewServer(gatewayClient *gclient.Client, storageClient storage.Client, lister *listing.Lister, serverURL string, sessionManager *omcp.SessionManager) *Server {
	s := &Server{
		gatewayClient:     gatewayClient,
		storageClient:     storageClient,
		lister:            lister,
		serverURL:         serverURL,
		internalServerURL: sessionManager.TransformObotHostname(serverURL),
	}

	// Create the MCP server with implementation info
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "obot-mcp-server",
		Version: "0.1.0",
	}, &mcp.ServerOptions{})

	s.mcpServer = mcpServer

	// Register tools
	s.registerTools()

	return s
}

// Handler returns an http.Handler for the MCP server with authentication middleware.
func (s *Server) Handler() http.Handler {
	// Create the streamable HTTP handler in stateless mode.
	// Stateless mode is used because the server does not need to maintain session state
	// between requests, and it allows the server to be horizontally scaled.
	httpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})

	// Wrap with authentication middleware
	return checkInternalRequestMiddleware(s.authMiddleware(httpHandler))
}
