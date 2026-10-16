package mcpgateway

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/guardrails"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	componenthttp "github.com/obot-platform/mmmcp/component/http"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tunnel"
	"github.com/obot-platform/obot/pkg/version"
	"golang.org/x/oauth2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	maxJSONRPCErrorRequestBody = 1 << 20
)

var (
	errMCPServerRequiresConfiguration = errors.New("mcp server requires configuration")
)

type Handler struct {
	ctx                       context.Context
	cancel                    func()
	mcpSessionManager         *mcp.SessionManager
	globalTokenStore          mcp.GlobalTokenStore
	tokenService              *persistent.TokenService
	auditLogCollector         proxyAuditCollector
	composite                 *mmmcp.Composite
	hookRunner                mcp.HookRunner
	guardrails                *guardrails.Engine
	tunnelManager             *tunnel.Manager
	secretBindingAllowedLabel string
	serverURL                 string
}

// mcpResponse keeps the client response and its audit representation together.
type mcpResponse struct {
	status  int
	headers http.Header
	body    []byte
}

func (r *mcpResponse) write(w http.ResponseWriter, audit *proxyAudit) {
	audit.recordRequest()
	for key, values := range r.headers {
		w.Header().Del(key)
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body)
	if audit != nil {
		audit.entry.ResponseHeaders, _ = json.Marshal(sanitizedMCPHeaders(w.Header()))
	}
	audit.recordHTTPResponse(r.body, r.status, nil)
}

func mcpTextResponse(status int, message string) *mcpResponse {
	return &mcpResponse{
		status: status,
		headers: http.Header{
			"Content-Length":         nil,
			"Content-Type":           {"text/plain; charset=utf-8"},
			"X-Content-Type-Options": {"nosniff"},
		},
		body: []byte(message + "\n"),
	}
}

func mcpJSONResponse(body []byte) *mcpResponse {
	return &mcpResponse{
		status:  http.StatusOK,
		headers: http.Header{"Content-Type": {"application/json"}},
		body:    body,
	}
}

func mcpJSONRPCErrorResponse(req *http.Request, rpcErr error) *mcpResponse {
	if req.Method != http.MethodPost || req.Body == nil {
		return nil
	}

	decoded, _, err := decodeMCPHookBody(req.Body, req.Header.Get("Content-Encoding"))
	if err != nil {
		return nil
	}

	defer decoded.Close()

	body, err := io.ReadAll(io.LimitReader(decoded, maxJSONRPCErrorRequestBody+1))
	if err != nil || len(body) > maxJSONRPCErrorRequestBody {
		return nil
	}

	msg, err := jsonrpc.DecodeMessage(body)
	if err != nil {
		return nil
	}

	call, ok := msg.(*jsonrpc.Request)
	if !ok || call == nil || !call.IsCall() {
		return nil
	}

	response, err := jsonrpc.EncodeMessage(&jsonrpc.Response{
		ID: call.ID,
		Error: &jsonrpc.Error{
			Code:    jsonrpc.CodeInternalError,
			Message: rpcErr.Error(),
		},
	})
	if err != nil {
		return nil
	}

	return mcpJSONResponse(response)
}

// A nil response leaves error handling to the caller, for example for notifications
// or requests that cannot be decoded as a JSON-RPC call.
func genericToolCallErrorResponse(req api.Context, callErr error) *mcpResponse {
	if errors.Is(callErr, errMCPBatchUnsupported) || errors.Is(callErr, errMCPCaseVariantFieldNames) {
		return mcpTextResponse(http.StatusBadRequest, callErr.Error())
	}
	if failed, ok := errors.AsType[*failedGenericToolCall](callErr); ok {
		result, err := json.Marshal(failed.result)
		if err != nil {
			return nil
		}
		body, err := json.Marshal(mcp.Message{JSONRPC: "2.0", ID: failed.id, Result: result})
		if err != nil {
			return nil
		}
		return mcpJSONResponse(body)
	}
	if authErr, ok := errors.AsType[*mmmcp.AuthorizationError](callErr); ok && authErr.StatusCode == http.StatusUnauthorized {
		return mcpAuthRequiredResponse(req, false)
	}
	return mcpJSONRPCErrorResponse(req.Request, callErr)
}

