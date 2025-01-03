package handlers

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ToolHandler struct {
	gptScript *gptscript.GPTScript
}

func NewToolHandler(gptScript *gptscript.GPTScript) *ToolHandler {
	return &ToolHandler{gptScript: gptScript}
}

var invalidEnv = regexp.MustCompile("^(OBOT|GPTSCRIPT)")

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

	for k := range env {
		if invalidEnv.MatchString(k) {
			return types.NewErrBadRequest("invalid env key %s", k)
		}
	}

	err = t.gptScript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  thread.Name,
		ToolName: tool.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      env,
	})
	if err != nil {
		return err
	}

	var envs []string
	for k, v := range env {
		if strings.TrimSpace(v) != "" {
			envs = append(envs, k)
		}
	}
	tool.Spec.Envs = envs
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

	cred, err := t.gptScript.RevealCredential(req.Context(), []string{thread.Name}, tool.Name)
	if errors.As(err, &gptscript.ErrNotFound{}) {
		return req.Write(map[string]string{})
	} else if err != nil {
		return err
	}

	return req.Write(cred.Env)
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
