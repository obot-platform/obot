package types

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	MCPServerRequestStatePending  MCPServerRequestState = "pending"
	MCPServerRequestStateApproved MCPServerRequestState = "approved"
	MCPServerRequestStateDenied   MCPServerRequestState = "denied"

	// shortDescriptionLimit is the catalog's limit on a short description, in characters.
	shortDescriptionLimit = 160
)

// MCPServerRequestState is where a request to add an MCP server stands.
type MCPServerRequestState string

// MCPServerRequestConfig is a value the requested server needs, such as an API key.
type MCPServerRequestConfig struct {
	// Key is the environment variable or, for a remote server, the header name.
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	// Usage is env or header. It defaults to header for a remote server and env otherwise.
	Usage Usage `json:"usage,omitempty"`
}

// MCPServerRequestManifest is what a user asks an administrator to add to the catalog,
// typically from their agent through Obot's built-in MCP server.
type MCPServerRequestManifest struct {
	// Name is the server's display name.
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Reason says why the user wants the server.
	Reason string `json:"reason,omitempty"`
	// SourceURL is where the server was found, such as its repository or documentation.
	SourceURL string `json:"sourceURL,omitempty"`

	// Runtime is remote, npx, uvx or containerized.
	Runtime Runtime `json:"runtime"`
	// RemoteURL is the MCP endpoint of a remote server.
	RemoteURL string `json:"remoteURL,omitempty"`
	// Package is the npm or PyPI package of an npx or uvx server.
	Package string `json:"package,omitempty"`
	// Image, Port and Path describe a containerized server.
	Image string   `json:"image,omitempty"`
	Port  int      `json:"port,omitempty"`
	Path  string   `json:"path,omitempty"`
	Args  []string `json:"args,omitempty"`

	// Config lists the values the server needs, such as API keys.
	Config []MCPServerRequestConfig `json:"config,omitempty"`

	// SuggestedVMCPID is the vMCP the requester would like the server added to.
	SuggestedVMCPID string `json:"suggestedVMCPID,omitempty"`

	// ExistingCatalogEntryID asks for a server already in the catalog to be made available,
	// rather than for a new one. Runtime and its fields are then unused.
	ExistingCatalogEntryID string `json:"existingCatalogEntryID,omitempty"`
}

// Validate checks the request is complete enough for an administrator to act on.
func (m MCPServerRequestManifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if m.ExistingCatalogEntryID != "" {
		return nil
	}
	switch m.Runtime {
	case RuntimeRemote:
		u, err := url.Parse(m.RemoteURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("remoteURL must be an http(s) URL for a remote server")
		}
	case RuntimeNPX, RuntimeUVX:
		if strings.TrimSpace(m.Package) == "" {
			return fmt.Errorf("package is required for a %s server", m.Runtime)
		}
	case RuntimeContainerized:
		if strings.TrimSpace(m.Image) == "" || m.Port <= 0 || strings.TrimSpace(m.Path) == "" {
			return fmt.Errorf("image, port and path are required for a containerized server")
		}
	default:
		return fmt.Errorf("runtime must be one of remote, npx, uvx or containerized")
	}
	for _, c := range m.Config {
		if strings.TrimSpace(c.Key) == "" {
			return fmt.Errorf("every config value needs a key")
		}
		if c.Usage != "" && c.Usage != Env && c.Usage != Header {
			return fmt.Errorf("config %q: usage must be env or header", c.Key)
		}
	}
	return nil
}

