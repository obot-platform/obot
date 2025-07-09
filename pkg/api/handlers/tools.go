package handlers

import (
	"errors"
	"maps"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type ToolHandler struct {
	invoke *invoke.Invoker
}

func NewToolHandler(invoke *invoke.Invoker) *ToolHandler {
	return &ToolHandler{
		invoke: invoke,
	}
}

func setEnvMap(req api.Context, threadName, toolName string, env map[string]string) error {
	for k := range env {
		if err := render.IsValidEnv(k); err != nil {
			return types.NewErrBadRequest("%v", err)
		}
	}

	return req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  threadName,
		ToolName: toolName,
		Type:     gptscript.CredentialTypeTool,
		Env:      env,
	})
}

func (t *ToolHandler) SetEnv(req api.Context) error {
	toolID := req.PathValue("tool_id")
	env := map[string]string{}

	if err := req.Read(&env); err != nil {
		return err
	}

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var tool v1.Tool
	if err = req.Get(&tool, toolID); err != nil {
		return err
	}

	if tool.Spec.ThreadName != thread.Name {
		return types.NewErrNotFound("tool %s not found", toolID)
	}

	if err = setEnvMap(req, thread.Name, tool.Name, env); err != nil {
		return err
	}

	tool.Spec.Envs = slices.Collect(maps.Keys(env))
	if err = req.Update(&tool); err != nil {
		return err
	}

	return req.Write(env)
}

func (t *ToolHandler) GetEnv(req api.Context) error {
	toolID := req.PathValue("tool_id")

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var tool v1.Tool
	if err = req.Get(&tool, toolID); err != nil {
		return err
	}

	if tool.Spec.ThreadName != thread.Name {
		return types.NewErrNotFound("tool %s not found", toolID)
	}

	data, err := getEnvMap(req, req.GPTClient, thread.Name, tool.Name)
	if err != nil {
		return err
	}

	return req.Write(data)
}

func getEnvMap(req api.Context, gptScript *gptscript.GPTScript, threadName, toolName string) (map[string]string, error) {
	cred, err := gptScript.RevealCredential(req.Context(), []string{threadName}, toolName)
	if errors.As(err, &gptscript.ErrNotFound{}) {
		return map[string]string{}, nil
	} else if err != nil {
		return nil, err
	}

	return cred.Env, nil
}

func (t *ToolHandler) Get(req api.Context) error {
	toolID := req.PathValue("tool_id")

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var tool v1.Tool
	if err = req.Get(&tool, toolID); err != nil {
		return err
	}

	if tool.Spec.ThreadName != thread.Name {
		return types.NewErrNotFound("tool %s not found", toolID)
	}

	return req.Write(convertTool(tool, slices.Contains(thread.Spec.Manifest.Tools, tool.Name)))
}

type TestInput struct {
	Input map[string]string    `json:"input"`
	Tool  *types.AssistantTool `json:"tool"`
	Env   map[string]string    `json:"env,omitempty"`
}

func convertTool(tool v1.Tool, enabled bool) types.AssistantTool {
	return types.AssistantTool{
		Metadata:     MetadataFrom(&tool),
		ToolManifest: tool.Spec.Manifest,
		Enabled:      enabled,
	}
}