func handleGenericToolCallError(req api.Context, audit *proxyAudit, callErr error) bool {
	response := genericToolCallErrorResponse(req, callErr)
	if response == nil {
		audit.recordRequest()
		audit.recordTransportError(callErr, http.StatusInternalServerError)
		return false
	}
	response.write(req.ResponseWriter, audit)
	return true
}

func compositeLoopbackURLs(serverURL, mcpServerName string, transform func(string) string) (audienceURL, targetURL string) {
	audienceURL = fmt.Sprintf("%s/mcp-connect-composite/%s", strings.TrimSuffix(serverURL, "/"), mcpServerName)
	return audienceURL, transform(audienceURL)
}

func compositeSessionKey(serverConfig mcp.ServerConfig) string {
	return tunnel.CompositeSessionKey(serverConfig.MCPServerName)
}

func NewHandler(ctx context.Context, mcpSessionManager *mcp.SessionManager, globalTokenStore mcp.GlobalTokenStore, tokenService *persistent.TokenService, auditLogCollector proxyAuditCollector, serverURL, dsn, secretBindingAllowedLabel string, tunnelManager *tunnel.Manager, authorizationURLs func(api.Context) ([]mmmcp.AuthorizationURL, error), guardrailEngine *guardrails.Engine) (*Handler, error) {
	ctx, cancel := context.WithCancel(ctx)

	var componentAuth componenthttp.OAuthHandlerProvider
	if tokenService != nil {
		componentAuth = compositeComponentAuth{ctx: ctx, tokens: tokenService}
	}
	composite, err := mmmcp.New(ctx, &mmmcpconfig.Config{}, mmmcp.Options{
		Logger:            slog.Default(),
		DSN:               dsn,
		ForwardClientInfo: true,
		OAuth:             componentAuth,
		AuthorizationURLs: compositeAuthorizationURLs(authorizationURLs),
		ClientInfo: &gomcp.Implementation{
			Name:    "Obot MCP Gateway",
			Version: version.Get().String(),
		},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create composite MCP server: %w", err)
	}

	return &Handler{
		ctx:                       ctx,
		cancel:                    cancel,
		mcpSessionManager:         mcpSessionManager,
		globalTokenStore:          globalTokenStore,
		tokenService:              tokenService,
		auditLogCollector:         auditLogCollector,
		composite:                 composite,
		hookRunner:                mcp.NewHookRunner(mcpSessionManager),
		guardrails:                guardrailEngine,
		tunnelManager:             tunnelManager,
		secretBindingAllowedLabel: secretBindingAllowedLabel,
		serverURL:                 serverURL,
	}, nil
}

// Close releases the MMMCP composite server and its component resources.
func (h *Handler) Close() error {
	if h == nil {
		return nil
	}

	if h.cancel != nil {
		h.cancel()
	}
	if h.tunnelManager != nil {
		h.tunnelManager.CloseCompositeSessions()
	}

	if h.composite != nil {
		return h.composite.Close()
	}

	return nil
}

func (h *Handler) Proxy(req api.Context) error {
	if !req.UserIsAuthenticated() {
		mcpAuthRequiredResponse(req, false).write(req.ResponseWriter, nil)
		return nil
	}

	serverConfig, err := h.ensureServerIsDeployed(req)
	if err != nil {
		if errors.Is(err, errMCPServerRequiresConfiguration) {
			return nil
		}
		err = fmt.Errorf("failed to ensure server is deployed: %w", err)
		if response := mcpJSONRPCErrorResponse(req.Request, err); response != nil {
			response.write(req.ResponseWriter, nil)
			return nil
		}
		return err
	}

	isCompositeRequest := strings.HasSuffix(req.URL.Path, "/mcp-connect-composite/"+req.PathValue("mcp_id"))
	if isCompositeRequest {
		ownerMarkerPresent, ownerLocal := h.tunnelManager.ConsumeCompositeOwnerRequest(req.Request)
		if ownerMarkerPresent && !ownerLocal {
			http.Error(req.ResponseWriter, "invalid composite owner marker", http.StatusForbidden)
			return nil
		}
		if !ownerLocal {
			key := compositeSessionKey(serverConfig)
			if !h.tunnelManager.HasCompositeSession(key) {
				if err := h.tunnelManager.ClaimCompositeSession(req.Context(), h.ctx, key); err != nil {
					http.Error(req.ResponseWriter, "composite owner is unavailable", http.StatusServiceUnavailable)
					return nil
				}
			}
			if err := h.tunnelManager.ForwardComposite(req.ResponseWriter, req.Request, key, serverConfig.MCPServerName); err != nil {
				http.Error(req.ResponseWriter, "composite owner is unavailable", http.StatusServiceUnavailable)
			}
			return nil
		}
	}

	var (
		token       string
		tokenSource oauth2.TokenSource
		now         = time.Now()
	)
	if !isCompositeRequest {
		// For aggregate runtimes, the /mcp-connect/{mcp_id} path only handles audit logs and hooks.
		// The URL is changed to /mcp-connect-composite/{mcp_id} which comes back here and handles
		// the multi-MCP server configuration (and calls no audit logs nor hooks).
		if serverConfig.Runtime == types.RuntimeVMCP {
			compositeAudienceURL, compositeTargetURL := compositeLoopbackURLs(h.serverURL, serverConfig.MCPServerName, h.mcpSessionManager.TransformObotHostname)
			serverConfig.URL = compositeTargetURL

			authorizedMCPIDs := make([]string, 0, len(serverConfig.Components))
			for _, component := range serverConfig.Components {
				authorizedMCPIDs = append(authorizedMCPIDs, component.ConnectID())
			}

			// In order for the loopback to work, we need to authenticate as a composite MCP server.
			token, _, err = newCompositeLoopbackToken(req.Context(), h.tokenService, compositeLoopbackTokenContext(req.User, compositeAudienceURL, serverConfig.MCPServerName, authorizedMCPIDs, now), now)
			if err != nil {
				return err
			}
		} else {
			// Token refreshes go to the server-supplied token endpoint, so they use a client
			// with the admin's egress policy and the server's network path, and none of the
			// server's configured headers.
			refreshClient, err := h.mcpSessionManager.OAuthClientForServer(serverConfig)
			if err != nil {
				return fmt.Errorf("failed to get HTTP client for token refresh: %w", err)
			}
			tokenSource, err = h.globalTokenStore.ForUserAndMCP(serverConfig.UserID, cmp.Or(serverConfig.MCPServerInstanceID, serverConfig.MCPServerName), serverConfig.URL).TokenSource(context.WithValue(h.ctx, oauth2.HTTPClient, refreshClient))
			if err != nil {
				return fmt.Errorf("failed to get token source: %w", err)
			}
		}

		if token != "" {
			serverConfig.Headers = append(serverConfig.Headers, "Authorization=Bearer "+token)
		}

		client, err := h.mcpSessionManager.HTTPClientForServer(serverConfig, mcp.HTTPClientOptions{TokenSource: tokenSource, DirectConnect: true})
		if err != nil {
			return fmt.Errorf("failed to get HTTP client for server: %w", err)
		}

		u, err := url.Parse(serverConfig.URL)
		if err != nil {
			return err
		}

		hookConfig, hookServers := mcp.ServerHookConfig(serverConfig)

		audit, err := newProxyAudit(req.Request, serverConfig.AuditLogMetadata, h.auditLogCollector, req.Storage)
		if err != nil {
			return fmt.Errorf("failed to prepare MCP request audit log: %w", err)
		}
		var resolvedCall *resolvedToolCall
		if serverConfig.Runtime == types.RuntimeVMCP && serverConfig.ToolSearch {
			// Guardrail filters run in-process, without the hook runner.
			inspect := audit != nil || (len(hookConfig) > 0 && (h.hookRunner != nil || (h.guardrails != nil && hookConfig.HasGuardrails())))
			resolvedCall, err = h.resolveGenericToolCall(req.Request, compositeConfig(serverConfig, req.User), token, inspect)
			if err != nil {
				if handleGenericToolCallError(req, audit, err) {
					return nil
				}
				return err
			}
			if resolvedCall != nil && audit != nil {
				audit.entry.CallIdentifier = resolvedCall.name
			}
		}

		var filterGuardrails *mcpGuardrails
		// Resolve the caller for guardrail filters on every method: tool results can also
		// arrive on a GET stream (stream resumption, or the legacy SSE transport).
		if h.guardrails != nil && hookConfig.HasGuardrails() {
			identity, _, err := guardrails.ResolveIdentity(req.Context(), req.GatewayClient, req.User)
			if err != nil {
				return err
			}
			filterGuardrails = newMCPGuardrails(h.guardrails, identity, serverConfig, req.Request.Header)
		}

		hooks, err := newHookProcessor(req.Request, h.hookRunner, hookConfig, hookServers, audit, newHookCorrelationStore(req.Storage, serverConfig.AuditLogMetadata), resolvedCall, filterGuardrails)
		if err != nil {
			if errors.Is(err, errMCPCaseVariantFieldNames) && handleGenericToolCallError(req, audit, err) {
				return nil
			}
			return fmt.Errorf("failed to prepare MCP request hooks: %w", err)
		}

		audit.recordRequest()

		if body, blocked, hookErr := hooks.blockedRequest(); blocked {
			audit.recordBlockedRequest(body, hookErr)
			req.ResponseWriter.Header().Set("Content-Type", "application/json")
			req.WriteHeader(http.StatusOK)
			_, _ = req.ResponseWriter.Write(body)
			return nil
		}

		(&httputil.ReverseProxy{
			Transport: client.Transport,
			Rewrite: func(r *httputil.ProxyRequest) {
				rewriteProxyRequest(r, u)
			},
			ModifyResponse: func(resp *http.Response) error {
				rewriteMCPAuthResponse(req, resp)
				if err := hooks.filterResponse(resp); err != nil {
					return err
				}
				return audit.wrapResponse(resp)
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
				audit.recordTransportError(err, http.StatusBadGateway)
				mcpServerName := serverConfig.MCPServerDisplayName
				if mcpServerName == "" {
					mcpServerName = serverConfig.MCPServerName
				}
				http.Error(w, fmt.Sprintf("failed to proxy request to MCP server %s: %v", mcpServerName, err), http.StatusBadGateway)
			},
		}).ServeHTTP(req.ResponseWriter, req.Request)

		return nil
	}

	for i := range serverConfig.PassthroughHeaderNames {
		if i < len(serverConfig.PassthroughHeaderValues) {
			req.Request.Header.Set(serverConfig.PassthroughHeaderNames[i], serverConfig.PassthroughHeaderValues[i])
		}
	}

	ctx := context.WithValue(req.Context(), compositeAPIContextKey{}, req)
	ctx = mmmcp.ContextWithConfig(ctx, compositeConfig(serverConfig, req.User))
	ctx = mmmcp.ContextWithConfigID(ctx, fmt.Sprintf("%s++%s", serverConfig.MCPServerName, serverConfig.UserID))
	h.composite.HTTPHandler().ServeHTTP(req.ResponseWriter, req.WithContext(ctx))
	return nil
}

// isLoopbackHost reports whether a Host header value, with or without a port,
// names localhost or a loopback address.
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func rewriteProxyRequest(r *httputil.ProxyRequest, upstreamURL *url.URL) {
	// These headers may authenticate the client to Obot and must not cross the
	// trust boundary to the upstream MCP server. The transport adds any
	// explicitly configured upstream credentials after this rewrite.
	for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization", "X-API-Key", "X-Obot-Api-Key"} {
		r.Out.Header.Del(header)
	}

	// SetXForwarded preserves the X-Forwarded-For handling that ReverseProxy
	// applied automatically under the deprecated Director. It also writes
	// X-Forwarded-Host and X-Forwarded-Proto from this hop, which are rewritten
	// below: dropped for a loopback host, otherwise the host the client used
	// over https.
	r.SetXForwarded()

	if isLoopbackHost(r.In.Host) {
		// Obot dialed itself (a vMCP component's local connect URL) or the client
		// connected locally. A loopback host means nothing to the upstream, and a
		// proxy in front of it that trusts these headers (ingress-nginx with
		// force-ssl-redirect) answers with a redirect to https://localhost/.
		r.Out.Header.Del("X-Forwarded-Host")
		r.Out.Header.Del("X-Forwarded-Proto")
	} else {
		r.Out.Header.Set("X-Forwarded-Host", r.In.Host)
		r.Out.Header.Set("X-Forwarded-Proto", "https")
	}

	r.Out.Host = upstreamURL.Host
	r.Out.URL.Scheme = upstreamURL.Scheme
	r.Out.URL.Host = upstreamURL.Host
	r.Out.URL.Path = upstreamURL.Path
	if rest := r.In.PathValue("rest"); rest != "" {
		if strings.HasPrefix(rest, "/") {
			r.Out.URL.Path = rest
		} else {
			r.Out.URL.Path = "/" + rest
		}
	}

	// Merge query parameters from the incoming request and the upstream URL.
	// Preserve all values; if a key exists in both, both values will be present.
	upstreamQuery := upstreamURL.Query()
	origQuery := r.In.URL.Query()
	for k, vs := range origQuery {
		for _, v := range vs {
			upstreamQuery.Add(k, v)
		}
	}
	r.Out.URL.RawQuery = upstreamQuery.Encode()
}

