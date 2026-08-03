package kubernetes

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/obot-platform/obot/pkg/agentbackend"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/streaming/pkg/httpstream"
)

var _ agentbackend.TerminalBackend = (*Backend)(nil)

// AttachTerminal connects to the console of a sandbox's running process.
//
// This attaches rather than execs. Exec would start a second process with its
// own TTY, which gives a shell beside the agent but not the agent's own
// session; attach joins the console the container was started with, so an
// operator sees exactly what the agent is doing. That console only exists
// because the harness was marked interactive, which is why a terminal requires
// one.
func (b *Backend) AttachTerminal(ctx context.Context, ref agentbackend.InstanceRef, size agentbackend.TerminalSize) (agentbackend.TerminalSession, error) {
	if b.attachClient == nil {
		return nil, fmt.Errorf("terminal requires a Kubernetes connection")
	}

	pod, err := b.runningPod(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	if !podHasTTY(pod) {
		return nil, fmt.Errorf("sandbox %s was not started with a terminal; its harness is not interactive", ref.ID)
	}

	request := b.attachClient.Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("attach").
		VersionedParams(&corev1.PodAttachOptions{
			Container: agentContainerName,
			Stdin:     true,
			Stdout:    true,
			// A TTY merges the two streams, so asking for stderr as well is
			// rejected by the API server.
			Stderr: false,
			TTY:    true,
		}, scheme.ParameterCodec)

	// WebSocket first, SPDY second.
	//
	// SPDY is an HTTP/1.1 upgrade to a protocol no general-purpose proxy
	// speaks, so an ingress, load balancer or corporate egress proxy between
	// Obot and the API server tends to refuse the upgrade -- a terminal that
	// fails while every ordinary API call succeeds. WebSocket traverses that
	// same infrastructure, which is why kubectl made the same move.
	//
	// SPDY stays as the fallback because NewWebSocketExecutor negotiates only
	// v5.channel.k8s.io, deliberately, and so cannot talk to an API server
	// older than the release that added it. Keeping the second path means Obot
	// does not impose a cluster version floor for terminals.
	websocketExecutor, err := remotecommand.NewWebSocketExecutor(b.opts.RESTConfig, "GET", request.URL().String())
	if err != nil {
		return nil, fmt.Errorf("build terminal executor: %w", err)
	}
	spdyExecutor, err := remotecommand.NewSPDYExecutor(b.opts.RESTConfig, "POST", request.URL())
	if err != nil {
		return nil, fmt.Errorf("build terminal fallback executor: %w", err)
	}
	executor, err := remotecommand.NewFallbackExecutor(websocketExecutor, spdyExecutor, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
	if err != nil {
		return nil, fmt.Errorf("build terminal executor: %w", err)
	}

	session := newTerminalSession(size)
	go func() {
		// StreamWithContext blocks until the console closes. Its error is
		// delivered to the reader so the browser learns why the session ended
		// rather than seeing the socket drop.
		err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:             session.inputReader,
			Stdout:            session.outputWriter,
			Tty:               true,
			TerminalSizeQueue: session,
		})
		session.finish(err)
	}()

	return session, nil
}

// terminalSession adapts the streaming executor, which wants readers and
// writers it can block on, to the ReadWriteCloser the caller wants.
type terminalSession struct {
	inputReader  *io.PipeReader
	inputWriter  *io.PipeWriter
	outputReader *io.PipeReader
	outputWriter *io.PipeWriter

	// resizes is consumed by the executor. It is buffered and lossy: only the
	// most recent size matters, and a resize must never block the session.
	resizes chan remotecommand.TerminalSize

	closeOnce sync.Once
}

func newTerminalSession(size agentbackend.TerminalSize) *terminalSession {
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()

	session := &terminalSession{
		inputReader:  inputReader,
		inputWriter:  inputWriter,
		outputReader: outputReader,
		outputWriter: outputWriter,
		resizes:      make(chan remotecommand.TerminalSize, 1),
	}
	// Seed the initial size so the first frame the program draws matches the
	// browser rather than an 80x24 default it would have to redraw.
	session.resizes <- remotecommand.TerminalSize{Width: size.Cols, Height: size.Rows}
	return session
}

func (s *terminalSession) Read(p []byte) (int, error)  { return s.outputReader.Read(p) }
func (s *terminalSession) Write(p []byte) (int, error) { return s.inputWriter.Write(p) }

// Next implements remotecommand.TerminalSizeQueue. Returning nil ends the
// queue, which the executor treats as "no more resizes".
func (s *terminalSession) Next() *remotecommand.TerminalSize {
	size, ok := <-s.resizes
	if !ok {
		return nil
	}
	return &size
}

func (s *terminalSession) Resize(size agentbackend.TerminalSize) error {
	next := remotecommand.TerminalSize{Width: size.Cols, Height: size.Rows}
	for {
		select {
		case s.resizes <- next:
			return nil
		default:
			// Full: drop the pending size, which is now stale, and retry. A
			// terminal only cares about its current dimensions.
			select {
			case <-s.resizes:
			default:
			}
		}
	}
}

func (s *terminalSession) Close() error {
	s.closeOnce.Do(func() {
		close(s.resizes)
		_ = s.inputWriter.Close()
		_ = s.outputWriter.Close()
	})
	return nil
}

// finish ends the session, surfacing the reason to the reader.
func (s *terminalSession) finish(err error) {
	if err != nil {
		_ = s.outputWriter.CloseWithError(err)
	}
	s.Close()
}

// attachConfig prepares a REST config for the pods/attach subresource, which is
// a raw stream rather than an API object and so carries no codec of its own.
func attachConfig(config *rest.Config) *rest.Config {
	attach := rest.CopyConfig(config)
	attach.GroupVersion = &corev1.SchemeGroupVersion
	attach.APIPath = "/api"
	attach.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	return attach
}

func podHasTTY(pod *corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == agentContainerName {
			return container.TTY && container.Stdin
		}
	}
	return false
}

// runningPod returns the sandbox's current pod. A terminal needs a live
// process, so a pod that is pending or terminating is reported as unavailable
// rather than attached to.
func (b *Backend) runningPod(ctx context.Context, instanceID string) (*corev1.Pod, error) {
	pods, err := b.instancePods(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	for i := range pods {
		pod := &pods[i]
		if pod.Status.Phase == corev1.PodRunning && pod.DeletionTimestamp.IsZero() {
			return pod, nil
		}
	}
	return nil, fmt.Errorf("sandbox %s has no running pod to attach to", instanceID)
}
