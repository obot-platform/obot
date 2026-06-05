package project

import (
	"errors"
	"fmt"
	"maps"

	"github.com/obot-platform/nah/pkg/router"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gatewayClient *gclient.Client
}

func New(gatewayClient *gclient.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) MigrateProjectV2(req router.Request, _ router.Response) error {
	//nolint:staticcheck
	projectV2 := req.Object.(*v1.ProjectV2)

	if _, err := h.gatewayClient.UserByID(req.Ctx, projectV2.Spec.UserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return req.Delete(projectV2)
		}
		return fmt.Errorf("failed to get user %s: %w", projectV2.Spec.UserID, err)
	}

	project := &v1.Project{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ProjectPrefix,
			Namespace:    projectV2.Namespace,
			Labels:       maps.Clone(projectV2.Labels),
			Annotations:  maps.Clone(projectV2.Annotations),
		},
		Spec:   projectV2.Spec,
		Status: projectV2.Status,
	}

	if err := req.Client.Create(req.Ctx, project); err != nil {
		return err
	}

	var agents v1.NanobotAgentList
	if err := req.List(&agents, &kclient.ListOptions{
		Namespace: projectV2.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.projectV2ID": projectV2.Name,
		}),
	}); err != nil {
		return err
	}

	for _, agent := range agents.Items {
		agent.Spec.ProjectID = project.Name
		agent.Spec.ProjectV2ID = ""
		if err := req.Client.Update(req.Ctx, &agent); err != nil {
			return err
		}
	}

	return req.Delete(projectV2)
}
