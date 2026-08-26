package tunnel

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
)

const (
	compositeSessionKeyPrefix = "obot-mcp-composite-"
)

type compositeRegistration struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type compositeOwnerRoundTripper struct {
	manager *Manager
	next    http.RoundTripper
}

// CompositeSessionKey derives the opaque remotedialer session key for a
// composite MCP server. Only the resolved MCP server name participates in the
// key so aliases that resolve to the same server share one owner.
func CompositeSessionKey(mcpServerName string) string {
	digest := sha256.Sum256([]byte("obot-mcp-composite\x00" + mcpServerName))
	return compositeSessionKeyPrefix + base64.RawURLEncoding.EncodeToString(digest[:])
}

func validCompositeSessionKey(key string) bool {
	encoded, ok := strings.CutPrefix(key, compositeSessionKeyPrefix)
	if !ok {
		return false
	}
	digest, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(digest) == sha256.Size
}

// ConsumeCompositeOwnerRequest verifies and removes the private owner-local
// marker. It also strips every remotedialer control header before the request
// can reach MMMCP. A present but invalid marker must be rejected by the caller.
func (m *Manager) ConsumeCompositeOwnerRequest(req *http.Request) (present, valid bool) {
	marker := req.Header.Get(compositeOwnerHeader)
	present = marker != ""
	valid = m != nil && present && subtle.ConstantTimeCompare([]byte(marker), []byte(m.bridgeAuthorization)) == 1
	stripCompositeControlHeaders(req.Header)
	return present, valid
}

// HasCompositeSession reports whether this replica or one of its peers is
// currently advertising the composite owner key.
func (m *Manager) HasCompositeSession(key string) bool {
	return m != nil && validCompositeSessionKey(key) && m.remoteDialer.HasSession(key)
}

// ClaimCompositeSession coalesces local claims for key, starts a reconnecting
// loopback registration when needed, and waits for the key to become routable.
// waitCtx only bounds this caller; lifetimeCtx owns the registration.
func (m *Manager) ClaimCompositeSession(waitCtx, lifetimeCtx context.Context, key string) error {
	if !validCompositeSessionKey(key) {
		return errors.New("invalid composite session key")
	}
	if m.remoteDialer.HasSession(key) {
		return nil
	}

	m.compositeMu.Lock()
	registration := m.compositeRegistrations[key]
	if registration == nil {
		registrationCtx, cancel := context.WithCancel(lifetimeCtx)
		registration = &compositeRegistration{cancel: cancel, done: make(chan struct{})}
		m.compositeRegistrations[key] = registration
		go m.runCompositeRegistration(registrationCtx, key, registration)
	}
	m.compositeMu.Unlock()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.remoteDialer.HasSession(key) {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return waitCtx.Err()
		case <-registration.done:
			return errors.New("composite registration stopped before becoming ready")
		case <-ticker.C:
		}
	}
}

