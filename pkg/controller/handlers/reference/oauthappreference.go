package reference

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func AssociateOAuthAppWithReference(req router.Request, resp router.Response) error {
	oa := req.Object.(*v1.OAuthApp)
	// Always create an oauth app reference for this webhook.
	resp.Objects(
		&v1.OAuthAppReference{
			ObjectMeta: metav1.ObjectMeta{
				Name: oa.Namespace + "-" + oa.Name,
			},

			Spec: v1.OAuthAppReferenceSpec{
				AppName:      oa.Name,
				AppNamespace: oa.Namespace,
			},
		},
	)

	oa.Status.External.RefNameAssigned = false
	oa.Status.External.RefName = oa.Namespace + "-" + oa.Name

	if oa.Spec.RefName == "" {
		return nil
	}

	ref := v1.OAuthAppReference{
		ObjectMeta: metav1.ObjectMeta{
			Name: oa.Spec.RefName,
		},

		Spec: v1.OAuthAppReferenceSpec{
			AppName:      oa.Name,
			AppNamespace: oa.Namespace,
			Custom:       true,
		},
	}

	var existingRef v1.OAuthAppReference
	if err := req.Get(&existingRef, ref.Namespace, ref.Name); apierrors.IsNotFound(err) {
		if err = req.Client.Create(req.Ctx, &ref); err != nil {
			return err
		}
	} else if err != nil {
		return nil
	}

	oa.Status.External.RefNameAssigned = existingRef.Spec == ref.Spec
	if oa.Status.External.RefNameAssigned {
		oa.Status.External.RefName = existingRef.Name
	}
	return nil
}

func CleanupOAuthApp(req router.Request, _ router.Response) error {
	oar := req.Object.(*v1.OAuthAppReference)
	if oar.Spec.AppName == "" || oar.Spec.AppNamespace == "" {
		return kclient.IgnoreNotFound(req.Delete(oar))
	}

	var app v1.OAuthApp
	if err := req.Get(&app, oar.Spec.AppNamespace, oar.Spec.AppName); apierrors.IsNotFound(err) {
		return kclient.IgnoreNotFound(req.Delete(oar))
	} else if err != nil {
		return err
	}

	// If this is not a "custom" app reference, then this is the "standard" app reference is that is associated to every
	// app. We don't want to delete it here because it will be deleted when the app is deleted.
	if oar.Spec.Custom && app.Spec.RefName != oar.Name {
		return kclient.IgnoreNotFound(req.Delete(oar))
	}

	return nil
}
