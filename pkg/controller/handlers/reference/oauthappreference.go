package reference

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateGlobalOAuthAppReference(req router.Request, resp router.Response) error {
	oa := req.Object.(*v1.OAuthApp)
	// Always create an oauth app reference for this webhook.
	resp.Objects(
		&v1.OAuthAppReference{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: This will have to change when we figure out how we want to do multitenancy.
				Name: oa.Spec.Manifest.Integration,
			},

			Spec: v1.OAuthAppReferenceSpec{
				AppName:      req.Name,
				AppNamespace: req.Namespace,
			},
		},
	)
	return nil
}