func (h *Handler) ensureServerIsDeployed(req api.Context) (mcp.ServerConfig, error) {
	mcpID := req.PathValue("mcp_id")

	if system.IsSystemMCPServerID(mcpID) {
		return h.ensureSystemServerIsDeployed(req, mcpID)
	}
	if system.IsVMCPID(mcpID) || system.IsVMCPInstanceID(mcpID) {
		return h.mcpSessionManager.ServerConfigForVMCP(req.Context(), mcpID, req.User)
	}

	mcpID, mcpServer, mcpServerConfig, missingConfig, err := h.mcpSessionManager.ServerForActionWithConnectIDAllowMissingConfig(req.Context(), mcpID, req.User)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to get mcp server config: %w", err)
	}
	if mcpServerConfig.Runtime == types.RuntimeVMCP {
		// Legacy slugs can resolve to vMCPs, which have no deployment of their own.
		return mcpServerConfig, nil
	}
	if mcpServer.Spec.Template {
		return mcp.ServerConfig{}, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}
	if len(missingConfig) > 0 {
		mcpAuthRequiredResponse(req, true).write(req.ResponseWriter, nil)
		return mcp.ServerConfig{}, errMCPServerRequiresConfiguration
	}

	mcpServerConfig, err = h.mcpSessionManager.LaunchServer(req.Context(), mcpServerConfig)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to launch mcp server: %w", err)
	}

	return mcpServerConfig, nil
}

