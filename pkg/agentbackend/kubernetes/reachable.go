package kubernetes

import (
	"context"
	"fmt"
	"net"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ReachableServerURL rewrites Obot's own address into one a sandbox can use.
//
// A sandbox is told where to find Obot, and that address is resolved from
// inside the cluster rather than on the machine Obot runs on. When Obot runs in
// the cluster the two agree. When it does not -- which is what `make dev` does,
// with Obot on the developer's machine and sandboxes in a cluster -- the
// configured address is a loopback one, and loopback inside a pod is the pod
// itself. An agent would then be handed a URL that resolves to nothing, with
// every MCP call and model request failing on a connection refused that says
// nothing about why.
//
// The node's address is used because the developer's machine is the node: Obot
// listens on all interfaces, so a pod reaching the node on Obot's port reaches
// Obot. Anything that is not a loopback address is returned untouched, since it
// is already an address someone chose to be reachable.
//
// This is a development convenience. In production Obot runs in the cluster and
// its own Service name resolves, so nothing here applies.
func ReachableServerURL(ctx context.Context, client kclient.Client, serverURL string) (string, error) {
	if serverURL == "" || client == nil {
		return serverURL, nil
	}

	parsed, err := url.Parse(serverURL)
	if err != nil {
		return serverURL, nil
	}
	host := parsed.Hostname()
	if host == "" || !isLoopback(host) {
		return serverURL, nil
	}

	nodeIP, err := anyNodeAddress(ctx, client)
	if err != nil {
		return "", fmt.Errorf("resolve an address for %s that a sandbox can reach: %w", serverURL, err)
	}

	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort(nodeIP, port)
	} else {
		parsed.Host = nodeIP
	}
	return parsed.String(), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// anyNodeAddress returns a node's internal address. Any node will do: the
// developer's machine is the cluster, so every node is the same host.
func anyNodeAddress(ctx context.Context, client kclient.Client) (string, error) {
	var nodes corev1.NodeList
	if err := client.List(ctx, &nodes); err != nil {
		return "", fmt.Errorf("list nodes: %w", err)
	}
	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP && address.Address != "" {
				return address.Address, nil
			}
		}
	}
	return "", fmt.Errorf("no node reports an internal address")
}
