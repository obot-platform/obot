package connectroute

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const VersionedPrefix = "/versioned-mcp-connect"

type Versioned struct {
	EntryID string
	Version int
	Rest    string
}

func ParseVersionedPath(path string) (Versioned, bool, error) {
	if path != VersionedPrefix && !strings.HasPrefix(path, VersionedPrefix+"/") {
		return Versioned{}, false, nil
	}

	parts := strings.Split(strings.TrimPrefix(path, VersionedPrefix+"/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Versioned{}, true, fmt.Errorf("versioned MCP connect path requires an entry ID and version")
	}
	version, err := strconv.Atoi(parts[1])
	if err != nil || version < 0 {
		return Versioned{}, true, fmt.Errorf("invalid MCP catalog entry version %q", parts[1])
	}

	route := Versioned{EntryID: parts[0], Version: version}
	if len(parts) > 2 {
		route.Rest = strings.Join(parts[2:], "/")
	}
	return route, true, nil
}

func ParseVersionedResource(resource string) (Versioned, bool, error) {
	u, err := url.Parse(resource)
	if err != nil {
		return Versioned{}, false, err
	}
	return ParseVersionedPath(u.Path)
}

func (r Versioned) Path() string {
	return fmt.Sprintf("%s/%s/%d", VersionedPrefix, r.EntryID, r.Version)
}

func (r Versioned) Resource(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + r.Path()
}
