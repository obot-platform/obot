package openapi

import (
	"fmt"
	"regexp"
	"strings"
)

// Credential headers must not override HTTP transport or MCP protocol headers.
var headerToken = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func reservedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "cookie", "set-cookie", "content-length", "content-type", "accept",
		"connection", "transfer-encoding", "upgrade", "te", "trailer", "keep-alive",
		"expect", "proxy-authorization", "proxy-authenticate", "accept-encoding":
		return true
	}
	name = strings.ToLower(name)
	return strings.HasPrefix(name, "mcp-") || strings.HasPrefix(name, "sec-") || strings.HasPrefix(name, "proxy-")
}

func validateHeader(name string) error {
	if !headerToken.MatchString(name) || reservedHeader(name) {
		return fmt.Errorf("credential headers must be valid non-transport header names")
	}
	return nil
}
