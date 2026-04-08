package mcp

import (
	"net/url"
	"os"
	"strings"
)

// nanobotOTELEnv returns OTEL environment variables to inject into Nanobot containers.
// It copies all OTEL_* variables from the current process, optionally rewriting OTLP
// endpoint URLs for the target runtime, and sets the service name so spans are
// distinguishable.
func nanobotOTELEnv(serviceName string, transformEndpoint func(string) string) map[string][]byte {
	env := make(map[string][]byte)

	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "OTEL_") {
			continue
		}

		if isOTLPEndpointEnv(key) && transformEndpoint != nil {
			value = transformEndpoint(value)
		}

		env[key] = []byte(value)
	}

	env["OTEL_SERVICE_NAME"] = []byte(serviceName)

	return env
}

func rewriteLocalhostURLHost(rawURL, host string) string {
	if rawURL == "" || host == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	if parsed.Hostname() != "localhost" {
		return rawURL
	}

	if port := parsed.Port(); port != "" {
		parsed.Host = host + ":" + port
	} else {
		parsed.Host = host
	}

	return parsed.String()
}

func isOTLPEndpointEnv(key string) bool {
	switch key {
	case "OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT":
		return true
	default:
		return false
	}
}
