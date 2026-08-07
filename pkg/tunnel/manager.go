package tunnel

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/rancher/remotedialer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	bridgePathPrefix          = "/tunnel/bridge/"
	bridgeAuthorizationHeader = "X-Obot-Tunnel-Bridge-Authorization"
	bridgePrincipalName       = "obot-tunnel-bridge"
	tunnelPeerPrincipalName   = "obot-tunnel-peer"
	tunnelNameHeader          = "X-Obot-Tunnel-Name"

	forwardTargetHeader = "X-Obot-Tunnel-Target"
	forwardErrorHeader  = "X-Obot-Tunnel-Error"
	forwardNetwork      = "obot-http"
	forwardAddress      = "request"
	disconnectNetwork   = "obot-control"
	disconnectAddress   = "disconnect"
)

type sessionKeyContextKey struct{}

// Manager owns the tunnel sessions, the remotedialer peer mesh, and the HTTP
// bridge used by Obot's existing MCP HTTP client.
type Manager struct {
	PeerConfig
	bridgeBaseURL       string
	bridgeHost          string
	bridgeAuthorization string
	tunnels             kclient.Reader
	remoteDialer        *remotedialer.Server

	mu          sync.RWMutex
	connections map[string]map[*localTunnelConnection]struct{}

	peerMu          sync.Mutex
	peers           map[string]struct{}
	peerConnections map[net.Conn]string
	closed          bool
}

type localTunnelConnection struct {
	key  string
	conn net.Conn
}

// NewManager creates a tunnel manager whose bridge is served from
// bridgeBaseURL. The caller must register ServeBridge on the HTTP server mux.
// Supplying a peer config enables remotedialer peering; the controller layer
// feeds discovered EndpointSlice peers through ReconcilePeers. An omitted or
// empty config keeps single-replica mode.
func NewManager(ctx context.Context, bridgeBaseURL string, tunnels kclient.Reader, peerConfig PeerConfig) (*Manager, error) {
	if tunnels == nil {
		return nil, errors.New("tunnel storage reader is required")
	}
	parsedBaseURL, err := url.Parse(strings.TrimSpace(bridgeBaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse tunnel bridge base URL: %w", err)
	}
	if parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https" {
		return nil, errors.New("tunnel bridge base URL scheme must be either http or https")
	}
	if parsedBaseURL.Hostname() == "" {
		return nil, errors.New("tunnel bridge base URL hostname is required")
	}
	if parsedBaseURL.User != nil || parsedBaseURL.RawQuery != "" || parsedBaseURL.Fragment != "" ||
		(parsedBaseURL.EscapedPath() != "" && parsedBaseURL.EscapedPath() != "/") {
		return nil, errors.New("tunnel bridge base URL must contain only a scheme and host")
	}

	bridgeAuthorization := make([]byte, 32)
	if _, err := rand.Read(bridgeAuthorization); err != nil {
		return nil, fmt.Errorf("failed to generate tunnel bridge capability: %w", err)
	}

	peerConfig = peerConfig.normalized()
	if err := peerConfig.Validate(); err != nil {
		return nil, err
	}
	m := &Manager{
		PeerConfig:          peerConfig,
		bridgeBaseURL:       parsedBaseURL.Scheme + "://" + parsedBaseURL.Host,
		bridgeHost:          parsedBaseURL.Host,
		bridgeAuthorization: "Bearer " + base64.RawURLEncoding.EncodeToString(bridgeAuthorization),
		tunnels:             tunnels,
		connections:         make(map[string]map[*localTunnelConnection]struct{}),
		peers:               make(map[string]struct{}),
		peerConnections:     make(map[net.Conn]string),
	}
	m.remoteDialer = remotedialer.New(m.authorizeRemoteDialer, remoteDialerErrorWriter)
	m.remoteDialer.PeerID = peerConfig.ID
	m.remoteDialer.PeerToken = peerConfig.Token
	m.remoteDialer.ClientConnectAuthorizer = func(network, address string) bool {
		return network == forwardNetwork && address == forwardAddress ||
			network == disconnectNetwork && address == disconnectAddress
	}

	go func() {
		<-ctx.Done()
		m.Close()
	}()

	return m, nil
}

