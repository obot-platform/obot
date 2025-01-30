package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/system"
)

var log = logger.Package()

type ShellHandler struct {
	docker *client.Client
	invoke *invoke.Invoker
}

func NewShellHandler(invoke *invoke.Invoker) (*ShellHandler, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &ShellHandler{
		docker: docker,
		invoke: invoke,
	}, nil
}

func (h *ShellHandler) Shell(req api.Context) error {
	if req.Request.Header.Get("Connection") != "Upgrade" {
		return types.NewErrBadRequest("Upgrade header missing")
	}

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	output, err := h.invoke.EphemeralThreadTask(req.Context(), thread, system.DockerShellIDTool, map[string]any{
		"START": "false",
	})
	if err != nil {
		return err
	}

	id := strings.TrimSpace(output)

	stream, err := h.docker.ContainerAttach(req.Context(), id, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return err
	}

	if err := h.docker.ContainerStart(req.Context(), id, container.StartOptions{}); err != nil {
		return err
	}

	u := &websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	wsConn, err := u.Upgrade(req.ResponseWriter, req.Request, nil)
	if err != nil {
		return err
	}
	defer wsConn.Close()

	go func() {
		ws, err := h.docker.ContainerWait(req.Context(), id, container.WaitConditionNextExit)
		select {
		case <-ws:
		case err := <-err:
			log.Debugf("error waiting for container: %v", err)
		}
		wsConn.Close()
	}()

	go func() {
		defer wsConn.Close()
		_, err = io.Copy(wsWriter{wsConn: wsConn}, stream.Reader)
		if err != nil {
			log.Debugf("error copying from stream to wsConn: %v", err)
		}
	}()

	for {
		_, data, err := wsConn.ReadMessage()
		if err != nil {
			log.Debugf("error reading from wsConn: %v", err)
			// no point in returning error, the connection is already hijacked
			return nil
		}

		if len(data) == 0 {
			continue
		}

		if data[0] == 1 {
			var resize struct {
				Cols uint `json:"cols"`
				Rows uint `json:"rows"`
			}
			if err := json.Unmarshal(data[1:], &resize); err != nil {
				log.Errorf("error unmarshalling resize message: %v", err)
				continue
			}
			if resize.Cols == 0 || resize.Rows == 0 {
				continue
			}
			_ = h.docker.ContainerResize(req.Context(), id, container.ResizeOptions{
				Height: resize.Rows,
				Width:  resize.Cols,
			})
			continue
		}

		_, err = stream.Conn.Write(data[1:])
		if err != nil {
			log.Debugf("error writing to stream: %v", err)
			// no point in returning error, the connection is already hijacked
			return nil
		}
	}
}

type wsWriter struct {
	wsConn *websocket.Conn
}

func (w wsWriter) Write(p []byte) (n int, err error) {
	err = w.wsConn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
