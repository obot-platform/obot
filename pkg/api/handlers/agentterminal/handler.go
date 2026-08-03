package agentterminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentbackend"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	// writeWait bounds a single write so one stalled browser cannot hold a
	// sandbox attachment open indefinitely.
	writeWait = 10 * time.Second
	// pingPeriod detects a browser that vanished without closing. A console can
	// be idle for a long time, so silence alone is not a signal.
	pingPeriod = 25 * time.Second
	// pingWait is how long a browser has to answer before it is considered gone.
	pingWait = 20 * time.Second

	readBufferSize = 32 * 1024
	// readLimit caps a single inbound frame. Keystrokes and resize messages are
	// tiny; anything approaching this is not a terminal client.
	readLimit = 64 * 1024
)

type Handler struct {
	backend agentbackend.InstanceBackend
	// devOrigins are additional origins permitted to open a terminal. It is
	// empty outside development: the upgrade is authorized by the session
	// cookie, so accepting a foreign origin would let any page a user visits
	// open a shell in their sandbox.
	devOrigins []string
}

// New builds the terminal handler. devUIPort is the port a separate dev UI
// server runs on, or 0 in production. The dev server proxies /api with
// changeOrigin, which rewrites Host but leaves Origin pointing at the dev
// server, so without this the same-origin check rejects every upgrade during
// `make dev`.
func New(backend agentbackend.InstanceBackend, devUIPort int) *Handler {
	var devOrigins []string
	if devUIPort > 0 {
		devOrigins = []string{
			fmt.Sprintf("localhost:%d", devUIPort),
			fmt.Sprintf("127.0.0.1:%d", devUIPort),
		}
	}
	return &Handler{backend: backend, devOrigins: devOrigins}
}

// Attach upgrades to a websocket and joins the sandbox's console.
//
// Authorization has already happened: the route is registered against the
// instance, and checkHostedAgentInstance narrows it to the owner. The websocket
// carries the session cookie, since a browser cannot set an Authorization
// header on an upgrade request, so no separate token scheme is needed. That is
// also why the upgrade is restricted to same-origin requests.
func (h *Handler) Attach(req api.Context) error {
	var instance v1.HostedAgentInstance
	if err := req.Get(&instance, req.PathValue("hosted_agent_instance_id")); err != nil {
		return err
	}

	var agent v1.HostedAgent
	if err := req.Get(&agent, instance.Spec.HostedAgentName); err != nil {
		return err
	}
	// The terminal is an administrator's decision about the agent, so an
	// instance of an agent that does not offer one has no console to attach to
	// regardless of what the sandbox is running.
	if !agent.Spec.Manifest.Terminal {
		return types.NewErrBadRequest("agent %s does not offer a terminal", agent.Name)
	}

	terminals, ok := h.backend.(agentbackend.TerminalBackend)
	if !ok {
		return types.NewErrBadRequest("the configured agent runtime does not support terminals")
	}

	// Accept rejects cross-origin upgrades by default, which is what this
	// connection needs: it is authorized by the session cookie, so a page on
	// another origin must not be able to open one. OriginPatterns adds the dev
	// UI server and nothing else, and is empty in production.
	conn, err := websocket.Accept(req.ResponseWriter, req.Request, &websocket.AcceptOptions{
		OriginPatterns: h.devOrigins,
	})
	if err != nil {
		// Accept has already written its own response.
		return nil
	}
	// A no-op once the session has closed cleanly; this is the backstop for
	// every path that has not.
	defer func() { _ = conn.CloseNow() }()

	conn.SetReadLimit(readLimit)

	// Upgrade first, then attach: a failed attach is reported on the connection
	// where the browser can display it, rather than as an HTTP error a
	// websocket client never sees.
	session, err := terminals.AttachTerminal(req.Context(), instanceRef(&instance), initialSize(req))
	if err != nil {
		writeSessionError(req.Context(), conn, err.Error())
		return nil
	}
	defer session.Close()

	(&pump{conn: conn, session: session}).run(req.Context())
	return nil
}

