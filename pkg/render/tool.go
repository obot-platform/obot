package render

import (
	"context"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/env"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CustomTool(ctx context.Context, c client.Client, tool v1.Tool) (toolDefs []gptscript.ToolDef, _ error) {
	if tool.Spec.Manifest.Name == "" {
		return nil, nil
	}

	if tool.Spec.Manifest.ToolType != "" && tool.Spec.Manifest.ToolType != "container" && tool.Spec.Manifest.Instructions == "" {
		return nil, fmt.Errorf("instructions are required for custom tools")
	}

	contextName := fmt.Sprintf("%s-context", tool.Name)

	dockerTool, err := ResolveToolReference(ctx, c, types.ToolReferenceTypeSystem, tool.Namespace, system.DockerTool)
	if err != nil {
		return nil, err
	}

	credTool, err := ResolveToolReference(ctx, c, types.ToolReferenceTypeSystem, tool.Namespace, system.ExistingCredTool)
	if err != nil {
		return nil, err
	}

	var envs []string

	var params []string
	for k, v := range tool.Spec.Manifest.Params {
		params = append(params, k, v)
		envs = append(envs, env.ToEnvLike(k))
	}

	for _, env := range tool.Spec.Envs {
		if !validEnv.MatchString(env) {
			return nil, fmt.Errorf("invalid env var %s, must match %s", env, validEnv.String())
		}
		envs = append(envs, env)
	}

	var instructions []string
	if len(envs) > 0 {
		instructions = append(instructions, fmt.Sprintf("%q as obot_tool_envs", strings.Join(envs, ",")))
	}
	if tool.Spec.Manifest.Image != "" {
		instructions = append(instructions, fmt.Sprintf("%q as obot_tool_image", tool.Spec.Manifest.Image))
	}
	if tool.Spec.Manifest.ToolType != "" && tool.Spec.Manifest.ToolType != "docker" {
		instructions = append(instructions, fmt.Sprintf("%q as obot_tool_type", tool.Spec.Manifest.ToolType))
	}

	if len(instructions) > 0 {
		instructions = append([]string{"#!sys.call", dockerTool, "with"}, instructions...)
	} else {
		instructions = []string{"#!sys.call", dockerTool}
	}

	toolDefs = []gptscript.ToolDef{{
		Name:         tool.Spec.Manifest.Name,
		Description:  tool.Spec.Manifest.Description,
		Arguments:    gptscript.ObjectSchema(params...),
		Tools:        []string{dockerTool},
		Credentials:  []string{credTool + " as " + tool.Name},
		Instructions: strings.Join(instructions, " ") + "\n" + tool.Spec.Manifest.Instructions,
	}}

	if tool.Spec.Manifest.Context != "" {
		toolDefs[0].ExportContext = []string{contextName}
		toolDefs = append(toolDefs, gptscript.ToolDef{
			Name: contextName,
			Type: "context",
			Instructions: fmt.Sprintf(`#!sys.echo
START INSTRUCTIONS: TOOL %q

%s

END INSTRUCTIONS: TOOL %q`, tool.Spec.Manifest.Name, tool.Spec.Manifest.Context, tool.Spec.Manifest.Name),
		})
	}

	return toolDefs, nil
}

func tool(ctx context.Context, c client.Client, ns, name string) (string, map[string]string, []gptscript.ToolDef, error) {
	if !system.IsToolID(name) {
		name, metadata, err := resolveToolReferenceWithMetadata(ctx, c, types.ToolReferenceTypeTool, ns, name)
		return name, metadata, nil, err
	}

	var tool v1.Tool
	if err := c.Get(ctx, router.Key(ns, name), &tool); err != nil {
		return name, nil, nil, err
	}

	toolDefs, err := CustomTool(ctx, c, tool)
	if err != nil {
		return "", nil, nil, err
	}

	if len(toolDefs) == 0 {
		return "", nil, toolDefs, nil
	}

	return toolDefs[0].Name, nil, toolDefs, nil
}
