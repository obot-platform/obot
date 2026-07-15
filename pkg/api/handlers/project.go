package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectHandler struct{}

func NewProjectHandler() *ProjectHandler {
	return nil
}

func (*ProjectHandler) List(req api.Context) error {
	var (
		projectList v1.ProjectList
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

	items := make([]types.Project, 0, len(projectList.Items))
	for _, project := range projectList.Items {
		items = append(items, convertProject(project))
	}
	return req.Write(types.ProjectList{Items: items})
}

func (*ProjectHandler) Create(req api.Context) error {
	var manifest types.ProjectManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	project := v1.Project{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ProjectPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ProjectSpec{
			ProjectManifest: types.ProjectManifest(manifest),
			UserID:          req.User.GetUID(),
		},
	}

	if err := req.Create(&project); err != nil {
		return err
	}

	return req.WriteCreated(convertProject(project))
}

func (*ProjectHandler) ByID(req api.Context) error {
	var project v1.Project
	if err := req.Get(&project, req.PathValue("project_id")); err != nil {
		return err
	}

	return req.Write(convertProject(project))
}

func (*ProjectHandler) Update(req api.Context) error {
	var (
		id      = req.PathValue("project_id")
		project v1.Project
	)

	if err := req.Get(&project, id); err != nil {
		return err
	}

	var manifest types.ProjectManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	project.Spec.ProjectManifest = types.ProjectManifest(manifest)
	if err := req.Update(&project); err != nil {
		return err
	}

	return req.Write(convertProject(project))
}

func (*ProjectHandler) Delete(req api.Context) error {
	var id = req.PathValue("project_id")

	return req.Delete(&v1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func convertProject(project v1.Project) types.Project {
	return types.Project{
		Metadata:        MetadataFrom(&project),
		ProjectManifest: types.ProjectManifest(project.Spec.ProjectManifest),
		UserID:          project.Spec.UserID,
	}
}
