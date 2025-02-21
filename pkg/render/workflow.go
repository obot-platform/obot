package render

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var validEnv = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")

type WorkflowOptions struct {
	Step             *types.Step
	ManifestOverride *types.WorkflowManifest
	Input            string
}

func IsExternalTool(tool string) bool {
	return strings.ContainsAny(tool, ".\\/")
}

func ResolveToolReference(ctx context.Context, c kclient.Client, toolRefType types.ToolReferenceType, ns, name string) (string, error) {
	name, err := resolveToolReferenceWithMetadata(ctx, c, toolRefType, ns, name)
	return name, err
}

func resolveToolReferenceWithMetadata(ctx context.Context, c kclient.Client, toolRefType types.ToolReferenceType, ns, name string) (string, error) {
	if IsExternalTool(name) {
		return name, nil
	}

	var tool v1.ToolReference
	if err := c.Get(ctx, router.Key(ns, name), &tool); apierror.IsNotFound(err) {
		return name, nil
	} else if err != nil {
		return "", err
	}

	if toolRefType != "" && tool.Spec.Type != toolRefType {
		return name, fmt.Errorf("tool reference %s is not of type %s", name, toolRefType)
	}
	if tool.Status.Reference == "" {
		return "", fmt.Errorf("tool reference %s has no reference", name)
	}
	if toolRefType == types.ToolReferenceTypeTool {
		return fmt.Sprintf("%s as %s", tool.Status.Reference, name), nil
	}
	return tool.Status.Reference, nil
}
