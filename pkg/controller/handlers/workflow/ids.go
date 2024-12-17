package workflow

import (
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/obot/apiclient/types"
)

func PopulateIDs(manifest types.WorkflowManifest) types.WorkflowManifest {
	manifest = *manifest.DeepCopy()
	ids := map[string]struct{}{}
	for i, step := range manifest.Steps {
		manifest.Steps[i] = populateStepID(ids, step)
	}
	return manifest
}

func nextID(seen map[string]struct{}) string {
	for {
		next, err := randomtoken.Generate()
		if err != nil {
			panic(err)
		}
		id := "si1" + next[:5]
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			return id
		}
	}
}

func populateStepID(seen map[string]struct{}, step types.Step) types.Step {
	if step.ID == "" {
		step.ID = nextID(seen)
	} else if _, ok := seen[step.ID]; ok {
		step.ID = nextID(seen)
	}

	if step.While != nil {
		step.While = populateWhileID(seen, *step.While)
	}
	if step.If != nil {
		step.If = populateIfID(seen, *step.If)
	}
	return step
}

func populateWhileID(seen map[string]struct{}, while types.While) *types.While {
	for i, step := range while.Steps {
		while.Steps[i] = populateStepID(seen, step)
	}
	return &while
}

func populateIfID(seen map[string]struct{}, ifStep types.If) *types.If {
	for i, step := range ifStep.Steps {
		ifStep.Steps[i] = populateStepID(seen, step)
	}
	for i, step := range ifStep.Else {
		ifStep.Else[i] = populateStepID(seen, step)
	}
	return &ifStep
}
