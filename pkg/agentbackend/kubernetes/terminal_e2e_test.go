package kubernetes

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/agentbackend"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// TestAttachTerminalAgainstCluster attaches to a sandbox that is already
// running in a real cluster.
//
// Attaching is the part of the terminal that unit tests cannot reach: it needs
// an API server that will upgrade the connection, a pod started with a TTY, and
// a shell on the other end. It is opt-in because it needs all three.
//
// Which upgrade it exercises depends on the cluster, since the executor prefers
// WebSocket and falls back to SPDY. Running this against both an API server new
// enough for v5.channel.k8s.io and one older than it is what covers each path;
// neither is reachable from a unit test.
//
//	OBOT_E2E_KUBECONFIG=~/.kube/config \
//	OBOT_E2E_NAMESPACE=obot-mcp \
//	OBOT_E2E_INSTANCE_ID=<obot.ai/hosted-agent-instance label> \
//	go test ./pkg/agentbackend/kubernetes -run TestAttachTerminalAgainstCluster -v
func TestAttachTerminalAgainstCluster(t *testing.T) {
	kubeconfig := os.Getenv("OBOT_E2E_KUBECONFIG")
	namespace := os.Getenv("OBOT_E2E_NAMESPACE")
	instanceID := os.Getenv("OBOT_E2E_INSTANCE_ID")
	if kubeconfig == "" || namespace == "" || instanceID == "" {
		t.Skip("set OBOT_E2E_KUBECONFIG, OBOT_E2E_NAMESPACE and OBOT_E2E_INSTANCE_ID to run")
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	client, err := kclient.New(restConfig, kclient.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	backend, err := New(client, nil, Options{Namespace: namespace, RESTConfig: restConfig})
	if err != nil {
		t.Fatalf("build backend: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session, err := backend.AttachTerminal(ctx,
		agentbackend.InstanceRef{ID: instanceID, Namespace: namespace},
		agentbackend.TerminalSize{Rows: 40, Cols: 120})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	defer session.Close()

	// A marker rather than a fixed string: the shell echoes the command back
	// over the TTY, so the output has to prove the command actually ran.
	const marker = "obot-terminal-e2e-ok"
	if _, err := session.Write([]byte("echo " + marker + "\n")); err != nil {
		t.Fatalf("write to console: %v", err)
	}

	if err := session.Resize(agentbackend.TerminalSize{Rows: 24, Cols: 80}); err != nil {
		t.Errorf("resize: %v", err)
	}

	output := readUntil(t, session, marker, 30*time.Second)
	// Two occurrences: the echoed command line, then its output. One means the
	// TTY is relaying keystrokes but the shell is not running them.
	if strings.Count(output, marker) < 2 {
		t.Fatalf("expected the command to be echoed and to run; got:\n%s", output)
	}
	t.Logf("console output:\n%s", output)
}

// readUntil collects console output until the marker has appeared twice or the
// deadline passes, so a failure shows what the console actually said.
func readUntil(t *testing.T, session agentbackend.TerminalSession, marker string, timeout time.Duration) string {
	t.Helper()

	var (
		collected strings.Builder
		done      = make(chan struct{})
	)
	go func() {
		defer close(done)
		buffer := make([]byte, 4096)
		for {
			n, err := session.Read(buffer)
			if n > 0 {
				collected.Write(buffer[:n])
				if strings.Count(collected.String(), marker) >= 2 {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Error("timed out waiting for the console to echo the command")
	}
	return collected.String()
}
