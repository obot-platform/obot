package mcp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type recoveryConnectKey struct{}

type recoveryAttemptKey struct{}

type recoveryAttempt struct {
	sent     atomic.Bool
	rejected atomic.Bool
}

type recoverableSession struct {
	*gomcp.ClientSession
	missing atomic.Bool
}

type sessionReconnect struct {
	done              chan struct{}
	err               error
	initiatorCanceled bool
}

type sessionTransport struct {
	base    http.RoundTripper
	missing *atomic.Bool
}

// sessionRecovery owns the connection; the embedded SDK reference never changes.
// Retained references and iterators reach the current connection through middleware.
type sessionRecovery struct {
	mu        sync.Mutex
	current   *recoverableSession
	pending   *sessionReconnect
	connect   func(context.Context) (*recoverableSession, error)
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	done      chan struct{}
	closeErr  error
}

func newRecoveringClient(ctx context.Context, sdk *gomcp.Client, transport gomcp.StreamableClientTransport) (*Client, error) {
	r := &sessionRecovery{done: make(chan struct{})}
	r.ctx, r.cancel = context.WithCancel(context.Background())
	var connectOpts *gomcp.ClientSessionOptions
	r.connect = func(ctx context.Context) (*recoverableSession, error) {
		state := &recoverableSession{}
		httpClient := transport.HTTPClient
		if httpClient == nil {
			httpClient = http.DefaultClient
		}

		clientCopy := *httpClient
		clientCopy.Transport = &sessionTransport{base: clientCopy.Transport, missing: &state.missing}
		transportCopy := transport
		transportCopy.HTTPClient = &clientCopy

		session, err := sdk.Connect(context.WithValue(ctx, recoveryConnectKey{}, r), &transportCopy, connectOpts)
		if err != nil {
			return nil, err
		}
		state.ClientSession = session
		return state, nil
	}
	sdk.AddSendingMiddleware(r.middleware)

	session, err := r.connect(ctx)
	if err != nil {
		r.cancel()
		return nil, err
	}
	r.current = session
	// Stateful HTTP uses the legacy protocol, without SDK list caches or
	// pre-middleware metadata injection. Keep that protocol on reconnect so
	// retained SDK references have the same dispatch behavior. Modern stateless
	// clients keep normal SDK caching and never replay a sessionless 404.
	connectOpts = &gomcp.ClientSessionOptions{ProtocolVersion: session.InitializeResult().ProtocolVersion}

	return &Client{ClientSession: session.ClientSession, recovery: r}, nil
}

