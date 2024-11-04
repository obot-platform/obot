package reference

import (
	"github.com/otto8-ai/nah/pkg/router"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateGlobalOAuthAppReference(req router.Request, resp router.Response) error {
	oa := req.Object.(*v1.OAuthApp)
	var existing v1.OAuthAppReference
	if err := req.Get(&existing, oa.Namespace, oa.Spec.Manifest.Integration); apierrors.IsNotFound(err) {
		return kclient.IgnoreAlreadyExists(req.Client.Create(req.Ctx, &v1.OAuthAppReference{
			ObjectMeta: metav1.ObjectMeta{
				// TODO: This will have to change when we figure out how we want to do multitenancy.
				Name: oa.Spec.Manifest.Integration,
			},

			Spec: v1.OAuthAppReferenceSpec{
				AppName:      req.Name,
				AppNamespace: req.Namespace,
			},
		}))
	} else if err != nil {
		return err
	}

	return nil
}