func (m *Manager) runCompositeRegistration(ctx context.Context, key string, registration *compositeRegistration) {
	defer close(registration.done)
	defer func() {
		m.compositeMu.Lock()
		if m.compositeRegistrations[key] == registration {
			delete(m.compositeRegistrations, key)
		}
		m.compositeMu.Unlock()
	}()

	backoff := 25 * time.Millisecond
	for ctx.Err() == nil {
		connection, err := m.dialCompositeRegistration(ctx, key)
		if err == nil {
			backoff = 25 * time.Millisecond
			err = m.serveCompositeConnection(ctx, connection)
			_ = connection.Close()
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		slog.Debug("Composite affinity registration disconnected, retrying", "error", err, "backoff", backoff)
		backoff = min(backoff*2, time.Second)
	}
}

func (m *Manager) dialCompositeRegistration(ctx context.Context, key string) (*websocket.Conn, error) {
	endpoint, err := url.Parse(m.bridgeBaseURL + compositeRegisterPath + key)
	if err != nil {
		return nil, err
	}
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	header := make(http.Header, 1)
	header.Set(bridgeAuthorizationHeader, m.bridgeAuthorization)
	connection, response, err := (&websocket.Dialer{HandshakeTimeout: 15 * time.Second}).DialContext(ctx, endpoint.String(), header)
	if err == nil {
		return connection, nil
	}
	if response == nil {
		return nil, fmt.Errorf("failed to register composite session: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024))
	return nil, fmt.Errorf("failed to register composite session: %s: %s", response.Status, strings.TrimSpace(string(body)))
}

func (m *Manager) serveCompositeConnection(ctx context.Context, connection *websocket.Conn) error {
	defer connection.Close()
	forwarder := &clientForwarder{
		client: &http.Client{Transport: compositeOwnerRoundTripper{manager: m, next: http.DefaultTransport}},
	}
	localDialer := func(_ context.Context, network, address string) (net.Conn, error) {
		if network != compositeNetwork || address != compositeAddress {
			return nil, fmt.Errorf("composite connection to %s/%s is not allowed", network, address)
		}
		forwardCtx, cancelForward := context.WithCancel(ctx)
		clientConnection, handlerConnection := net.Pipe()
		go func() {
			if err := forwarder.serve(forwardCtx, handlerConnection); err != nil &&
				!errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
				slog.Debug("Composite forwarding connection closed", "error", err)
			}
			cancelForward()
		}()
		return &cancelOnCloseConn{Conn: clientConnection, cancel: cancelForward}, nil
	}
	authorize := compositeConnectAuthorized
	session := remotedialer.NewClientSessionWithDialer(authorize, connection, localDialer)
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	_, err := session.Serve(ctx)
	return err
}

func compositeConnectAuthorized(network, address string) bool {
	return network == compositeNetwork && address == compositeAddress
}

func (c compositeOwnerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	outbound := req.Clone(req.Context())
	outbound.Header = req.Header.Clone()
	stripCompositeControlHeaders(outbound.Header)
	outbound.Header.Set(compositeOwnerHeader, c.manager.bridgeAuthorization)
	ownerBase, err := url.Parse(c.manager.bridgeBaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse composite owner loopback URL: %w", err)
	}
	outbound.URL.Scheme = ownerBase.Scheme
	outbound.URL.Host = ownerBase.Host
	outbound.Host = ownerBase.Host
	return c.next.RoundTrip(outbound)
}

// ServeCompositeRegistration accepts only capability-authenticated loopback
// websocket registrations for valid composite keys.
func (m *Manager) ServeCompositeRegistration(w http.ResponseWriter, r *http.Request) {
	if !m.bridgeRequestAuthorized(r) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, compositeRegisterPath) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key := r.PathValue("key")
	if key == "" {
		key = strings.TrimPrefix(r.URL.Path, compositeRegisterPath)
	}
	if !validCompositeSessionKey(key) || strings.Contains(key, "/") {
		http.NotFound(w, r)
		return
	}

	request := r.Clone(context.WithValue(r.Context(), sessionKeyContextKey{}, key))
	request.Header = request.Header.Clone()
	request.Header.Del(compositeOwnerHeader)
	request.Header.Del(remotedialer.ID)
	request.Header.Del(remotedialer.Token)
	m.remoteDialer.ServeHTTP(w, request)
}

// ForwardComposite forwards a complete HTTP exchange to the advertised owner.
// Any disappearance or transport failure is returned to the caller, which must
// surface it as 503 rather than executing against a different local MMMCP.
func (m *Manager) ForwardComposite(w http.ResponseWriter, r *http.Request, key, mcpServerName string) error {
	if !m.HasCompositeSession(key) {
		return errors.New("composite owner is unavailable")
	}
	targetURL, err := url.Parse(m.bridgeBaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse composite loopback URL: %w", err)
	}
	targetURL.Path = "/mcp-connect-composite/" + mcpServerName
	targetURL.RawQuery = r.URL.RawQuery
	header := r.Header.Clone()
	stripCompositeControlHeaders(header)
	_ = http.NewResponseController(w).EnableFullDuplex()
	response, err := m.roundTripAddress(r.Context(), key, compositeNetwork, compositeAddress, "", r.Method, targetURL.String(), header, r.ContentLength, r.Body)
	if err != nil {
		return fmt.Errorf("failed to forward composite request: %w", err)
	}
	defer response.Body.Close()

	responseHeader := response.Header.Clone()
	stripCompositeControlHeaders(responseHeader)
	removeHopHeaders(responseHeader)
	isSSE := strings.HasPrefix(strings.ToLower(responseHeader.Get("Content-Type")), "text/event-stream")
	if isSSE {
		responseHeader.Del("Content-Length")
	}
	copyHeaders(w.Header(), responseHeader)
	w.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return nil
	}
	if isSSE {
		_ = copyFlushed(w, response.Body)
		return nil
	}
	_, _ = io.Copy(w, response.Body)
	return nil
}

func copyFlushed(w http.ResponseWriter, body io.Reader) error {
	buffer := make([]byte, 32*1024)
	controller := http.NewResponseController(w)
	for {
		n, readErr := body.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return err
			}
			if err := controller.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func stripCompositeControlHeaders(header http.Header) {
	if header == nil {
		return
	}
	header.Del(bridgeAuthorizationHeader)
	header.Del(compositeOwnerHeader)
	header.Del(forwardTargetHeader)
	header.Del(forwardErrorHeader)
	header.Del(tunnelNameHeader)
	header.Del(remotedialer.ID)
	header.Del(remotedialer.Token)
}

// CloseCompositeSessions cancels local registrations and waits for every
// reconnect loop to stop before the local composite handler is closed.
func (m *Manager) CloseCompositeSessions() {
	if m == nil {
		return
	}
	m.compositeMu.Lock()
	registrations := make([]*compositeRegistration, 0, len(m.compositeRegistrations))
	for _, registration := range m.compositeRegistrations {
		registrations = append(registrations, registration)
		registration.cancel()
	}
	m.compositeMu.Unlock()
	for _, registration := range registrations {
		<-registration.done
	}
}