func mcpAuthChallenge(req api.Context) string {
	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	connectPath := "mcp-connect"
	if strings.HasPrefix(req.URL.Path, "/mcp-connect-composite/") {
		connectPath = "mcp-connect-composite"
	}

	return fmt.Sprintf(`Bearer realm="Obot MCP Gateway", resource_metadata="%s/.well-known/oauth-protected-resource/%s/%s"`, baseURL, connectPath, req.PathValue("mcp_id"))
}

// Upstream authentication challenges must lead clients back through Obot,
// including when the upstream is an aggregate's internal loopback endpoint.
func rewriteMCPAuthResponse(req api.Context, resp *http.Response) {
	if resp.StatusCode != http.StatusUnauthorized {
		return
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	response := mcpAuthRequiredResponse(req, false)
	resp.Header = response.headers.Clone()
	resp.Header.Del("Content-Length")
	resp.Body = io.NopCloser(bytes.NewReader(response.body))
	resp.ContentLength = int64(len(response.body))
	resp.TransferEncoding = nil
	resp.Trailer = nil
}

func mcpAuthRequiredResponse(req api.Context, requiresConfig bool) *mcpResponse {
	message := "MCP server requires authentication"
	if requiresConfig {
		message = "MCP server requires configuration"
	}
	response := mcpTextResponse(http.StatusUnauthorized, message)
	response.headers.Set("WWW-Authenticate", mcpAuthChallenge(req))
	return response
}

// compositeLoopbackTokenContext returns the token a composite server's loopback authenticates with, for the caller
// that reached the composite server.
func compositeLoopbackTokenContext(caller user.Info, audience, mcpServerName string, authorizedMCPIDs []string, now time.Time) persistent.TokenContext {
	tokenContext := persistent.TokenContext{
		Audience:         audience,
		IssuedAt:         persistent.NewTime(now),
		ExpiresAt:        persistent.NewTime(now.Add(10 * time.Minute)),
		UserID:           caller.GetUID(),
		UserName:         caller.GetName(),
		UserEmail:        cmp.Or(caller.GetExtra()["email"]...),
		UserGroups:       []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
		MCPID:            mcpServerName,
		AuthorizedMCPIDs: authorizedMCPIDs,
	}
	if authz.IsClaudeEnvironment(caller) {
		tokenContext.UserGroups = append(tokenContext.UserGroups, types.GroupClaudeEnvironment)
	}

	return tokenContext
}

func (h *Handler) ensureSystemServerIsDeployed(req api.Context, mcpID string) (mcp.ServerConfig, error) {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, mcpID); err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to get system MCP server %q: %w", mcpID, err)
	}

	if systemServer.Spec.Manifest.Enabled != nil && !*systemServer.Spec.Manifest.Enabled {
		return mcp.ServerConfig{}, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "systemmcpserver"}, mcpID)
	}

	// Only look up credentials if the manifest has env vars without static values.
	// This avoids expensive credential lookups on the hot path for servers
	// where all env vars have static values.
	credEnv := make(map[string]string)
	var needsCredentials bool
	for _, env := range systemServer.Spec.Manifest.Config {
		if env.Value == "" {
			needsCredentials = true
			break
		}
	}

	if needsCredentials {
		credCtx := systemServer.Name
		creds, err := req.GatewayClient.ListCredentials(req.Context(), gateway.ListCredentialsOptions{
			CredentialContexts: []string{credCtx},
		})
		if err != nil {
			return mcp.ServerConfig{}, fmt.Errorf("failed to list credentials for system server: %w", err)
		}

		for _, cred := range creds {
			credDetail, err := req.GatewayClient.RevealCredential(req.Context(), []string{credCtx}, cred.Name)
			if err != nil {
				continue
			}
			maps.Copy(credEnv, credDetail.Secrets)
		}
	}

	credEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, systemServer.Spec.Manifest.Config, credEnv, h.secretBindingAllowedLabel)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	audiences := systemServer.ValidConnectURLs(baseURL)

	// Ownership, not the acting identity: a system server deployed for an agent
	// belongs to the person who created that agent.
	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, audiences, req.User.GetUID(), credEnv)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to convert system server to config: %w", err)
	}

	serverConfig, err = h.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to launch system MCP server: %w", err)
	}

	return serverConfig, nil
}
