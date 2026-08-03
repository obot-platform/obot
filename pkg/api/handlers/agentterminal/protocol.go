// Package agentterminal carries an interactive sandbox console over a
// websocket.
package agentterminal

import "encoding/json"

// Frames are binary websocket messages whose first byte selects a channel and
// whose remainder is the payload. This is the shape Docker and Kubernetes both
// use for multiplexed streams, and it is used here for the same reason: one
// ordered connection carrying several kinds of traffic, without a length-prefix
// framing layer of its own, because websockets already preserve message
// boundaries.
//
// Channels are directional by convention rather than enforcement, which keeps
// the framing trivial:
//
//	ChannelStdin    browser -> server
//	ChannelStdout   server  -> browser
//	ChannelStderr   server  -> browser
//	ChannelControl  either direction
const (
	// ChannelStdin carries keystrokes toward the sandbox.
	ChannelStdin byte = 0
	// ChannelStdout carries console output. A session attached over a TTY
	// merges its output onto this channel, because a TTY is a single stream.
	ChannelStdout byte = 1
	// ChannelStderr carries the sandbox's stderr. It is unused for a TTY
	// session, where the kernel has already merged stderr into stdout, and is
	// reserved for a non-TTY session that keeps the two streams apart. Errors
	// about the session itself travel on the control channel instead, so this
	// one only ever carries the sandbox's own output.
	ChannelStderr byte = 2
	// ChannelControl carries JSON messages that are about the session rather
	// than its content.
	ChannelControl byte = 3
)

// ControlMessage is the payload of a control frame.
type ControlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	// Message describes a session-level failure. It accompanies ControlError.
	Message string `json:"message,omitempty"`
}

const (
	// ControlResize is sent by the browser when the terminal changes shape.
	ControlResize = "resize"
	// ControlError reports a failure of the session rather than of the program
	// running in it: an attach that did not succeed, a sandbox that stopped.
	// It is distinct from the output channels so a client can tell Obot's
	// errors from anything the sandbox itself printed.
	ControlError = "error"
)

// Frame builds a message for a channel.
func Frame(channel byte, payload []byte) []byte {
	out := make([]byte, 0, len(payload)+1)
	return append(append(out, channel), payload...)
}

// ParseFrame splits a message into its channel and payload. A message with no
// channel byte is rejected rather than guessed at.
func ParseFrame(message []byte) (byte, []byte, bool) {
	if len(message) == 0 {
		return 0, nil, false
	}
	return message[0], message[1:], true
}

// ControlFrame encodes a control message.
func ControlFrame(message ControlMessage) ([]byte, error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	return Frame(ChannelControl, payload), nil
}