func (m *Manager) authorizeRemoteDialer(req *http.Request) (string, bool, error) {
	key, _ := req.Context().Value(sessionKeyContextKey{}).(string)
	return key, key != "", nil
}

func remoteDialerErrorWriter(w http.ResponseWriter, _ *http.Request, status int, err error) {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}
	http.Error(w, err.Error(), status)
}

// BridgeHost returns the gateway host:port that must be allowlisted by the
// safe MCP HTTP client.
func (m *Manager) BridgeHost() string {
	if m == nil {
		return ""
	}
	return m.bridgeHost
}

// BridgeAuthorization returns the internal header that must accompany bridge
// requests. ServeBridge removes this header before forwarding the request to
// the tunnel client.
func (m *Manager) BridgeAuthorization() (name, value string) {
	if m == nil {
		return "", ""
	}
	return bridgeAuthorizationHeader, m.bridgeAuthorization
}

// AuthenticateRequest authenticates requests generated for the internal
// tunnel bridge and websocket connections made by another Obot replica.
func (m *Manager) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if m.bridgeRequestAuthorized(req) {
		return &authenticator.Response{User: &user.DefaultInfo{
			Name:   bridgePrincipalName,
			UID:    bridgePrincipalName,
			Groups: []string{apitypes.GroupTunnelBridge},
		}}, true, nil
	}
	if m.peerRequestAuthorized(req) {
		peerID := req.Header.Get(remotedialer.ID)
		return &authenticator.Response{User: &user.DefaultInfo{
			Name:   tunnelPeerPrincipalName,
			UID:    peerID,
			Groups: []string{apitypes.GroupTunnelPeer},
		}}, true, nil
	}

	return nil, false, nil
}

func (m *Manager) bridgeRequestAuthorized(req *http.Request) bool {
	return m != nil && req != nil && subtle.ConstantTimeCompare(
		[]byte(req.Header.Get(bridgeAuthorizationHeader)),
		[]byte(m.bridgeAuthorization),
	) == 1
}

func (m *Manager) peerRequestAuthorized(req *http.Request) bool {
	return m != nil && m.Token != "" && isTunnelPeerRequest(req) &&
		req.Header.Get(remotedialer.ID) != "" && subtle.ConstantTimeCompare(
		[]byte(req.Header.Get(remotedialer.Token)),
		[]byte(m.Token),
	) == 1
}

type bridgeTarget struct {
	TunnelName string `json:"tunnelName"`
	URL        string `json:"url"`
}

type bridgeRoundTripper struct {
	manager    *Manager
	tunnelName string
	transport  http.RoundTripper
	headers    http.Header
}

func (b *bridgeRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("tunnel bridge request URL is required")
	}

	outbound := request.Clone(request.Context())
	if outbound.Header == nil {
		outbound.Header = make(http.Header, len(b.headers)+1)
	}

	maps.Copy(outbound.Header, b.headers)

	if !b.manager.isBridgeURLForTunnel(outbound.URL, b.tunnelName) {
		bridgeURL, err := b.manager.BridgeURL(b.tunnelName, outbound.URL.String())
		if err != nil {
			return nil, fmt.Errorf("failed to prepare tunneled HTTP request: %w", err)
		}
		outbound.URL, err = url.Parse(bridgeURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tunnel bridge URL: %w", err)
		}
		outbound.Host = ""
	}

	name, value := b.manager.BridgeAuthorization()
	outbound.Header.Set(name, value)
	return b.transport.RoundTrip(outbound)
}

// HTTPClient returns an HTTP client that routes every request through the
// named tunnel. Redirects rewritten by the bridge remain on the same tunnel.
func (m *Manager) HTTPClient(tunnelName string, headers http.Header, timeout time.Duration) (*http.Client, error) {
	if m == nil {
		return nil, errors.New("tunnel manager is not configured")
	}
	if err := apitypes.ValidateTunnelName(tunnelName); err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &bridgeRoundTripper{
			manager:    m,
			tunnelName: tunnelName,
			transport:  http.DefaultTransport,
			headers:    headers.Clone(),
		},
	}, nil
}