// CatalogEntryManifest is the catalog entry the request proposes. An administrator can
// edit it before approving.
func (m MCPServerRequestManifest) CatalogEntryManifest() MCPServerCatalogEntryManifest {
	manifest := MCPServerCatalogEntryManifest{
		Name:             m.Name,
		ShortDescription: firstLine(m.Description),
		Description:      m.Description,
		RepoURL:          m.SourceURL,
		Runtime:          m.Runtime,
	}
	switch m.Runtime {
	case RuntimeRemote:
		manifest.RemoteConfig = &RemoteCatalogConfig{FixedURL: m.RemoteURL}
	case RuntimeNPX:
		manifest.NPXConfig = &NPXRuntimeConfig{Package: m.Package, Args: m.Args}
	case RuntimeUVX:
		manifest.UVXConfig = &UVXRuntimeConfig{Package: m.Package, Args: m.Args}
	case RuntimeContainerized:
		manifest.ContainerizedConfig = &ContainerizedRuntimeConfig{Image: m.Image, Port: m.Port, Path: m.Path, Args: m.Args}
	}
	for _, c := range m.Config {
		usage := c.Usage
		if usage == "" {
			usage = Env
			if m.Runtime == RuntimeRemote {
				usage = Header
			}
		}
		manifest.Config = append(manifest.Config, MCPConfig{
			Name:        c.Key,
			Key:         c.Key,
			Description: c.Description,
			Sensitive:   c.Sensitive,
			Required:    true,
			Usage:       usage,
		})
	}
	return manifest
}

// firstLine is the first line of s, shortened at a word to fit a short description.
func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= shortDescriptionLimit {
		return string(runes)
	}
	cut := string(runes[:shortDescriptionLimit-1])
	if i := strings.LastIndex(cut, " "); i > shortDescriptionLimit/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.-") + "…"
}

type MCPServerRequest struct {
	Metadata
	MCPServerRequestManifest

	RequesterID    string `json:"requesterID"`
	RequesterName  string `json:"requesterName,omitempty"`
	RequesterEmail string `json:"requesterEmail,omitempty"`

	State          MCPServerRequestState `json:"state"`
	DecidedBy      string                `json:"decidedBy,omitempty"`
	DecidedAt      *Time                 `json:"decidedAt,omitempty"`
	DecisionReason string                `json:"decisionReason,omitempty"`
	// CatalogEntryID and VMCPID are set when the request is approved.
	CatalogEntryID string `json:"catalogEntryID,omitempty"`
	VMCPID         string `json:"vmcpID,omitempty"`
	// MatchedCatalogEntryID, shown to administrators, is an existing catalog entry for the
	// same server, which approval reuses.
	MatchedCatalogEntryID string `json:"matchedCatalogEntryID,omitempty"`
	// RequesterCanConnect reports, for an approved request, whether the vMCP's profiles let
	// the requester connect. Approving into an existing vMCP doesn't change who it grants.
	RequesterCanConnect *bool `json:"requesterCanConnect,omitempty"`
}

type MCPServerRequestList List[MCPServerRequest]

// MCPServerRequestNewVMCP describes the vMCP to create for an approved server.
type MCPServerRequestNewVMCP struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
}

// MCPServerRequestApproval is an administrator's approval: where the server goes, and
// optionally the catalog entry as edited by the administrator.
type MCPServerRequestApproval struct {
	// VMCPID adds the server to an existing vMCP.
	VMCPID string `json:"vmcpID,omitempty"`
	// NewVMCP creates a vMCP for the server.
	NewVMCP *MCPServerRequestNewVMCP `json:"newVMCP,omitempty"`
	// Manifest overrides the catalog entry derived from the request.
	Manifest *MCPServerCatalogEntryManifest `json:"manifest,omitempty"`
	// GrantRequesterAccess, when adding to an existing vMCP, gives the requester a profile for
	// the new server alone if no profile lets them connect. Nil means true.
	GrantRequesterAccess *bool  `json:"grantRequesterAccess,omitempty"`
	Reason               string `json:"reason,omitempty"`
}

func (a MCPServerRequestApproval) Validate() error {
	if (a.VMCPID == "") == (a.NewVMCP == nil) {
		return fmt.Errorf("exactly one of vmcpID or newVMCP is required")
	}
	if a.NewVMCP != nil && strings.TrimSpace(a.NewVMCP.DisplayName) == "" {
		return fmt.Errorf("newVMCP.displayName is required")
	}
	return nil
}

type MCPServerRequestDenial struct {
	Reason string `json:"reason,omitempty"`
}
