package kubernetes

import (
	"fmt"
	"net/http"
	"strings"
)

// DevRoute returns an address and transport that reach a sandbox from outside
// the cluster, by way of the API server's service proxy.
//
// A sandbox is only addressable at a cluster-internal name, which resolves
// nowhere when Obot runs on a developer's machine. The API server will proxy to
// a Service on our behalf, so a developer already holding cluster credentials
// can reach their agent without a port-forward or a tunnel.
//
// This is for development only. Reaching a Service this way needs the
// services/proxy permission, which grants access to every Service in the
// cluster, not merely agent sandboxes. A production deployment runs Obot inside
// the cluster where the internal address resolves directly, so it neither needs
// nor should be granted that permission.
//
// The bool reports whether a route could be produced at all; callers fall back
// to the sandbox's own address when it is false.
func (b *Backend) DevRoute(backendID string) (string, http.RoundTripper, bool) {
	// devTransport is nil when there is no Kubernetes connection, or when one
	// could not be built from it. Either way there is no route; the caller falls
	// back and reports the real connection failure.
	if b == nil || b.opts.RESTConfig == nil || b.devTransport == nil || backendID == "" {
		return "", nil, false
	}

	// The scheme prefix selects the Service port by name, which is how the
	// sandbox Service declares it.
	target := fmt.Sprintf("%s/api/v1/namespaces/%s/services/http:%s:80/proxy",
		strings.TrimSuffix(b.opts.RESTConfig.Host, "/"), b.opts.Namespace, backendID)

	return target, b.devTransport, true
}
