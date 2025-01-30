package handlers

import (
	"context"
	"errors"
	"maps"
	"regexp"
	"slices"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ToolHandler struct {
	gptScript *gptscript.GPTScript
	invoke    *invoke.Invoker
}

func NewToolHandler(gptScript *gptscript.GPTScript, invoke *invoke.Invoker) *ToolHandler {
	return &ToolHandler{
		gptScript: gptScript,
		invoke:    invoke,
	}
}

var invalidEnv = regexp.MustCompile("^(OBOT|GPTSCRIPT|KNOW)")

func setEnvMap(req api.Context, gptScript *gptscript.GPTScript, threadName, toolName string, env map[string]string) error {
	for k := range env {
		if invalidEnv.MatchString(k) {
			return types.NewErrBadRequest("invalid env key %s", k)
		}
	}

	return gptScript.CreateCredential(req.Context(), gptscript.Credential{
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
	if err := req.Get(&tool, toolID); err != nil {
		return err
	}

	if tool.Spec.ThreadName != thread.Name {
		return types.NewErrNotFound("tool %s not found", toolID)
	}

	if err := setEnvMap(req, t.gptScript, thread.Name, tool.Name, env); err != nil {
		return err
	}

	tool.Spec.Envs = slices.Collect(maps.Keys(env))
	if err := req.Update(&tool); err != nil {
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
	if err := req.Get(&tool, toolID); err != nil {
		return err
	}

	if tool.Spec.ThreadName != thread.Name {
		return types.NewErrNotFound("tool %s not found", toolID)
	}

	data, err := getEnvMap(req, t.gptScript, thread.Name, tool.Name)
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
	if err := req.Get(&tool, toolID); err != nil {
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

func (t *ToolHandler) Test(req api.Context) error {
	var (
		toolID      = req.PathValue("tool_id")
		agent       v1.Agent
		envs        []string
		envNameList []string
		testID      = system.ToolPrefix + "-test-cred"
	)

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
		return err
	}

	for _, env := range agent.Spec.Manifest.Env {
		if env.Name != "" && env.Value != "" {
			envs = append(envs, env.Name+"="+env.Value)
		}
	}

	var tool v1.Tool
	if err := req.Get(&tool, toolID); err != nil {
		return err
	}

	if tool.Spec.ThreadName != thread.Name {
		return types.NewErrNotFound("tool %s not found", toolID)
	}

	var input TestInput
	if err := req.Read(&input); err != nil {
		return err
	}

	if len(input.Env) > 0 {
		for envName := range input.Env {
			if invalidEnv.MatchString(envName) {
				return types.NewErrBadRequest("invalid env key %s", envName)
			}
			envNameList = append(envNameList, envName)
		}
		err := t.gptScript.CreateCredential(req.Context(), gptscript.Credential{
			Context:  thread.Name,
			ToolName: testID,
			Type:     gptscript.CredentialTypeTool,
			Env:      input.Env,
		})
		if err != nil {
			return err
		}
		defer func() {
			_ = t.gptScript.DeleteCredential(req.Context(), thread.Name, testID)
		}()
	}

	if input.Tool != nil {
		tool.Spec.Manifest = input.Tool.ToolManifest
	}

	tool.Spec.Manifest.Name = testID
	tool.Spec.Envs = envNameList

	tools, err := render.CustomTool(req.Context(), req.Storage, tool)
	if err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(req.Context(), 1*time.Minute)
	defer cancel()

	result, err := t.invoke.EphemeralThreadTask(timeoutCtx, thread, tools, input.Input, invoke.SystemTaskOptions{
		Env: envs,
	})
	if err != nil {
		return err
	}

	return req.Write(map[string]string{"output": result})
}

func (t *ToolHandler) Create(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var manifest types.ToolManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	tool := v1.Tool{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ToolPrefix,
			Namespace:    thread.Namespace,
		},
		Spec: v1.ToolSpec{
			ThreadName: thread.Name,
			Manifest:   manifest,
		},
	}

	if err := req.Create(&tool); err != nil {
		return err
	}

	thread.Spec.Manifest.Tools = append(thread.Spec.Manifest.Tools, tool.Name)
	if err := req.Update(thread); err != nil {
		return err
	}

	return req.WriteCreated(convertTool(tool, true))
}

func convertTool(tool v1.Tool, enabled bool) types.AssistantTool {
	return types.AssistantTool{
		Metadata:     MetadataFrom(&tool),
		ToolManifest: tool.Spec.Manifest,
		Enabled:      enabled,
	}
}
