package tunnel

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/rancher/remotedialer"
)

var tunnelLog = logger.Package()

const websocketCloseTimeout = time.Second

// ConnectURL returns the tunnel websocket endpoint on an Obot instance.
func ConnectURL(serverURL string) (string, error) {
	serverURL = strings.TrimSpace(serverURL)
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse Obot URL: %w", err)
	}
	if parsed.Host == "" {
		return "", errors.New("obot URL must include a host")
	}
	if parsed.User != nil {
		return "", errors.New("obot URL must not include user information")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("obot URL must not include a query or fragment")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
		parsed.Scheme = strings.ToLower(parsed.Scheme)
	default:
		return "", errors.New("obot URL must use http or https")
	}

	basePath := strings.TrimRight(parsed.Path, "/")
	basePath = strings.TrimSuffix(basePath, "/api")
	parsed.Path = strings.TrimRight(basePath, "/") + "/tunnel/connect"
	parsed.RawPath = ""
	return parsed.String(), nil
}

// Dial opens one authenticated websocket connection to an Obot instance.
func Dial(ctx context.Context, serverURL, token string) (*websocket.Conn, error) {
	connection, _, err := dial(ctx, serverURL, token)
	return connection, err
}

func dial(ctx context.Context, serverURL, token string) (*websocket.Conn, string, error) {
	if strings.TrimSpace(token) == "" {
		return nil, "", errors.New("tunnel token is required")
	}
	endpoint, err := ConnectURL(serverURL)
	if err != nil {
		return nil, "", err
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+token)
	connection, response, err := (&websocket.Dialer{HandshakeTimeout: 15 * time.Second}).DialContext(ctx, endpoint, header)
	if err == nil {
		var name string
		if response != nil {
			name = response.Header.Get(tunnelNameHeader)
		}
		if name != "" {
			if err := apitypes.ValidateTunnelName(name); err != nil {
				_ = connection.Close()
				return nil, "", fmt.Errorf("server returned an invalid tunnel name: %w", err)
			}
		}
		return connection, name, nil
	}
	if response == nil {
		return nil, "", fmt.Errorf("failed to connect tunnel: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = response.Status
	}
	return nil, "", fmt.Errorf("failed to connect tunnel: %s: %s", response.Status, message)
}

// Run keeps a tunnel connected until ctx is canceled. Failed handshakes and
// lost connections are retried with bounded exponential backoff.
func Run(ctx context.Context, serverURL, token string) error {
	if _, err := ConnectURL(serverURL); err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("tunnel token is required")
	}

	backoff := time.Second
	for {
		connectionLog := tunnelLog.Fields()
		connection, name, err := dial(ctx, serverURL, token)
		if err == nil {
			if name != "" {
				connectionLog = connectionLog.Fields("tunnel", name)
			}
			connectionLog.Infof("Tunnel connected")
			backoff = time.Second
			err = serveConnection(ctx, connection, name)
			_ = connection.Close()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		connectionLog.Warnf("Tunnel disconnected, retrying in %s: %v", backoff, err)

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func serveConnection(ctx context.Context, connection *websocket.Conn, name string) error {
	return serveConnectionWithClient(ctx, connection, name, newForwardHTTPClient())
}

func serveConnectionWithClient(ctx context.Context, connection *websocket.Conn, name string, client *http.Client) error {
	defer connection.Close()
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	forwarder := &clientForwarder{
		client: client,
		name:   name,
	}
	authorize := func(network, address string) bool {
		return network == forwardNetwork && address == forwardAddress ||
			network == disconnectNetwork && address == disconnectAddress
	}
	localDialer := func(_ context.Context, network, address string) (net.Conn, error) {
		switch {
		case network == forwardNetwork && address == forwardAddress:
			forwardCtx, cancelForward := context.WithCancel(sessionCtx)
			clientConnection, handlerConnection := net.Pipe()
			go func() {
				if err := forwarder.serve(forwardCtx, handlerConnection); err != nil &&
					!errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
					tunnelLog.Debugf("Tunnel forwarding connection closed: %v", err)
				}
				cancelForward()
			}()
			return &cancelOnCloseConn{Conn: clientConnection, cancel: cancelForward}, nil
		case network == disconnectNetwork && address == disconnectAddress:
			clientConnection, handlerConnection := net.Pipe()
			go disconnectOnClose(handlerConnection, cancelSession)
			return clientConnection, nil
		default:
			return nil, fmt.Errorf("tunnel connection to %s/%s is not allowed", network, address)
		}
	}

	session := remotedialer.NewClientSessionWithDialer(authorize, connection, localDialer)
	defer session.Close()

	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-sessionCtx.Done():
			deadline := time.Now().Add(websocketCloseTimeout)
			if err := connection.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				deadline,
			); err != nil {
				_ = connection.Close()
				return
			}

			timer := time.NewTimer(time.Until(deadline))
			defer timer.Stop()
			select {
			case <-done:
			case <-timer.C:
				_ = connection.Close()
			}
		case <-done:
		}
	}()
	defer func() {
		close(done)
		<-shutdownDone
	}()

	_, err := session.Serve(sessionCtx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if sessionCtx.Err() != nil {
		return errors.New("tunnel disconnected by server")
	}
	return err
}

