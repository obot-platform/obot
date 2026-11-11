package authz

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
)

const apiProxyPattern = "/api-proxy/{vmcp_id}/{component_id}/{path...}"

var errInvalidAPIProxySegment = errors.New("invalid API proxy segment")

func (a *Authorizer) authorizeAPIProxy(req *http.Request, user User) bool {
	if !slices.Contains(user.GetGroups(), types.GroupMCP) {
		return false
	}
	vmcpID, _, ok := parseAPIProxyRoute(req)
	if !ok || !system.IsVMCPID(vmcpID) {
		return false
	}
	connection, err := resolveMCPConnection(req.Context(), a.uncached, vmcpID, user.GetUID())
	if err != nil || connection.vmcp == nil {
		return false
	}
	resources := Resources{}
	authorized, err := userCanConnectToResolvedMCP(req.Context(), a.uncached, a.acrHelper, user.Info, connection, &resources)
	return err == nil && authorized
}

func isAPIProxyRequest(req *http.Request) bool {
	return req.Pattern == apiProxyPattern
}

func parseAPIProxyRoute(req *http.Request) (string, string, bool) {
	remainder, ok := strings.CutPrefix(req.URL.EscapedPath(), "/api-proxy/")
	if !ok {
		return "", "", false
	}
	firstSlash := strings.IndexByte(remainder, '/')
	if firstSlash < 1 {
		return "", "", false
	}
	vmcpSegment := remainder[:firstSlash]
	remainder = remainder[firstSlash+1:]
	secondSlash := strings.IndexByte(remainder, '/')
	componentSegment := remainder
	if secondSlash >= 0 {
		componentSegment = remainder[:secondSlash]
	}
	vmcpID, err := decodeAPIProxySegment(vmcpSegment)
	if err != nil || !system.IsVMCPID(vmcpID) {
		return "", "", false
	}
	componentID, err := decodeAPIProxySegment(componentSegment)
	if err != nil || componentID == "" {
		return "", "", false
	}
	return vmcpID, componentID, true
}

func decodeAPIProxySegment(segment string) (string, error) {
	decoded, err := url.PathUnescape(segment)
	if err != nil || !utf8.ValidString(decoded) || decoded == "" ||
		decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "\x00/\r\n\t") {
		return "", errInvalidAPIProxySegment
	}
	return decoded, nil
}