func (r *sessionRecovery) middleware(next gomcp.MethodHandler) gomcp.MethodHandler {
	return func(ctx context.Context, method string, req gomcp.Request) (gomcp.Result, error) {
		// Connect's initialize and initialized messages must use their new session.
		if ctx.Value(recoveryConnectKey{}) == r {
			return next(ctx, method, req)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if r.ctx.Err() != nil {
			return nil, gomcp.ErrConnectionClosed
		}

		// Closing the owner must also release in-flight calls before SDK Close waits.
		ctx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(r.ctx, cancel)
		if method == "subscriptions/listen" {
			// The SDK returns after starting the stream; Unsubscribe or Close
			// ends its context, not the return from the sending handler.
			context.AfterFunc(ctx, func() { stop() })
		} else {
			defer func() {
				stop()
				cancel()
			}()
		}

		r.mu.Lock()
		session := r.current
		r.mu.Unlock()
		if session.missing.Load() {
			var err error
			session, err = r.reconnect(ctx, session)
			if err != nil {
				return nil, err
			}
		}

		for attempt := 0; ; attempt++ {
			if r.ctx.Err() != nil {
				return nil, gomcp.ErrConnectionClosed
			}
			// Replay only this request: server-held subscriptions and cursors are
			// not recreated, and a rejected continuation cursor remains an error.
			requestAttempt := &recoveryAttempt{}
			attemptCtx := context.WithValue(ctx, recoveryAttemptKey{}, requestAttempt)
			result, err := next(attemptCtx, method, &gomcp.ClientRequest[gomcp.Params]{Session: session.ClientSession, Params: req.GetParams()})
			if errors.Is(err, gomcp.ErrSessionMissing) && session.ID() != "" {
				session.missing.Store(true)
				// The SDK has now recorded session loss, so Close cannot send DELETE.
				_ = session.Close()
			}
			// The SDK broadcasts session loss to every pending call, including
			// accepted operations. Replay only this POST's rejection or an unsent call.
			missing := session.missing.Load() && (errors.Is(err, gomcp.ErrSessionMissing) || errors.Is(err, gomcp.ErrConnectionClosed))
			safeToReplay := requestAttempt.rejected.Load() || !requestAttempt.sent.Load()
			if !missing || !safeToReplay || attempt == 1 || ctx.Err() != nil {
				return result, err
			}

			session, err = r.reconnect(ctx, session)
			if err != nil {
				return nil, err
			}
		}
	}
}

func (r *sessionRecovery) reconnect(ctx context.Context, stale *recoverableSession) (*recoverableSession, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		r.mu.Lock()
		if r.ctx.Err() != nil {
			r.mu.Unlock()
			return nil, gomcp.ErrConnectionClosed
		}
		if r.current != stale {
			current := r.current
			r.mu.Unlock()
			return current, nil
		}
		if pending := r.pending; pending != nil {
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-r.ctx.Done():
				return nil, gomcp.ErrConnectionClosed
			case <-pending.done:
				// A live waiter can replace a reconnect abandoned by its initiator.
				if pending.initiatorCanceled {
					continue
				}
				if pending.err != nil {
					return nil, pending.err
				}
				r.mu.Lock()
				current := r.current
				r.mu.Unlock()
				return current, nil
			}
		}
		pending := &sessionReconnect{done: make(chan struct{})}
		r.pending = pending
		r.mu.Unlock()

		// Bound initialization independently of potentially long-running tool calls.
		connectCtx, cancel := context.WithTimeout(ctx, time.Minute)
		session, err := r.connect(connectCtx)
		cancel()

		r.mu.Lock()
		closed := r.ctx.Err() != nil
		if closed {
			err = gomcp.ErrConnectionClosed
		} else if err == nil {
			r.current = session
		}
		pending.err = err
		// Only the caller's cancellation permits a waiter to start another attempt;
		// the shared initialization timeout must be returned to all waiters.
		pending.initiatorCanceled = ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
		r.pending = nil
		close(pending.done)
		r.mu.Unlock()
		if closed && session != nil {
			_ = session.Close()
		}

		return session, err
	}
}

func (c *Client) Close() error {
	r := c.recovery
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.cancel()
		session := r.current
		r.mu.Unlock()

		r.closeErr = session.Close()
		close(r.done)
	})
	return r.closeErr
}

// Wait follows the cached client's lifetime, rather than an obsolete connection.
func (c *Client) Wait() error {
	<-c.recovery.done
	return c.recovery.closeErr
}

func (c *Client) ID() string {
	c.recovery.mu.Lock()
	defer c.recovery.mu.Unlock()
	return c.recovery.current.ID()
}

func (c *Client) InitializeResult() *gomcp.InitializeResult {
	c.recovery.mu.Lock()
	defer c.recovery.mu.Unlock()
	return c.recovery.current.InitializeResult()
}

func (t *sessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempt, _ := req.Context().Value(recoveryAttemptKey{}).(*recoveryAttempt)
	if attempt != nil && req.Method == http.MethodPost {
		attempt.sent.Store(true)
	}

	if req.Method == http.MethodDelete {
		// Close may race the SDK recording a 404; never DELETE a known lost session.
		if t.missing.Load() {
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
		}
		// The SDK uses a detached context for DELETE; bound graceful shutdown.
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		req = req.Clone(ctx)
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Preserve the 404 response so the SDK records ErrSessionMissing internally.
	// A JSON-RPC error body would otherwise bypass that SDK failure state.
	if req.Method == http.MethodPost && req.Header.Get("Mcp-Session-Id") != "" && resp.StatusCode == http.StatusNotFound {
		if attempt != nil {
			attempt.rejected.Store(true)
		}
		t.missing.Store(true)
		_ = resp.Body.Close()
		resp.Body = http.NoBody
		resp.ContentLength = 0
		resp.Header.Del("Content-Length")
	}

	return resp, nil
}
