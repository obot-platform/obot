package tunnel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestTunnelStreamsResponseWhileChunkedRequestRemainsOpen(t *testing.T) {
	const (
		requestBody    = "request body"
		firstEvent     = "data: first\n\n"
		secondEvent    = "data: second\n\n"
		requestTimeout = 3 * time.Second
	)

	firstEventFlushed := make(chan struct{})
	targetSetupError := make(chan error, 1)
	requestReceived := make(chan struct {
		body []byte
		err  error
	}, 1)
	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
			targetSetupError <- err
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, firstEvent)
		w.(http.Flusher).Flush()
		close(firstEventFlushed)

		body, err := io.ReadAll(r.Body)
		requestReceived <- struct {
			body []byte
			err  error
		}{body: body, err: err}

		_, _ = io.WriteString(w, secondEvent)
		w.(http.Flusher).Flush()
	}))
	defer target.Close()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	bridgeURL, err := manager.BridgeURL("office", target.URL+"/duplex")
	if err != nil {
		t.Fatal(err)
	}
	bodyReader, bodyWriter := io.Pipe()
	defer bodyWriter.Close()
	request, err := http.NewRequest(http.MethodPost, bridgeURL, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	request.ContentLength = -1

	responseResult := make(chan struct {
		response *http.Response
		err      error
	}, 1)
	go func() {
		response, err := bridgeClient.Do(request)
		responseResult <- struct {
			response *http.Response
			err      error
		}{response: response, err: err}
	}()

	uploadResult := make(chan error, 1)
	go func() {
		_, err := io.WriteString(bodyWriter, requestBody)
		uploadResult <- err
	}()

	select {
	case <-firstEventFlushed:
	case err := <-targetSetupError:
		t.Fatalf("enabling target full duplex: %v", err)
	case <-time.After(requestTimeout):
		t.Fatal("target did not flush a response while the request remained open")
	}

	select {
	case err := <-uploadResult:
		if err != nil {
			t.Fatalf("upload error = %v", err)
		}
	case <-time.After(requestTimeout):
		t.Fatal("request upload did not reach the target")
	}

	var response *http.Response
	select {
	case result := <-responseResult:
		if result.err != nil {
			t.Fatalf("bridge request error = %v", result.err)
		}
		response = result.response
	case <-time.After(requestTimeout):
		t.Fatal("bridge withheld the response until request EOF")
	}
	defer response.Body.Close()

	first := make([]byte, len(firstEvent))
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatalf("reading first response event before request EOF: %v", err)
	}
	if !bytes.Equal(first, []byte(firstEvent)) {
		t.Fatalf("first response event = %q, want %q", first, firstEvent)
	}

	if err := bodyWriter.Close(); err != nil {
		t.Fatalf("closing request body: %v", err)
	}
	select {
	case received := <-requestReceived:
		if received.err != nil {
			t.Fatalf("target request body error = %v", received.err)
		}
		if !bytes.Equal(received.body, []byte(requestBody)) {
			t.Fatalf("target request body = %q, want %q", received.body, requestBody)
		}
	case <-time.After(requestTimeout):
		t.Fatal("target did not observe request EOF")
	}

	remainder, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading response after request EOF: %v", err)
	}
	if !bytes.Equal(remainder, []byte(secondEvent)) {
		t.Fatalf("remaining response = %q, want %q", remainder, secondEvent)
	}
}

func TestEarlyTunnelResponseStopsOpenUploadAndKeepsSession(t *testing.T) {
	const requestTimeout = 3 * time.Second

	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/early":
			if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "early")
		case "/health":
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	bridgeURL, err := manager.BridgeURL("office", target.URL+"/early")
	if err != nil {
		t.Fatal(err)
	}
	bodyReader, bodyWriter := io.Pipe()
	defer bodyWriter.Close()
	request, err := http.NewRequest(http.MethodPost, bridgeURL, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	request.ContentLength = -1

	uploadResult := make(chan error, 1)
	go func() {
		_, err := io.WriteString(bodyWriter, "partial request")
		uploadResult <- err
	}()

	responseResult := make(chan struct {
		response *http.Response
		err      error
	}, 1)
	go func() {
		response, err := bridgeClient.Do(request)
		responseResult <- struct {
			response *http.Response
			err      error
		}{response: response, err: err}
	}()

	var response *http.Response
	select {
	case result := <-responseResult:
		if result.err != nil {
			t.Fatalf("bridge request error = %v", result.err)
		}
		response = result.response
	case <-time.After(requestTimeout):
		t.Fatal("bridge withheld the early response until request EOF")
	}
	select {
	case err := <-uploadResult:
		if err != nil {
			t.Fatalf("initial upload error = %v", err)
		}
	case <-time.After(requestTimeout):
		t.Fatal("initial request upload remained blocked")
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("reading early response: %v", err)
	}
	if !bytes.Equal(body, []byte("early")) {
		t.Fatalf("early response = %q, want %q", body, "early")
	}

	uploadStopped := make(chan error, 1)
	go func() {
		chunk := bytes.Repeat([]byte("x"), 32*1024)
		for {
			if _, err := bodyWriter.Write(chunk); err != nil {
				uploadStopped <- err
				return
			}
		}
	}()
	select {
	case err := <-uploadStopped:
		if err == nil {
			t.Fatal("open request upload stopped without an error")
		}
	case <-time.After(requestTimeout):
		t.Fatal("request upload remained blocked after the target completed its response")
	}

	assertTunnelHealth(t, manager, bridgeClient, target.URL+"/health")
}

