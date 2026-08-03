package hostedagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/pkg/hostedagentmodels"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// agentConfig is the whole of what a sandbox is told, written to
// /etc/obot/agent.json at start-up.
//
// Every endpoint in it is a complete URL, so an image boots by reading this
// file and needs to know nothing about Obot -- not its address, not its
// routing, not that it is Obot at all. An image that had to assemble a URL from
// a base and an ID would be coupled to Obot's routes and would break the moment
// they changed.
//
// It carries no credentials. Those live in a second file, named by SecretsFile
// and readable only by the agent, so that this one can be world-readable and
// safely inspected, logged or copied without leaking a token.
type agentConfig struct {
	Version  string          `json:"version"`
	Instance agentConfigMeta `json:"instance"`

	// SecretsFile is where the credentials for the endpoints below are kept.
	// An agent merges the two by ID; see agentSecrets.
	SecretsFile string `json:"secretsFile,omitempty"`

	// Workspace is the writable directory that survives a restart.
	Workspace string `json:"workspace,omitempty"`
	// Source is the repository to check out, if the agent or user chose one.
	Source *agentConfigSource `json:"source,omitempty"`
	// ListenPort is set when the agent is expected to serve HTTP.
	ListenPort int `json:"listenPort,omitempty"`
	// PublicPath is the path prefix the agent is published under, and PublicURL
	// the whole address. Both are set only when the agent serves HTTP.
	//
	// An agent is not reached at its own root: Obot mounts it under a prefix and
	// strips that prefix before forwarding. A server that does not know this
	// builds links and API calls against the root, which land outside its own
	// prefix -- a single-page UI ends up calling Obot instead of itself.
	// X-Forwarded-Prefix says the same thing per request, but a server that
	// needs it at start-up, to bake into the page it serves, cannot use a header.
	PublicPath string `json:"publicPath,omitempty"`
	PublicURL  string `json:"publicURL,omitempty"`

	// ObotURL is Obot itself, at the address this sandbox reaches it on. The
	// endpoints below are already absolute, so an agent needs this only for
	// what they do not cover -- calling Obot's API directly with the credential
	// it was issued.
	//
	// It is deliberately not PublicURL's host: the public address is the one a
	// browser uses, and is commonly not routable from inside the cluster.
	// Answering "where is Obot" with the browser's answer is what breaks a
	// sandbox that then cannot reach its own models.
	ObotURL string `json:"obotURL,omitempty"`

	MCPServers []agentConfigMCPServer `json:"mcpServers,omitempty"`
	Models     []agentConfigModel     `json:"models,omitempty"`
	Skills     []agentConfigSkill     `json:"skills,omitempty"`

	// Answers are the user's responses to the agent's questions. Sensitive
	// answers are excluded.
	Answers map[string]string `json:"answers,omitempty"`
}

type agentConfigMeta struct {
	ID     string `json:"id"`
	UserID string `json:"userID"`
	Name   string `json:"name,omitempty"`
}

type agentConfigSource struct {
	URL string `json:"url"`
	// Ref is a branch, tag or commit. Empty means the default branch.
	Ref    string `json:"ref,omitempty"`
	Subdir string `json:"subdir,omitempty"`
}

type agentConfigMCPServer struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// URL is complete and ready to connect to.
	URL string `json:"url"`
	// Transport names the wire protocol so a client can configure itself
	// without guessing from the URL shape.
	Transport string `json:"transport"`
	// Headers holds only non-sensitive headers. Anything carrying a credential
	// is in the secrets file under the same ID.
	Headers map[string]string `json:"headers,omitempty"`
}

type agentConfigModel struct {
	// ID is Obot's identifier; Model is the name to send on the wire.
	ID    string `json:"id,omitempty"`
	Model string `json:"model"`
	// APIs are the wire protocols this model accepts: "anthropic",
	// "openai-responses", "openai-chat-completions". An image picks a model it
	// can speak to; a client written for one format cannot use another, and a
	// model is often reachable over more than one. Provider is the underlying
	// Obot model provider, present for diagnostics rather than for dispatch.
	APIs     []string `json:"apis"`
	Provider string   `json:"provider,omitempty"`
	// BaseURL is the root of Obot's proxy for this provider, carrying no API
	// version. The version belongs to the protocol the client speaks, not to
	// the routing: an Anthropic client appends /v1/messages to it, while an
	// OpenAI one treats /v1 as part of its base and appends /responses. Naming
	// a version here would pick one of those conventions for every client, and
	// could not express a provider moving to /v2 without a new field.
	BaseURL string `json:"baseURL"`
	// There is deliberately no API key here: it is in the secrets file, keyed by
	// Provider, since that is what decides the endpoint the key is sent to.

	// Default marks the model to reach for when an image wants one. It is a
	// hint, not a restriction: the agent may use any model listed here, and it
	// exists because a bare list gives an image no way to choose.
	Default bool `json:"default,omitempty"`
}

