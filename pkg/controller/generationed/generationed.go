package generationed

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// UpdateObservedGeneration should be the last handler that runs on such an object to ensure
// that updating of the observed generation only happens if no error occurs.
func UpdateObservedGeneration(req router.Request, resp router.Response) error {
	if req.Object == nil {
		return nil
	}

	if errored, ok := resp.Attributes()["generation:errored"]; !ok || errored != true {
		req.Object.(v1.Generationed).SetObservedGeneration(req.Object.GetGeneration())
	}

	return nil
}
