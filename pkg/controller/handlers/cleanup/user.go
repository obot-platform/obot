package cleanup

import (
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func User(req router.Request, _ router.Response) error {
	userDelete := req.Object.(*v1.UserDelete)
	var projects v1.ThreadList
	if err := req.List(&projects, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userUID": strconv.FormatUint(uint64(userDelete.Spec.UserID), 10),
		}),
	}); err != nil {
		return err
	}

	for _, project := range projects.Items {
		if project.Spec.Project {
			if err := req.Delete(&project); err != nil {
				return err
			}
		}
	}

	// If everything is cleaned up successfully, then delete this object because we don't need it.
	return req.Delete(userDelete)
}