func TestTunnelChunkedRequestEOFAllowsStreamingResponse(t *testing.T) {
	const (
		requestBody    = "request body"
		firstEvent     = "data: first\n\n"
		secondEvent    = "data: second\n\n"
		requestTimeout = 3 * time.Second
	)

	type receivedRequest struct {
		body             []byte
		err              error
		contentLength    int64
		transferEncoding []string
	}

	requestReceived := make(chan receivedRequest, 1)
	firstEventFlushed := make(chan struct{})
	releaseSecondEvent := make(chan struct{})
	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		requestReceived <- receivedRequest{
			body:             body,
			err:              err,
			contentLength:    r.ContentLength,
			transferEncoding: append([]string(nil), r.TransferEncoding...),
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, firstEvent)
		w.(http.Flusher).Flush()
		close(firstEventFlushed)

		<-releaseSecondEvent
		_, _ = io.WriteString(w, secondEvent)
		w.(http.Flusher).Flush()
	}))
	defer target.Close()
	defer func() {
		select {
		case <-releaseSecondEvent:
		default:
			close(releaseSecondEvent)
		}
	}()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	bridgeURL, err := manager.BridgeURL("office", target.URL+"/stream")
	if err != nil {
		t.Fatal(err)
	}
	bodyReader, bodyWriter := io.Pipe()
	request, err := http.NewRequest(http.MethodPost, bridgeURL, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	request.ContentLength = -1

	responseResult := make(chan struct {
		response *http.Response
		err      error
	}, 1)
	go func() {
		response, err := bridgeClient.Do(request)
		responseResult <- struct {
			response *http.Response
			err      error
		}{response: response, err: err}
	}()

	uploadResult := make(chan error, 1)
	go func() {
		_, err := io.WriteString(bodyWriter, requestBody)
		if closeErr := bodyWriter.Close(); err == nil {
			err = closeErr
		}
		uploadResult <- err
	}()

	select {
	case received := <-requestReceived:
		if received.err != nil {
			t.Fatalf("target request body error = %v", received.err)
		}
		if !bytes.Equal(received.body, []byte(requestBody)) {
			t.Fatalf("target request body = %q, want %q", received.body, requestBody)
		}
		if received.contentLength != -1 {
			t.Fatalf("target ContentLength = %d, want -1", received.contentLength)
		}
		if len(received.transferEncoding) != 1 || received.transferEncoding[0] != "chunked" {
			t.Fatalf("target TransferEncoding = %#v, want [chunked]", received.transferEncoding)
		}
	case <-time.After(requestTimeout):
		t.Fatal("target did not observe request body EOF")
	}

	select {
	case err := <-uploadResult:
		if err != nil {
			t.Fatalf("upload error = %v", err)
		}
	case <-time.After(requestTimeout):
		t.Fatal("request upload did not complete")
	}

	select {
	case <-firstEventFlushed:
	case <-time.After(requestTimeout):
		t.Fatal("target did not flush the first response event")
	}

	var response *http.Response
	select {
	case result := <-responseResult:
		if result.err != nil {
			t.Fatalf("bridge request error = %v", result.err)
		}
		response = result.response
	case <-time.After(requestTimeout):
		t.Fatal("bridge did not return the streaming response")
	}
	defer response.Body.Close()

	firstRead := make(chan struct {
		body []byte
		err  error
	}, 1)
	go func() {
		body := make([]byte, len(firstEvent))
		_, err := io.ReadFull(response.Body, body)
		firstRead <- struct {
			body []byte
			err  error
		}{body: body, err: err}
	}()

	select {
	case result := <-firstRead:
		if result.err != nil {
			t.Fatalf("reading first response event: %v", result.err)
		}
		if !bytes.Equal(result.body, []byte(firstEvent)) {
			t.Fatalf("first response event = %q, want %q", result.body, firstEvent)
		}
	case <-time.After(requestTimeout):
		t.Fatal("first response event did not arrive while the response remained open")
	}

	close(releaseSecondEvent)
	remainder, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading remaining response: %v", err)
	}
	if !bytes.Equal(remainder, []byte(secondEvent)) {
		t.Fatalf("remaining response = %q, want %q", remainder, secondEvent)
	}
}

