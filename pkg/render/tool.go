package render

import (
	"context"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Tool(ctx context.Context, c client.Client, ns, name string) (_ string, toolDefs []gptscript.ToolDef, _ error) {
	if !system.IsToolID(name) {
		name, err := ResolveToolReference(ctx, c, types.ToolReferenceTypeTool, ns, name)
		return name, nil, err
	}

	var tool v1.Tool
	if err := c.Get(ctx, router.Key(ns, name), &tool); err != nil {
		return name, nil, err
	}

	contextName := fmt.Sprintf("%s-context", tool.Name)

	dockerTool, err := ResolveToolReference(ctx, c, types.ToolReferenceTypeSystem, ns, system.DockerTool)
	if err != nil {
		return name, nil, err
	}

	credTool, err := ResolveToolReference(ctx, c, types.ToolReferenceTypeSystem, ns, system.ExistingCredTool)
	if err != nil {
		return name, nil, err
	}

	params := []string{}
	for k, v := range tool.Spec.Manifest.Params {
		params = append(params, k, v)
	}

	instructions := []string{"#!sys.call",
		dockerTool,
	}
	if len(tool.Spec.Envs) > 0 {
		instructions = append(instructions, strings.Join(tool.Spec.Envs, ","), "as", "envs")
	}

	toolDefs = append(toolDefs, gptscript.ToolDef{
		Name:          tool.Spec.Manifest.Name,
		Description:   tool.Spec.Manifest.Description,
		Arguments:     gptscript.ObjectSchema(params...),
		Tools:         []string{dockerTool},
		ExportContext: []string{contextName},
		Credentials:   []string{credTool + " as " + tool.Name},
		Instructions:  strings.Join(instructions, " ") + "\n" + tool.Spec.Manifest.Instructions,
	}, gptscript.ToolDef{
		Name:         contextName,
		Type:         "context",
		Instructions: "#!sys.echo\n" + tool.Spec.Manifest.Context,
	})
	return toolDefs[0].Name, toolDefs, nil
}