// pump moves bytes in both directions.
//
// All writes happen on the single goroutine running writeToBrowser, which is
// why no lock is needed: a websocket permits one writer at a time, and
// concentrating writes in one place is simpler than serialising them.
type pump struct {
	conn    *websocket.Conn
	session agentbackend.TerminalSession
}

func (p *pump) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go p.readFromBrowser(ctx, cancel)
	p.writeToBrowser(ctx)
}

// readFromBrowser carries keystrokes and control messages toward the sandbox.
func (p *pump) readFromBrowser(ctx context.Context, cancel context.CancelFunc) {
	// Cancelling stops the writer too, so a closed browser tears the whole
	// session down rather than leaving output accumulating with no reader.
	defer cancel()

	for {
		_, message, err := p.conn.Read(ctx)
		if err != nil {
			return
		}

		channel, payload, ok := ParseFrame(message)
		if !ok {
			continue
		}

		switch channel {
		case ChannelStdin:
			if _, err := p.session.Write(payload); err != nil {
				return
			}
		case ChannelControl:
			var control ControlMessage
			if err := json.Unmarshal(payload, &control); err != nil {
				continue
			}
			if control.Type == ControlResize && control.Cols > 0 && control.Rows > 0 {
				// A failed resize is cosmetic; the session continues at its
				// previous size rather than being torn down.
				_ = p.session.Resize(agentbackend.TerminalSize{Rows: control.Rows, Cols: control.Cols})
			}
		}
	}
}

// writeToBrowser carries console output, and pings so a vanished browser is
// noticed even while the console is silent.
func (p *pump) writeToBrowser(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	output := make(chan []byte, 16)
	readErr := make(chan error, 1)
	go func() {
		defer close(output)
		buffer := make([]byte, readBufferSize)
		for {
			n, err := p.session.Read(buffer)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buffer[:n])
				output <- chunk
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, pingWait)
			err := p.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		case chunk, ok := <-output:
			if !ok {
				// Report why the console ended, then close cleanly, so the
				// browser can tell this from a dropped connection. Both are
				// best effort: the session is already over.
				if err := <-readErr; err != nil && !errors.Is(err, io.EOF) {
					writeSessionError(ctx, p.conn, err.Error())
					return
				}
				_ = p.conn.Close(websocket.StatusNormalClosure, "session ended")
				return
			}
			if err := p.writeFrame(ctx, ChannelStdout, chunk); err != nil {
				return
			}
		}
	}
}

func (p *pump) writeFrame(ctx context.Context, channel byte, payload []byte) error {
	writeCtx, cancel := context.WithTimeout(ctx, writeWait)
	defer cancel()
	return p.conn.Write(writeCtx, websocket.MessageBinary, Frame(channel, payload))
}

// writeSessionError reports a problem with the session rather than with the
// program running in it, so it travels on the control channel. The output
// channels are left to carry only what the sandbox produced.
func writeSessionError(ctx context.Context, conn *websocket.Conn, message string) {
	writeCtx, cancel := context.WithTimeout(ctx, writeWait)
	defer cancel()
	if frame, err := ControlFrame(ControlMessage{Type: ControlError, Message: message}); err == nil {
		_ = conn.Write(writeCtx, websocket.MessageBinary, frame)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "terminal unavailable")
}

// initialSize takes the browser's terminal dimensions from the query so the
// first thing the program draws already fits.
func initialSize(req api.Context) agentbackend.TerminalSize {
	size := agentbackend.TerminalSize{Rows: 24, Cols: 80}
	if rows, err := strconv.ParseUint(req.URL.Query().Get("rows"), 10, 16); err == nil && rows > 0 {
		size.Rows = uint16(rows)
	}
	if cols, err := strconv.ParseUint(req.URL.Query().Get("cols"), 10, 16); err == nil && cols > 0 {
		size.Cols = uint16(cols)
	}
	return size
}

func instanceRef(instance *v1.HostedAgentInstance) agentbackend.InstanceRef {
	id := string(instance.UID)
	if id == "" {
		id = instance.Name
	}
	return agentbackend.InstanceRef{
		ID:        id,
		Namespace: instance.Namespace,
		UserID:    instance.Spec.UserID,
		BackendID: instance.Status.BackendID,
	}
}
