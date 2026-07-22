package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type HarnessHandler struct{}

func NewHarnessHandler() *HarnessHandler {
	return nil
}

func (*HarnessHandler) List(req api.Context) error {
	var list v1.HarnessList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list harnesses: %w", err)
	}

	items := make([]types.Harness, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertHarness(item))
	}

	return req.Write(types.HarnessList{Items: items})
}

func (*HarnessHandler) Get(req api.Context) error {
	var harness v1.Harness
	if err := req.Get(&harness, req.PathValue("harness_id")); err != nil {
		return fmt.Errorf("failed to get harness: %w", err)
	}

	return req.Write(convertHarness(harness))
}

func (*HarnessHandler) Create(req api.Context) error {
	var manifest types.HarnessManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read harness manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid harness manifest: %v", err)
	}

	harness := v1.Harness{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.HarnessPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.HarnessSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&harness); err != nil {
		return fmt.Errorf("failed to create harness: %w", err)
	}

	return req.WriteCreated(convertHarness(harness))
}

func (*HarnessHandler) Update(req api.Context) error {
	var manifest types.HarnessManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read harness manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid harness manifest: %v", err)
	}

	var harness v1.Harness
	if err := req.Get(&harness, req.PathValue("harness_id")); err != nil {
		return fmt.Errorf("failed to get harness: %w", err)
	}

	harness.Spec.Manifest = manifest
	if err := req.Update(&harness); err != nil {
		return fmt.Errorf("failed to update harness: %w", err)
	}

	return req.Write(convertHarness(harness))
}

// Delete refuses to remove a harness that agents still run on, rather than
// leaving them pointing at nothing.
func (*HarnessHandler) Delete(req api.Context) error {
	id := req.PathValue("harness_id")

	var agents v1.HostedAgentList
	if err := req.List(&agents, kclient.MatchingFields{"spec.harnessID": id}); err != nil {
		return fmt.Errorf("failed to list hosted agents for harness %s: %w", id, err)
	}
	if count := len(agents.Items); count > 0 {
		return types.NewErrBadRequest("harness %s is in use by %d agent(s)", id, count)
	}

	return req.Delete(&v1.Harness{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func convertHarness(harness v1.Harness) types.Harness {
	return types.Harness{
		Metadata:        MetadataFrom(&harness),
		HarnessManifest: harness.Spec.Manifest,
	}
}
