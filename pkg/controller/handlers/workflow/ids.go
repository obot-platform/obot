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
	return step
}