type agentConfigSkill struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Path is a directory in the sandbox holding the skill's files.
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

// agentSecrets is the credential half of the contract, keyed by the same IDs
// used in agentConfig so the two merge without further lookup.
//
// It is a separate file so that the configuration an agent needs to read --
// endpoints, skills, answers -- carries no secret, and only this one has to be
// protected.
type agentSecrets struct {
	Version    string                    `json:"version"`
	MCPServers map[string]agentSecretMCP `json:"mcpServers,omitempty"`
	// ModelProviders is keyed by model provider, not by model. A credential
	// belongs to the endpoint it is sent to, and every model from one provider
	// shares an endpoint -- keying by model repeated the same value once per
	// model, which on an installation with a large catalogue meant hundreds of
	// copies of one string.
	ModelProviders map[string]agentSecretModelProvider `json:"modelProviders,omitempty"`
}

type agentSecretMCP struct {
	Headers map[string]string `json:"headers,omitempty"`
}

type agentSecretModelProvider struct {
	APIKey string `json:"apiKey,omitempty"`
}

// transportStreamableHTTP is what the MCP gateway speaks.
const transportStreamableHTTP = "streamable-http"

// mcpServerConfig builds a ready-to-use connection for each MCP server the
// agent was granted.
func mcpServerConfigs(serverURL string, ids []string) []agentConfigMCPServer {
	servers := make([]agentConfigMCPServer, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		servers = append(servers, agentConfigMCPServer{
			ID:        id,
			Name:      id,
			URL:       fmt.Sprintf("%s/mcp-connect/%s", strings.TrimSuffix(serverURL, "/"), id),
			Transport: transportStreamableHTTP,
		})
	}
	return servers
}

// modelConfigs resolves what the agent may use into complete endpoints.
//
// Resolution is shared with the API, which uses the same answer to decide
// whether an agent can be launched at all. If the two disagreed, a user could
// be offered an agent that then resolves no models.
func modelConfigs(ctx context.Context, client kclient.Client, namespace, serverURL string, selections, providers []string) ([]agentConfigModel, error) {
	resolved, err := hostedagentmodels.Resolve(ctx, client, namespace, selections, providers)
	if err != nil {
		return nil, err
	}

	defaultID := hostedagentmodels.Default(ctx, client, namespace, resolved)
	models := make([]agentConfigModel, 0, len(resolved))
	for _, model := range resolved {
		path, ok := hostedagentmodels.ProxyPath(model.Provider)
		if !ok {
			continue
		}
		models = append(models, agentConfigModel{
			ID:       model.ID,
			Model:    model.Model,
			APIs:     model.APIs,
			Provider: model.Provider,
			BaseURL:  fmt.Sprintf("%s/api/llm-proxy/%s", strings.TrimSuffix(serverURL, "/"), path),
			Default:  model.ID == defaultID,
		})
	}
	return models, nil
}

// buildSecrets pairs every endpoint in the config with the credential it needs.
//
// Today all of them use the agent's own credential; keying by ID rather than
// emitting one shared token means a per-server credential later changes only
// what is written here, not the contract an image reads.
func buildSecrets(config agentConfig, credential string) agentSecrets {
	secrets := agentSecrets{Version: config.Version}
	if credential == "" {
		return secrets
	}

	if len(config.MCPServers) > 0 {
		secrets.MCPServers = make(map[string]agentSecretMCP, len(config.MCPServers))
		for _, server := range config.MCPServers {
			secrets.MCPServers[server.ID] = agentSecretMCP{
				Headers: map[string]string{"Authorization": "Bearer " + credential},
			}
		}
	}
	for _, model := range config.Models {
		if model.Provider == "" {
			continue
		}
		if secrets.ModelProviders == nil {
			secrets.ModelProviders = map[string]agentSecretModelProvider{}
		}
		secrets.ModelProviders[model.Provider] = agentSecretModelProvider{APIKey: credential}
	}
	return secrets
}
