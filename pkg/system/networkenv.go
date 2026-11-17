package system

import (
	"os"
	"slices"
)

var (
	// networkEnvVars are the variables that configure a Go child process's outbound network
	// access: the egress proxy and the CA bundles used to trust TLS-intercepting proxies.
	networkEnvVars = []string{
		"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"http_proxy", "https_proxy", "no_proxy",
		"SSL_CERT_FILE", "SSL_CERT_DIR",
	}
)

// NetworkEnv returns the proxy and CA bundle variables set in Obot's own environment, as
// NAME=value entries, so that child processes reach the network the same way Obot does.
// extra names further variables a child needs, such as NODE_EXTRA_CA_CERTS for Node.js,
// which ignores SSL_CERT_FILE.
func NetworkEnv(extra ...string) []string {
	var env []string
	for _, name := range slices.Concat(networkEnvVars, extra) {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}
