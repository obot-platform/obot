package tools

import (
	"context"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ResolveToolReferences(ctx context.Context, gptClient *gptscript.GPTScript, name, reference string, builtin bool, toolType v1.ToolReferenceType) ([]*v1.ToolReference, error) {
	annotations := map[string]string{
		"obot.obot.ai/timestamp": time.Now().String(),
	}

	var result []*v1.ToolReference

	prg, err := gptClient.LoadFile(ctx, reference)
	if err != nil {
		return nil, err
	}

	tool := prg.ToolSet[prg.EntryToolID]

	entryTool := v1.ToolReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   system.DefaultNamespace,
			Finalizers:  []string{v1.ToolReferenceFinalizer},
			Annotations: annotations,
		},
		Spec: v1.ToolReferenceSpec{
			Type:         toolType,
			ToolMetadata: tool.MetaData,
			Reference:    reference,
			Builtin:      builtin,
		},
	}
	result = append(result, &entryTool)

	return result, nil
}
