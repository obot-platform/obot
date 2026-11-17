package system

import (
	"os"
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
func NetworkEnv() []string {
	var env []string
	for _, name := range networkEnvVars {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}
