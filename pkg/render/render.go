package render

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func ResolveToolReference(ctx context.Context, c kclient.Client, toolRefType v1.ToolReferenceType, ns, name string) (string, error) {
	if strings.ContainsAny(name, " .\\/") {
		return name, nil
	}

	var tool v1.ToolReference
	if err := c.Get(ctx, router.Key(ns, name), &tool); apierrors.IsNotFound(err) {
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
	return tool.Status.Reference, nil
}
