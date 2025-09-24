package nanobotgateway

import (
	"context"
	"os"
	"strings"

	napi "github.com/nanobot-ai/nanobot/pkg/api"
	"github.com/nanobot-ai/nanobot/pkg/confirm"
	"github.com/nanobot-ai/nanobot/pkg/llm"
	"github.com/nanobot-ai/nanobot/pkg/llm/responses"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/nanobot-ai/nanobot/pkg/runtime"
	nserver "github.com/nanobot-ai/nanobot/pkg/server"
	"github.com/nanobot-ai/nanobot/pkg/session"
	ntypes "github.com/nanobot-ai/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func Handler(dsn string) (api.HandlerFunc, error) {
	sessionManager, err := session.NewManager(dsn)
	if err != nil {
		return nil, err
	}

	runt, err := runtime.NewRuntime(llm.Config{
		// TODO(thedadams): this should be `string(types.DefaultModelAliasTypeLLM)`
		DefaultModel: "gpt-4.1",
		Responses: responses.Config{
			// TODO(thedadams): Figure out a way to configure these to point to Obot.
			APIKey:  os.Getenv("OPENAI_API_KEY"),
			BaseURL: "https://api.openai.com/v1",
		},
	}, runtime.Options{
		DSN: dsn,
		// TODO(thedadams): This would be used for the second-level OAuth, but that is currently untested.
		CallbackHandler: nmcp.NewCallbackServer(confirm.New()),
	})
	if err != nil {
		return nil, err
	}

	return (&handler{
		runtime:        runt,
		sessionManager: sessionManager,
	}).Proxy, nil
}

type handler struct {
	runtime        *runtime.Runtime
	sessionManager *session.Manager
}

func (h *handler) Proxy(req api.Context) error {
	host := req.Host
	nanobotID, _, ok := strings.Cut(host, ".")
	if !ok || nanobotID == "" {
		return types.NewErrBadRequest("%s is not a valid host", host)
	}

	var nanobotConfig v1.NanobotConfig
	if err := req.Get(&nanobotConfig, nanobotID); err != nil {
		return err
	}

	proto := req.URL.Scheme
	if proto == "" {
		proto = "http"
	}

	req.Request.Header.Set("X-Forwarded-Host", host)
	req.Request.Header.Set("X-Forwarded-Proto", proto)

	nctx := ntypes.Context{
		User: ntypes.User{
			ID:    req.User.GetUID(),
			Sub:   req.User.GetName(),
			Login: req.User.GetName(),
			Email: auth.FirstExtraValue(req.User.GetExtra(), "email"),
			Name:  req.User.GetName(),
		},
		Config: func(context.Context, string) (ntypes.Config, error) {
			return h.convertManifestToConfig(nanobotConfig.Spec.Manifest)
		},
	}

	// TODO(thedadams): Figure out a way to construct this UISession object once.
	// The host variable is dynamic, so this may require some changes in nanobot (i.e. pass a function that gets the host instead of the host as a string).
	// TODO(thedadams): This also requires running the nanobot UI separately.
	// The nanobot UI is packaged when running go generate in that repo, so it would be great if we could do this in Obot.
	session.UISession(nmcp.NewHTTPServer(nil, nserver.NewServer(h.runtime, nil, h.sessionManager), nmcp.HTTPServerOptions{
		SessionStore: h.sessionManager,
	}), h.sessionManager, napi.Handler(h.sessionManager, host)).ServeHTTP(req.ResponseWriter, req.WithContext(ntypes.WithNanobotContext(req.Context(), nctx)))

	return nil
}

