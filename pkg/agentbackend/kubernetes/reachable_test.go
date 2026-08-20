package kubernetes

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func nodeClient(addresses ...corev1.NodeAddress) *fake.ClientBuilder {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&corev1.Node{
		Name:   "node-1",
		Status: corev1.NodeStatus{Addresses: addresses},
	})
}

// A loopback address inside a pod is the pod itself, so an agent told to reach
// Obot there fails on every request with a connection refused that says nothing
// about why.
func TestLoopbackIsRewrittenToTheNode(t *testing.T) {
	client := nodeClient(corev1.NodeAddress{Type: corev1.NodeInternalIP, Address: "172.26.119.21"}).Build()

	for _, tt := range []struct{ in, want string }{
		{"http://localhost:8080", "http://172.26.119.21:8080"},
		{"http://127.0.0.1:8080", "http://172.26.119.21:8080"},
		{"http://localhost:8080/", "http://172.26.119.21:8080/"},
		// No port: the scheme's default is implied and must stay implied.
		{"http://localhost", "http://172.26.119.21"},
	} {
		got, err := ReachableServerURL(context.Background(), client, tt.in)
		if err != nil {
			t.Fatalf("ReachableServerURL(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ReachableServerURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// An address someone deliberately configured is already reachable; rewriting it
// would break a production deployment whose Obot runs in the cluster.
func TestRoutableAddressIsLeftAlone(t *testing.T) {
	client := nodeClient(corev1.NodeAddress{Type: corev1.NodeInternalIP, Address: "172.26.119.21"}).Build()

	for _, in := range []string{
		"https://obot.example.com",
		"http://obot.obot-system.svc.cluster.local:8080",
		"https://10.0.0.5:8443",
		"",
	} {
		got, err := ReachableServerURL(context.Background(), client, in)
		if err != nil {
			t.Fatalf("ReachableServerURL(%q): %v", in, err)
		}
		if got != in {
			t.Errorf("ReachableServerURL(%q) = %q, want it untouched", in, got)
		}
	}
}

// Failing loudly beats handing every sandbox an address that cannot work.
func TestNoNodeAddressIsAnError(t *testing.T) {
	client := nodeClient(corev1.NodeAddress{Type: corev1.NodeHostName, Address: "node-1"}).Build()

	if _, err := ReachableServerURL(context.Background(), client, "http://localhost:8080"); err == nil {
		t.Fatal("expected an error when no node reports an internal address")
	}
}
