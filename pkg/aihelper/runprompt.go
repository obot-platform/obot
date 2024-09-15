package aihelper

import (
	"context"
	"encoding/json"

	"github.com/acorn-io/baaah/pkg/typed"
	"github.com/gptscript-ai/go-gptscript"
)

type AIHelper struct {
	gptscript *gptscript.GPTScript
	modelName string
}

func New(gptscript *gptscript.GPTScript, modelName string) *AIHelper {
	return &AIHelper{
		gptscript: gptscript,
		modelName: modelName,
	}
}

func (a *AIHelper) GenerateObject(ctx context.Context, output any, instructions, input string) error {
	_, isString := output.(*string)
	run, err := a.gptscript.Evaluate(ctx, gptscript.Options{
		Input: input,
	}, gptscript.ToolDef{
		ModelName:    a.modelName,
		JSONResponse: !isString,
		Instructions: instructions,
		Temperature:  typed.Pointer((float32)(0.7)),
	})
	if err != nil {
		return err
	}

	text, err := run.Text()
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(text), output)
}
