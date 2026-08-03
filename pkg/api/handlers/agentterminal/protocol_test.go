package agentterminal

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte("ls -la\r")

	channel, got, ok := ParseFrame(Frame(ChannelStdin, payload))
	if !ok {
		t.Fatal("ParseFrame rejected a frame produced by Frame")
	}
	if channel != ChannelStdin {
		t.Errorf("channel = %d, want %d", channel, ChannelStdin)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload = %q, want %q", got, payload)
	}
}

// An empty payload is ordinary: a control message can be nothing but its
// channel byte, and the browser must not treat that as a malformed frame.
func TestFrameEmptyPayload(t *testing.T) {
	channel, payload, ok := ParseFrame(Frame(ChannelStdout, nil))
	if !ok || channel != ChannelStdout || len(payload) != 0 {
		t.Fatalf("ParseFrame(empty payload) = %d, %q, %v", channel, payload, ok)
	}
}

// A message with no channel byte cannot be attributed to a stream, so it is
// rejected rather than guessed at.
func TestParseFrameRejectsEmptyMessage(t *testing.T) {
	if _, _, ok := ParseFrame(nil); ok {
		t.Fatal("expected an empty message to be rejected")
	}
}

func TestControlFrame(t *testing.T) {
	frame, err := ControlFrame(ControlMessage{Type: ControlResize, Cols: 120, Rows: 40})
	if err != nil {
		t.Fatalf("ControlFrame: %v", err)
	}

	channel, payload, ok := ParseFrame(frame)
	if !ok || channel != ChannelControl {
		t.Fatalf("channel = %d, ok = %v, want %d", channel, ok, ChannelControl)
	}

	var got ControlMessage
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal control payload: %v", err)
	}
	if got.Type != ControlResize || got.Cols != 120 || got.Rows != 40 {
		t.Errorf("control = %+v", got)
	}
}

// Session errors travel on the control channel, leaving the output channels to
// carry only what the sandbox itself produced.
func TestControlErrorCarriesMessage(t *testing.T) {
	frame, err := ControlFrame(ControlMessage{Type: ControlError, Message: "sandbox stopped"})
	if err != nil {
		t.Fatalf("ControlFrame: %v", err)
	}

	channel, payload, _ := ParseFrame(frame)
	if channel != ChannelControl {
		t.Fatalf("channel = %d, want %d", channel, ChannelControl)
	}

	var got ControlMessage
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal control payload: %v", err)
	}
	if got.Type != ControlError || got.Message != "sandbox stopped" {
		t.Errorf("control = %+v", got)
	}
	// Size fields are absent on an error, so a client never reads a stale shape.
	if got.Cols != 0 || got.Rows != 0 {
		t.Errorf("expected no dimensions on an error frame: %+v", got)
	}
}

// The terminal is authorized by the session cookie, so a page on another origin
// must never be able to open one. Only the dev UI server is exempt, and only
// when a dev port is configured.
func TestNewRestrictsOrigins(t *testing.T) {
	if got := New(nil, 0).devOrigins; len(got) != 0 {
		t.Fatalf("production must permit no foreign origin, got %v", got)
	}

	got := New(nil, 5174).devOrigins
	for _, want := range []string{"localhost:5174", "127.0.0.1:5174"} {
		if !slices.Contains(got, want) {
			t.Errorf("expected %q in %v", want, got)
		}
	}
	// A wildcard would defeat the check entirely, which is the mistake this
	// guards against.
	if slices.Contains(got, "*") {
		t.Error("the dev exemption must not be a wildcard")
	}
}
