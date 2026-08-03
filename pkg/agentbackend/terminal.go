package agentbackend

import (
	"context"
	"io"
)

// TerminalBackend is an optional capability: a backend that can attach an
// interactive session to a running sandbox.
//
// It is separate from Backend so that a runtime without a console -- or one
// that has not implemented it yet -- simply does not satisfy it, and callers
// discover that with a type assertion rather than through a method that returns
// "unsupported" at runtime.
type TerminalBackend interface {
	// AttachTerminal connects to a sandbox's existing console. It attaches to
	// the process the sandbox is already running rather than starting a new
	// one, so an operator sees the same session the agent is driving.
	//
	// This requires the sandbox to have been started with a TTY, which is what
	// a harness marks by being interactive. Attaching to one that was not is an
	// error rather than a silently empty session.
	AttachTerminal(ctx context.Context, ref InstanceRef, size TerminalSize) (TerminalSession, error)
}

// TerminalSize is measured in character cells.
type TerminalSize struct {
	Rows uint16
	Cols uint16
}

// TerminalSession is a live console.
//
// Read yields console output and Write sends input. A console multiplexes what
// would otherwise be stdout and stderr onto one stream, because that is what a
// TTY does: the terminal is the process's controlling terminal, and both
// descriptors point at it. Callers that distinguish the two are reporting their
// own errors, not the sandbox's.
type TerminalSession interface {
	io.ReadWriteCloser

	// Resize tells the sandbox its terminal changed shape. Without it a program
	// drawing a full-screen interface keeps using the size it saw at startup.
	Resize(size TerminalSize) error
}