func TestClosingTunnelResponseCancelsTargetAndKeepsSession(t *testing.T) {
	const (
		firstEvent     = "data: first\n\n"
		requestTimeout = 3 * time.Second
	)

	targetCanceled := make(chan error, 1)
	stopTarget := make(chan struct{})
	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, firstEvent)
			w.(http.Flusher).Flush()

			select {
			case <-r.Context().Done():
				targetCanceled <- r.Context().Err()
			case <-stopTarget:
			}
		case "/health":
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		close(stopTarget)
		target.Close()
	}()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	streamURL, err := manager.BridgeURL("office", target.URL+"/stream")
	if err != nil {
		t.Fatal(err)
	}
	response, err := bridgeClient.Get(streamURL)
	if err != nil {
		t.Fatalf("opening streaming response: %v", err)
	}

	first := make([]byte, len(firstEvent))
	if _, err := io.ReadFull(response.Body, first); err != nil {
		response.Body.Close()
		t.Fatalf("reading first response event: %v", err)
	}
	if !bytes.Equal(first, []byte(firstEvent)) {
		response.Body.Close()
		t.Fatalf("first response event = %q, want %q", first, firstEvent)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("closing streaming response: %v", err)
	}

	select {
	case err := <-targetCanceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("target context error = %v, want context canceled", err)
		}
	case <-time.After(requestTimeout):
		t.Fatal("target context was not canceled after the caller closed the response")
	}

	assertTunnelHealth(t, manager, bridgeClient, target.URL+"/health")
}

func TestTruncatedTunnelResponseDoesNotHangOrDropSession(t *testing.T) {
	const (
		partialBody    = "short"
		contentLength  = 20
		requestTimeout = 3 * time.Second
	)

	target := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/truncated":
			w.Header().Set("Content-Length", "20")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, partialBody)
		case "/health":
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	manager, bridgeClient, cleanup := newConnectedTestTunnel(t, "office")
	defer cleanup()

	truncatedURL, err := manager.BridgeURL("office", target.URL+"/truncated")
	if err != nil {
		t.Fatal(err)
	}
	response, err := bridgeClient.Get(truncatedURL)
	if err != nil {
		t.Fatalf("opening truncated response: %v", err)
	}
	defer response.Body.Close()
	if response.ContentLength != contentLength {
		t.Fatalf("response ContentLength = %d, want %d", response.ContentLength, contentLength)
	}

	readResult := make(chan struct {
		body []byte
		err  error
	}, 1)
	go func() {
		body, err := io.ReadAll(response.Body)
		readResult <- struct {
			body []byte
			err  error
		}{body: body, err: err}
	}()

	select {
	case result := <-readResult:
		if !errors.Is(result.err, io.ErrUnexpectedEOF) {
			t.Fatalf("truncated response error = %v, want unexpected EOF", result.err)
		}
		if !bytes.Equal(result.body, []byte(partialBody)) {
			t.Fatalf("truncated response body = %q, want %q", result.body, partialBody)
		}
	case <-time.After(requestTimeout):
		t.Fatal("reading the truncated response hung")
	}

	assertTunnelHealth(t, manager, bridgeClient, target.URL+"/health")
}

func assertTunnelHealth(t *testing.T, manager *Manager, client *http.Client, targetURL string) {
	t.Helper()

	bridgeURL, err := manager.BridgeURL("office", targetURL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(bridgeURL)
	if err != nil {
		t.Fatalf("tunnel health request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading tunnel health response: %v", err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Equal(body, []byte("ok")) {
		t.Fatalf("tunnel health response = %d %q, want 200 %q", response.StatusCode, body, "ok")
	}
}
