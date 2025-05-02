package threadshare

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func CopyProjectInfo(req router.Request, _ router.Response) error {
	share := req.Object.(*v1.ThreadShare)

	var project v1.Thread
	if err := req.Get(&project, share.Namespace, share.Spec.ProjectThreadName); err != nil {
		return err
	}

	var mcpServerList v1.MCPServerList
	if err := req.Client.List(req.Ctx, &mcpServerList,
		kclient.InNamespace(project.Namespace),
		kclient.MatchingFields{
			"spec.threadName": project.Name,
		},
	); err != nil {
		return err
	}

	mcpServers := make([]string, 0, len(mcpServerList.Items))
	for _, mcpServer := range mcpServerList.Items {
		if catalogID := mcpServer.Spec.MCPServerCatalogEntryName; catalogID != "" {
			mcpServers = append(mcpServers, catalogID)
		}
	}

	status := v1.ThreadShareStatus{
		Name:        project.Spec.Manifest.Name,
		Description: project.Spec.Manifest.Description,
		Icons:       project.Spec.Manifest.Icons,
		Tools:       project.Spec.Manifest.Tools,
		MCPServers:  mcpServers,
	}

	if !equality.Semantic.DeepEqual(status, share.Status) {
		share.Status = status
		return req.Client.Status().Update(req.Ctx, share)
	}

	return nil
}
