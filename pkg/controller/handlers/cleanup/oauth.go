package cleanup

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func OAuthClients(req router.Request, resp router.Response) error {
	o := req.Object.(*v1.OAuthClient)
	if until := time.Until(o.Spec.RegistrationTokenExpiresAt.Time); until < 0 {
		// Expired. Delete it.
		return req.Delete(o)
	} else if until < 10*time.Hour {
		// If the token expires within 10 hours, retry the request.
		// Otherwise, the token will get re-enqueued when the controller re-enqueues everything.
		resp.RetryAfter(until)
	}
	return nil
}

func OAuthAuthRequests(req router.Request, resp router.Response) error {
	o := req.Object.(*v1.OAuthAuthRequest)

	since := time.Since(o.CreationTimestamp.Time)
	if since > time.Hour {
		// Expired. Delete it.
		return req.Delete(o)
	}

	resp.RetryAfter(time.Hour - since)

	return nil
}
