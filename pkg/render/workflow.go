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

var (
	ValidEnv      = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")
	InvalidEnv    = regexp.MustCompile("^(OBOT|GPTSCRIPT|KNOW)")
	ValidImage    = regexp.MustCompile("^[a-zA-Z0-9_][a-zA-Z0-9_.-:/]*$")
	ValidToolType = regexp.MustCompile("^(container|script|javascript|python)$")
)

type WorkflowOptions struct {
	Step             *types.Step
	ManifestOverride *types.WorkflowManifest
	Input            string
}

func IsExternalTool(tool string) bool {
	return strings.ContainsAny(tool, ".\\/")
}

func IsValidEnv(env string) error {
	if !ValidEnv.MatchString(env) {
		return fmt.Errorf("invalid env var %s, must match %s", env, ValidEnv.String())
	}
	if InvalidEnv.MatchString(env) {
		return fmt.Errorf("invalid env var %s, cannot start with OBOT, GPTSCRIPT or KNOW", env)
	}
	return nil
}

func IsValidImage(image string) error {
	if !ValidImage.MatchString(image) {
		return fmt.Errorf("invalid image name %s, must match %s", image, ValidImage.String())
	}
	return nil
}

func IsValidToolType(toolType types.ToolType) error {
	if !ValidToolType.MatchString(string(toolType)) {
		return fmt.Errorf("invalid tool type %s, must match %s", toolType, ValidToolType.String())
	}
	return nil
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