func disconnectOnClose(connection net.Conn, cancelSession context.CancelFunc) {
	defer connection.Close()
	_, _ = io.Copy(io.Discard, connection)
	cancelSession()
}

type cancelOnCloseConn struct {
	net.Conn
	cancel context.CancelFunc
}

func (c *cancelOnCloseConn) Close() error {
	c.cancel()
	return c.Conn.Close()
}

func newForwardHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// Let the bridge rewrite Location so redirects remain inside the tunnel.
			return http.ErrUseLastResponse
		},
	}
}

type clientForwarder struct {
	client        *http.Client
	name          string
	nextRequestID atomic.Uint64
}

func (c *clientForwarder) serve(ctx context.Context, connection net.Conn) error {
	request, err := http.ReadRequest(bufio.NewReader(connection))
	if err != nil {
		_ = connection.Close()
		return fmt.Errorf("failed to read tunnel request: %w", err)
	}
	defer func() {
		// Closing an HTTP request body may drain unread upload data. Close this
		// one-request stream first so an early target response cannot leave the
		// tunnel waiting forever for the caller to finish uploading.
		_ = connection.Close()
		_ = request.Body.Close()
	}()

	encodedTarget := request.Header.Get(forwardTargetHeader)
	targetBytes, err := base64.RawURLEncoding.DecodeString(encodedTarget)
	if err != nil {
		return c.writeError(connection, errors.New("tunnel request has an invalid target"))
	}
	target, err := parseTargetURL(string(targetBytes))
	if err != nil {
		return c.writeError(connection, errors.New("tunnel request has an invalid target URL"))
	}

	requestID := c.nextRequestID.Add(1)
	started := time.Now()
	tunnelName := request.Header.Get(tunnelNameHeader)
	if err := apitypes.ValidateTunnelName(tunnelName); err != nil {
		tunnelName = c.name
	}
	request.RequestURI = ""
	request.URL = target
	request.Host = target.Host
	request.Close = false
	request.Header.Del(forwardTargetHeader)
	request.Header.Del(forwardErrorHeader)
	request.Header.Del(tunnelNameHeader)
	removeHopHeaders(request.Header)
	request = request.WithContext(ctx)

	requestLog := tunnelLog.Fields(
		"request_id", requestID,
		"method", request.Method,
		"url", tunnelRequestLogURL(request.URL),
		"has_query", request.URL.RawQuery != "",
		"request_content_length", request.ContentLength,
	)
	if tunnelName != "" {
		requestLog = requestLog.Fields("tunnel", tunnelName)
	}
	requestLog.Infof("Tunnel request received")

	response, err := c.client.Do(request)
	if err != nil {
		requestLog = requestLog.Fields("duration", time.Since(started))
		if errors.Is(err, context.Canceled) {
			requestLog.Infof("Tunnel request canceled")
		} else {
			requestLog.Warnf("Tunnel request failed: %v", tunnelRequestLogError(err))
		}
		return c.writeError(connection, err)
	}
	defer response.Body.Close()
	requestLog.Fields(
		"status", response.StatusCode,
		"response_content_length", response.ContentLength,
		"duration", time.Since(started),
	).Infof("Tunnel response received")

	response.Header = response.Header.Clone()
	response.Header.Del(forwardTargetHeader)
	response.Header.Del(forwardErrorHeader)
	response.Header.Del(tunnelNameHeader)
	removeHopHeaders(response.Header)
	response.Close = true
	return response.Write(connection)
}

func (c *clientForwarder) writeError(connection net.Conn, cause error) error {
	response := &http.Response{
		StatusCode:    http.StatusBadGateway,
		Status:        fmt.Sprintf("%d %s", http.StatusBadGateway, http.StatusText(http.StatusBadGateway)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          http.NoBody,
		ContentLength: 0,
		Close:         true,
	}
	response.Header.Set(forwardErrorHeader, base64.RawURLEncoding.EncodeToString([]byte(cause.Error())))
	return response.Write(connection)
}

func tunnelRequestLogURL(target *url.URL) string {
	if target == nil {
		return ""
	}
	logURL := *target
	logURL.RawQuery = ""
	logURL.ForceQuery = false
	logURL.Fragment = ""
	return logURL.String()
}

func tunnelRequestLogError(err error) error {
	var urlError *url.Error
	if errors.As(err, &urlError) && urlError.Err != nil {
		return urlError.Err
	}
	return err
}