func (h *handler) convertManifestToConfig(manifest types.NanobotConfigManifest) (ntypes.Config, error) {
	// TODO(thedadams): we need to pass the Auth parameter to this Config type. The client ID, secret, and authorization URL should point back to Obot.
	// The client ID and secret needs to be implemented in Obot, too.
	config := ntypes.Config{
		Extends: ntypes.StringList(manifest.Extends),
		Env:     make(map[string]ntypes.EnvDef, len(manifest.Env)),
		Publish: ntypes.Publish{
			Name:              manifest.Publish.Name,
			Introduction:      h.convertDynamicInstructions(manifest.Publish.Introduction),
			Version:           manifest.Publish.Version,
			Instructions:      manifest.Publish.Instructions,
			Tools:             ntypes.StringList(manifest.Publish.Tools),
			Prompts:           ntypes.StringList(manifest.Publish.Prompts),
			Resources:         ntypes.StringList(manifest.Publish.Resources),
			ResourceTemplates: ntypes.StringList(manifest.Publish.ResourceTemplates),
			MCPServers:        ntypes.StringList(manifest.Publish.MCPServers),
			Entrypoint:        ntypes.StringList(manifest.Publish.Entrypoint),
		},
		Agents:     make(map[string]ntypes.Agent, len(manifest.Agents)),
		MCPServers: make(map[string]nmcp.Server, len(manifest.MCPServers)),
	}

	// Convert environment variables
	for k, v := range manifest.Env {
		config.Env[k] = ntypes.EnvDef{
			Description:    v.Description,
			Default:        v.Default,
			Options:        ntypes.StringList(v.Options),
			Optional:       v.Optional,
			Sensitive:      v.Sensitive,
			UseBearerToken: v.UseBearerToken,
		}
	}

	// Convert agents
	for k, v := range manifest.Agents {
		var reasoning *ntypes.AgentReasoning
		if v.Reasoning != nil {
			reasoning = &ntypes.AgentReasoning{
				Effort:  v.Reasoning.Effort,
				Summary: v.Reasoning.Summary,
			}
		}

		config.Agents[k] = ntypes.Agent{
			Name:            v.Name,
			ShortName:       v.ShortName,
			Description:     v.Description,
			Icon:            v.Icon,
			IconDark:        v.IconDark,
			StarterMessages: ntypes.StringList(v.StarterMessages),
			Instructions:    h.convertDynamicInstructions(v.Instructions),
			Model:           v.Model,
			Before:          ntypes.StringList(v.Before),
			After:           ntypes.StringList(v.After),
			MCPServers:      ntypes.StringList(v.MCPServers),
			Tools:           ntypes.StringList(v.Tools),
			Agents:          ntypes.StringList(v.Agents),
			Flows:           ntypes.StringList(v.Flows),
			Prompts:         ntypes.StringList(v.Prompts),
			Reasoning:       reasoning,
			ThreadName:      v.ThreadName,
			Chat:            v.Chat,
			ToolChoice:      v.ToolChoice,
			Temperature:     v.Temperature,
			TopP:            v.TopP,
			MaxTokens:       v.MaxTokens,
			MimeTypes:       v.MimeTypes,
		}

		agent := config.Agents[k]
		if v.Output != nil {
			agent.Output = &ntypes.OutputSchema{
				Name:        v.Output.Name,
				Description: v.Output.Description,
				Schema:      v.Output.ToSchema(),
				Strict:      v.Output.Strict,
			}
		}
		config.Agents[k] = agent
	}

	// Convert MCP servers
	for k, v := range manifest.MCPServers {
		config.MCPServers[k] = nmcp.Server{
			Name:        v.Name,
			ShortName:   v.ShortName,
			Description: v.Description,
			Image:       v.Image,
			Dockerfile:  v.Dockerfile,
			Source: nmcp.ServerSource{
				Repo:      v.Source.Repo,
				Tag:       v.Source.Tag,
				Commit:    v.Source.Commit,
				Branch:    v.Source.Branch,
				SubPath:   v.Source.SubPath,
				Reference: v.Source.Reference,
			},
			Sandboxed:    v.Sandboxed,
			Env:          v.Env,
			Command:      v.Command,
			Args:         v.Args,
			BaseURL:      v.BaseURL,
			Ports:        v.Ports,
			ReversePorts: v.ReversePorts,
			Cwd:          v.Cwd,
			Workdir:      v.Workdir,
			Headers:      v.Headers,
		}
	}

	return config, nil
}

func (h *handler) convertDynamicInstructions(di types.NanobotDynamicInstructions) ntypes.DynamicInstructions {
	return ntypes.DynamicInstructions{
		Instructions: di.Instructions,
		MCPServer:    di.MCPServer,
		Prompt:       di.Prompt,
		Args:         di.Args,
	}
}
