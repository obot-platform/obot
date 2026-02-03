package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectV2Handler struct{}

func NewProjectV2Handler() *ProjectV2Handler {
	return &ProjectV2Handler{}
}

func (h *ProjectV2Handler) List(req api.Context) error {
	var (
		projectList v1.ProjectV2List
		fields      = kclient.MatchingFields{}
	)

	// By default, filter by user. Admins can use ?all=true to see all projects.
	all := (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true"
	if !all {
		fields["spec.userID"] = req.User.GetUID()
	}

	if err := req.List(&projectList, fields); err != nil {
		return err
	}

	items := make([]types.ProjectV2, 0, len(projectList.Items))
	for _, project := range projectList.Items {
		items = append(items, convertProjectV2(project))
	}
	return req.Write(types.ProjectV2List{Items: items})
}

func (h *ProjectV2Handler) Create(req api.Context) error {
	var manifest types.ProjectV2Manifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	project := v1.ProjectV2{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ProjectV2Prefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ProjectV2Spec{
			ProjectV2Manifest: manifest,
			UserID:            req.User.GetUID(),
		},
	}

	if err := req.Create(&project); err != nil {
		return err
	}

	return req.WriteCreated(convertProjectV2(project))
}

func (h *ProjectV2Handler) ByID(req api.Context) error {
	var project v1.ProjectV2
	if err := req.Get(&project, req.PathValue("projectv2_id")); err != nil {
		return err
	}

	return req.Write(convertProjectV2(project))
}

func (h *ProjectV2Handler) Update(req api.Context) error {
	var (
		id      = req.PathValue("projectv2_id")
		project v1.ProjectV2
	)

	if err := req.Get(&project, id); err != nil {
		return err
	}

	var manifest types.ProjectV2Manifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	project.Spec.ProjectV2Manifest = manifest
	if err := req.Update(&project); err != nil {
		return err
	}

	return req.Write(convertProjectV2(project))
}

func (h *ProjectV2Handler) Delete(req api.Context) error {
	var id = req.PathValue("projectv2_id")

	return req.Delete(&v1.ProjectV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func convertProjectV2(project v1.ProjectV2) types.ProjectV2 {
	return types.ProjectV2{
		Metadata:          MetadataFrom(&project),
		ProjectV2Manifest: project.Spec.ProjectV2Manifest,
		UserID:            project.Spec.UserID,
	}
}
