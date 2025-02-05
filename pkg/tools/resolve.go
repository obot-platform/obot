package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ResolveToolReferences(ctx context.Context, gptClient *gptscript.GPTScript, name, reference string, builtin bool, toolType types.ToolReferenceType) ([]client.Object, error) {
	annotations := map[string]string{
		"obot.obot.ai/timestamp": time.Now().String(),
	}

	var result []client.Object

	prg, err := gptClient.LoadFile(ctx, reference)
	if err != nil {
		return nil, err
	}

	tool := prg.ToolSet[prg.EntryToolID]
	isCapability := tool.MetaData["category"] == "Capability"
	isBundleTool := false
	if len(tool.LocalTools) > 1 {
		isBundleTool = true
	}

	toolName := resolveToolReferenceName(toolType, isBundleTool, isCapability, name, tool.Name)
	entryTool := v1.ToolReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:        toolName,
			Namespace:   system.DefaultNamespace,
			Finalizers:  []string{v1.ToolReferenceFinalizer},
			Annotations: annotations,
		},
		Spec: v1.ToolReferenceSpec{
			Type:      toolType,
			Reference: reference,
			Builtin:   builtin,
			Bundle:    isBundleTool,
		},
	}
	result = append(result, &entryTool)

	if isCapability {
		return result, nil
	}

	for _, peerToolID := range tool.LocalTools {
		if peerToolID == prg.EntryToolID {
			continue
		}

		peerTool := prg.ToolSet[peerToolID]
		if isValidTool(peerTool) {
			toolName = resolveToolReferenceName(toolType, false, peerTool.MetaData["category"] == "Capability", name, peerTool.Name)
			result = append(result, &v1.ToolReference{
				ObjectMeta: metav1.ObjectMeta{
					Name:        toolName,
					Namespace:   system.DefaultNamespace,
					Finalizers:  []string{v1.ToolReferenceFinalizer},
					Annotations: annotations,
				},
				Spec: v1.ToolReferenceSpec{
					Type:           toolType,
					Reference:      fmt.Sprintf("%s from %s", peerTool.Name, reference),
					Builtin:        builtin,
					BundleToolName: entryTool.Name,
				},
			})
		}
	}

	return result, nil
}

func resolveToolReferenceName(toolType types.ToolReferenceType, isBundle bool, isCapability bool, toolName, subToolName string) string {
	if toolType == types.ToolReferenceTypeTool {
		if isBundle {
			if isCapability {
				return toolName
			}
			return toolName + "-bundle"
		}

		if subToolName == "" {
			return toolName
		}
		return normalize(toolName, subToolName)
	}

	return toolName
}

func normalize(names ...string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.Join(names, "-"), " ", "-"), "_", "-"))
}

func isValidTool(tool gptscript.Tool) bool {
	if tool.MetaData["index"] == "false" {
		return false
	}
	return tool.Name != "" && (tool.Type == "" || tool.Type == "tool")
}