func (m *Manager) isBridgeURLForTunnel(targetURL *url.URL, tunnelName string) bool {
	if targetURL == nil ||
		!strings.EqualFold(targetURL.Scheme+"://"+targetURL.Host, m.bridgeBaseURL) ||
		!strings.HasPrefix(targetURL.Path, bridgePathPrefix) {
		return false
	}

	encoded := strings.TrimPrefix(targetURL.Path, bridgePathPrefix)
	if encoded == "" || strings.Contains(encoded, "/") {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	var target bridgeTarget
	return json.Unmarshal(payload, &target) == nil && target.TunnelName == tunnelName
}

// BridgeURL converts an ordinary HTTP(S) target and MCPTunnel name into an
// internal URL on the gateway bridge.
func (m *Manager) BridgeURL(tunnelName, rawURL string) (string, error) {
	if m == nil {
		return "", errors.New("tunnel manager is not configured")
	}
	if err := apitypes.ValidateTunnelName(tunnelName); err != nil {
		return "", err
	}
	targetURL, err := parseTargetURL(rawURL)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(bridgeTarget{TunnelName: tunnelName, URL: targetURL.String()})
	if err != nil {
		return "", fmt.Errorf("failed to encode tunnel bridge target: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return m.bridgeBaseURL + bridgePathPrefix + encoded, nil
}

func parseTargetURL(rawURL string) (*url.URL, error) {
	targetURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse tunnel target URL: %w", err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return nil, errors.New("tunnel target URL scheme must be either http or https")
	}
	if targetURL.Hostname() == "" {
		return nil, errors.New("tunnel target URL hostname is required")
	}
	if targetURL.User != nil {
		return nil, errors.New("tunnel target URL must not include user information")
	}
	return targetURL, nil
}

func tunnelSessionKey(name, credentialID string) string {
	digest := sha256.Sum256([]byte(name + "\x00" + credentialID))
	return "obot-" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func configuredTunnelSessionKey(tunnel *v1.MCPTunnel) (string, error) {
	credentialID, err := CredentialID(tunnel.Spec.Credential)
	if err != nil {
		return "", fmt.Errorf("failed to read credential ID for tunnel %q: %w", tunnel.Name, err)
	}
	if indexedID := strings.TrimSpace(tunnel.Spec.CredentialID); indexedID != "" && indexedID != credentialID {
		return "", fmt.Errorf("tunnel %q has an inconsistent credential ID", tunnel.Name)
	}
	return tunnelSessionKey(tunnel.Name, credentialID), nil
}

// Connections preserves the process-local snapshot returned by earlier
// versions of Manager. Use ConnectionsContext for an installation-wide view.
func (m *Manager) Connections() []apitypes.TunnelConnection {
	if m == nil {
		return []apitypes.TunnelConnection{}
	}
	m.mu.RLock()
	connections := make([]string, 0, len(m.connections))
	for name, tunnelConnections := range m.connections {
		for connection := range tunnelConnections {
			if connection.conn != nil {
				connections = append(connections, name)
				break
			}
		}
	}
	m.mu.RUnlock()
	sort.Strings(connections)

	result := make([]apitypes.TunnelConnection, 0, len(connections))
	for _, name := range connections {
		result = append(result, apitypes.TunnelConnection{Name: name})
	}
	return result
}

// ConnectionsContext returns a deterministic installation-wide snapshot of
// the configured tunnels that currently have a local or peer-advertised
// session. A connection may disappear immediately after the snapshot is taken.
func (m *Manager) ConnectionsContext(ctx context.Context) ([]apitypes.TunnelConnection, error) {
	if m == nil {
		return []apitypes.TunnelConnection{}, nil
	}
	var tunnels v1.MCPTunnelList
	if err := m.tunnels.List(ctx, &tunnels, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, fmt.Errorf("failed to list MCP tunnels: %w", err)
	}

	connections := make([]string, 0, len(tunnels.Items))
	for i := range tunnels.Items {
		key, err := configuredTunnelSessionKey(&tunnels.Items[i])
		if err != nil {
			return nil, err
		}
		if m.remoteDialer.HasSession(key) {
			connections = append(connections, tunnels.Items[i].Name)
		}
	}
	sort.Strings(connections)

	result := make([]apitypes.TunnelConnection, 0, len(connections))
	for _, name := range connections {
		result = append(result, apitypes.TunnelConnection{Name: name})
	}
	return result, nil
}

// Disconnect closes all tunnel clients with this name attached to this
// replica. Call DisconnectCredential when the previous credential ID is
// available so clients attached to other replicas can also be reached.
func (m *Manager) Disconnect(name string) {
	m.DisconnectCredential(name, "")
}

// DisconnectCredential closes all sessions using a tunnel's previous
// credential. Control connections are routed through remotedialer so clients
// attached to other replicas can also be reached.
func (m *Manager) DisconnectCredential(name, credentialID string) {
	if m == nil {
		return
	}
	var key string
	if credentialID != "" {
		key = tunnelSessionKey(name, credentialID)
	}

	var local []net.Conn
	m.mu.Lock()
	connections := m.connections[name]
	for connection := range connections {
		if key != "" && connection.key != key {
			continue
		}
		delete(connections, connection)
		if connection.conn != nil {
			local = append(local, connection.conn)
		}
	}
	if len(connections) == 0 {
		delete(m.connections, name)
	}
	m.mu.Unlock()
	for _, connection := range local {
		_ = connection.Close()
	}
	if key == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for m.remoteDialer.HasSession(key) {
		if err := m.disconnectRemoteSession(ctx, key); err != nil && ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *Manager) disconnectRemoteSession(ctx context.Context, key string) error {
	connection, err := m.remoteDialer.Dialer(key)(ctx, disconnectNetwork, disconnectAddress)
	if err != nil {
		return err
	}
	return connection.Close()
}

// CloseAll closes only the tunnel clients accepted by this replica. It must
// not disconnect sessions owned by healthy peers during a rolling shutdown.
func (m *Manager) CloseAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	connections := make([]net.Conn, 0, len(m.connections))
	for name, tunnelConnections := range m.connections {
		delete(m.connections, name)
		for connection := range tunnelConnections {
			if connection.conn != nil {
				connections = append(connections, connection.conn)
			}
		}
	}
	m.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
}

// Close stops local tunnel sessions and all outgoing peer reconnect loops.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.CloseAll()

	m.peerMu.Lock()
	if m.closed {
		m.peerMu.Unlock()
		return
	}
	m.closed = true
	for id := range m.peers {
		m.remoteDialer.RemovePeer(id)
		delete(m.peers, id)
	}
	peerConnections := make([]net.Conn, 0, len(m.peerConnections))
	for connection := range m.peerConnections {
		delete(m.peerConnections, connection)
		peerConnections = append(peerConnections, connection)
	}
	m.peerMu.Unlock()
	for _, connection := range peerConnections {
		_ = connection.Close()
	}
}

// ServeConnect upgrades an authorized request for name. The name must come
// from the tunnel principal established by TunnelAuthenticator.
func (m *Manager) ServeConnect(w http.ResponseWriter, r *http.Request, name string) {
	if !isTunnelConnectRequest(r) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := apitypes.ValidateTunnelName(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	token, ok := tunnelBearerToken(r)
	if !ok {
		http.Error(w, "invalid tunnel credential", http.StatusUnauthorized)
		return
	}
	matcher, ok := newCredentialMatcher(token)
	if !ok {
		http.Error(w, "invalid tunnel credential", http.StatusUnauthorized)
		return
	}

	key := tunnelSessionKey(name, matcher.credentialID)
	trackedConnection := &localTunnelConnection{key: key}
	m.mu.Lock()
	connections := m.connections[name]
	if connections == nil {
		connections = make(map[*localTunnelConnection]struct{})
		m.connections[name] = connections
	}
	connections[trackedConnection] = struct{}{}
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		connection := trackedConnection.conn
		if connections := m.connections[name]; connections != nil {
			delete(connections, trackedConnection)
			if len(connections) == 0 {
				delete(m.connections, name)
			}
		}
		m.mu.Unlock()
		if connection != nil {
			_ = connection.Close()
		}
	}()

	// Verify after registering the connection so credential rotation cannot race
	// with the websocket upgrade.
	var tunnel v1.MCPTunnel
	if err := m.tunnels.Get(r.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, &tunnel); err != nil {
		if !apierrors.IsNotFound(err) {
			http.Error(w, "failed to verify tunnel credential", http.StatusInternalServerError)
			return
		}
		http.Error(w, "invalid tunnel credential", http.StatusUnauthorized)
		return
	}
	if !matcher.Matches(tunnel.Spec.Credential) {
		http.Error(w, "invalid tunnel credential", http.StatusUnauthorized)
		return
	}

	w.Header().Set(tunnelNameHeader, name)
	trackingWriter := &hijackTrackingResponseWriter{
		ResponseWriter: w,
		onHijack: func(connection net.Conn) bool {
			m.mu.Lock()
			defer m.mu.Unlock()
			if _, ok := m.connections[name][trackedConnection]; !ok {
				return false
			}
			trackedConnection.conn = connection
			return true
		},
	}
	ctx := context.WithValue(r.Context(), sessionKeyContextKey{}, key)
	request := r.Clone(ctx)
	request.Header = request.Header.Clone()
	request.Header.Del(remotedialer.ID)
	request.Header.Del(remotedialer.Token)
	m.remoteDialer.ServeHTTP(trackingWriter, request)
}

// ServePeer accepts an internal remotedialer connection from another Obot
// replica. Authentication is performed both by Obot middleware and by
// remotedialer against the discovered peer set.
func (m *Manager) ServePeer(w http.ResponseWriter, r *http.Request) {
	if !isTunnelPeerRequest(r) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	peerID := r.Header.Get(remotedialer.ID)
	var peerConnection net.Conn
	trackingWriter := &hijackTrackingResponseWriter{
		ResponseWriter: w,
		onHijack: func(connection net.Conn) bool {
			m.peerMu.Lock()
			defer m.peerMu.Unlock()
			if m.closed {
				return false
			}
			peerConnection = connection
			m.peerConnections[connection] = peerID
			return true
		},
	}
	m.remoteDialer.ServeHTTP(trackingWriter, r)
	if peerConnection != nil {
		m.peerMu.Lock()
		delete(m.peerConnections, peerConnection)
		m.peerMu.Unlock()
		_ = peerConnection.Close()
	}
}

func isTunnelPeerRequest(req *http.Request) bool {
	return req.Method == http.MethodGet && req.URL.Path == PeerConnectPath
}

type hijackTrackingResponseWriter struct {
	http.ResponseWriter
	onHijack func(net.Conn) bool
}

func (w *hijackTrackingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	connection, readWriter, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}
	if w.onHijack != nil && !w.onHijack(connection) {
		_ = connection.Close()
		return nil, nil, errors.New("connection is no longer accepted")
	}
	return connection, readWriter, nil
}

// ServeBridge forwards an HTTP request over the tunnel selected by the
// encoded internal bridge target in the request path.
func (m *Manager) ServeBridge(w http.ResponseWriter, r *http.Request) {
	if !m.bridgeRequestAuthorized(r) {
		http.NotFound(w, r)
		return
	}

	encoded := r.PathValue("target")
	if encoded == "" {
		encoded = r.PathValue("encoded")
	}
	if encoded == "" {
		encoded = strings.TrimPrefix(r.URL.Path, bridgePathPrefix)
	}
	if encoded == "" || encoded == r.URL.Path || strings.Contains(encoded, "/") {
		http.NotFound(w, r)
		return
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		http.Error(w, "invalid tunnel bridge URL", http.StatusBadRequest)
		return
	}
	var target bridgeTarget
	if err := json.Unmarshal(payload, &target); err != nil {
		http.Error(w, "invalid tunnel bridge target", http.StatusBadRequest)
		return
	}
	if err := apitypes.ValidateTunnelName(target.TunnelName); err != nil {
		http.Error(w, "invalid tunnel bridge target", http.StatusBadRequest)
		return
	}
	targetURL, err := parseTargetURL(target.URL)
	if err != nil {
		http.Error(w, "invalid tunnel target", http.StatusBadRequest)
		return
	}

	var configuredTunnel v1.MCPTunnel
	if err := m.tunnels.Get(r.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: target.TunnelName}, &configuredTunnel); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, fmt.Sprintf("tunnel %q does not exist", target.TunnelName), http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "failed to load tunnel configuration", http.StatusInternalServerError)
		return
	}
	if r.URL.RawQuery != "" {
		query := targetURL.Query()
		for key, values := range r.URL.Query() {
			for _, value := range values {
				query.Add(key, value)
			}
		}
		targetURL.RawQuery = query.Encode()
	}
	if !configuredTunnel.Spec.Manifest.AllowsURL(targetURL.String()) {
		http.Error(w, fmt.Sprintf("target URL is not allowed by tunnel %q", target.TunnelName), http.StatusForbidden)
		return
	}
	key, err := configuredTunnelSessionKey(&configuredTunnel)
	if err != nil {
		http.Error(w, "failed to load tunnel credential", http.StatusInternalServerError)
		return
	}
	if !m.remoteDialer.HasSession(key) {
		http.Error(w, fmt.Sprintf("tunnel %q is not connected", target.TunnelName), http.StatusServiceUnavailable)
		return
	}

	header := r.Header.Clone()
	header.Del(bridgeAuthorizationHeader)
	removeHopHeaders(header)
	// The target may start responding before it has consumed the entire request
	// body. HTTP/2 permits this by default; HTTP/1 requires opting in so the
	// server does not drain the upload before forwarding the response.
	_ = http.NewResponseController(w).EnableFullDuplex()
	response, err := m.roundTrip(r.Context(), key, target.TunnelName, r.Method, targetURL.String(), header, r.ContentLength, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("tunnel request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	responseHeader := response.Header.Clone()
	if responseHeader == nil {
		responseHeader = make(http.Header)
	}
	removeHopHeaders(responseHeader)
	responseHeader.Del(forwardErrorHeader)
	responseHeader.Del(forwardTargetHeader)
	responseHeader.Del(tunnelNameHeader)
	if location := responseHeader.Get("Location"); location != "" {
		if rewritten, rewriteErr := m.rewriteTargetURL(target.TunnelName, targetURL, location); rewriteErr == nil {
			responseHeader.Set("Location", rewritten)
		} else {
			responseHeader.Del("Location")
		}
	}

	isSSE := strings.HasPrefix(strings.ToLower(responseHeader.Get("Content-Type")), "text/event-stream")
	if isSSE {
		responseHeader.Del("Content-Length")
	} else if response.ContentLength >= 0 {
		responseHeader.Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	copyHeaders(w.Header(), responseHeader)
	w.WriteHeader(response.StatusCode)

	if r.Method == http.MethodHead {
		return
	}
	if isSSE {
		_ = m.copySSE(w, response.Body, target.TunnelName, targetURL)
		return
	}
	_, _ = io.Copy(w, response.Body)
}

// roundTrip carries exactly one framed HTTP exchange per remotedialer
// connection. The virtual connection is not exposed as a generic TCP tunnel;
// HTTP body framing keeps the request and response directions independent
// without relying on TCP CloseWrite semantics.
func (m *Manager) roundTrip(ctx context.Context, key, tunnelName, method, target string, header http.Header, contentLength int64, body io.ReadCloser) (*http.Response, error) {
	outboundBody := body
	if body != nil && body != http.NoBody {
		// The inbound HTTP server owns body. The outbound transport may close
		// its request early after receiving a response or an error; allowing
		// that Close to reach the inbound body can block while Go drains an
		// upload that the caller is still sending.
		outboundBody = noCloseReadCloser{Reader: body}
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://obot-tunnel-forward/", outboundBody)
	if err != nil {
		return nil, err
	}
	request.Header = header.Clone()
	request.Header.Set(forwardTargetHeader, base64.RawURLEncoding.EncodeToString([]byte(target)))
	request.Header.Set(tunnelNameHeader, tunnelName)
	request.Header.Del(forwardErrorHeader)
	request.ContentLength = contentLength

	transport := &http.Transport{
		DisableCompression: true,
		DisableKeepAlives:  true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return m.remoteDialer.Dialer(key)(ctx, forwardNetwork, forwardAddress)
		},
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if encodedError := response.Header.Get(forwardErrorHeader); encodedError != "" {
		defer response.Body.Close()
		message, decodeErr := base64.RawURLEncoding.DecodeString(encodedError)
		if decodeErr != nil {
			return nil, errors.New("tunnel client returned an invalid forwarding error")
		}
		return nil, errors.New(string(message))
	}
	return response, nil
}

type noCloseReadCloser struct {
	io.Reader
}

func (noCloseReadCloser) Close() error {
	return nil
}

// ReconcilePeers updates the remotedialer peer mesh to match desired. It is
// called by the all-replica EndpointSlice controller.
func (m *Manager) ReconcilePeers(desired map[string]string) {
	if m == nil || m.remoteDialer == nil {
		return
	}
	m.peerMu.Lock()
	if m.closed {
		m.peerMu.Unlock()
		return
	}
	var staleConnections []net.Conn
	for id := range m.peers {
		if _, ok := desired[id]; !ok {
			m.remoteDialer.RemovePeer(id)
			delete(m.peers, id)
			for connection, peerID := range m.peerConnections {
				if peerID == id {
					delete(m.peerConnections, connection)
					staleConnections = append(staleConnections, connection)
				}
			}
		}
	}
	for id, peerURL := range desired {
		m.remoteDialer.AddPeer(peerURL, id, m.Token)
		m.peers[id] = struct{}{}
	}
	m.peerMu.Unlock()

	// RemovePeer stops this replica's outgoing reconnect loop. Closing the
	// matching incoming websocket also removes the departed peer's advertised
	// sessions immediately instead of leaving them routable until that peer
	// happens to disconnect.
	for _, connection := range staleConnections {
		_ = connection.Close()
	}
}

func (m *Manager) rewriteTargetURL(name string, base *url.URL, reference string) (string, error) {
	ref, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return "", err
	}
	target := base.ResolveReference(ref)
	if !sameOrigin(base, target) {
		return "", fmt.Errorf("tunnel target reference changes origin")
	}
	return m.BridgeURL(name, target.String())
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// copySSE rewrites legacy MCP endpoint events so subsequent POSTs return to
// the bridge instead of trying to reach the private target directly.
func (m *Manager) copySSE(w http.ResponseWriter, body io.Reader, name string, target *url.URL) error {
	var (
		reader         = bufio.NewReaderSize(body, 64*1024)
		flusher, _     = w.(http.Flusher)
		eventName      string
		continuingLine bool
	)
	for {
		fragment, err := reader.ReadSlice('\n')
		line := string(fragment)
		if len(line) > 0 {
			oversized := continuingLine || errors.Is(err, bufio.ErrBufferFull)
			if !oversized {
				trimmed := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
				ending := line[len(trimmed):]
				if value, ok := strings.CutPrefix(trimmed, "event:"); ok {
					eventName = strings.TrimSpace(value)
				} else if eventName == "endpoint" {
					if value, ok := strings.CutPrefix(trimmed, "data:"); ok {
						rewritten, rewriteErr := m.rewriteTargetURL(name, target, strings.TrimSpace(value))
						if rewriteErr != nil {
							return fmt.Errorf("invalid tunneled SSE endpoint: %w", rewriteErr)
						}
						line = "data: " + rewritten + ending
					}
				}
				if trimmed == "" {
					eventName = ""
				}
			}
			if _, writeErr := io.WriteString(w, line); writeErr != nil {
				return writeErr
			}
			if flusher != nil {
				flusher.Flush()
			}
			continuingLine = errors.Is(err, bufio.ErrBufferFull)
		}
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				continue
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func copyHeaders(destination, source http.Header) {
	for key, values := range source {
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func removeHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for part := range strings.SplitSeq(value, ",") {
			header.Del(strings.TrimSpace(part))
		}
	}
	for _, key := range []string{
		"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		header.Del(key)
	}
}
