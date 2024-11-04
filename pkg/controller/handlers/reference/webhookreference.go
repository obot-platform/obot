package reference

import (
	"github.com/otto8-ai/nah/pkg/router"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func AssociateWebhookWithReference(req router.Request, resp router.Response) error {
	wh := req.Object.(*v1.Webhook)

	// Always create a webhook reference for this webhook.
	var standardWebhookRef v1.WebhookReference
	if err := req.Get(&standardWebhookRef, "", wh.Namespace+"-"+wh.Name); apierrors.IsNotFound(err) {
		if err = req.Client.Create(req.Ctx, &v1.WebhookReference{
			ObjectMeta: metav1.ObjectMeta{
				Name: wh.Namespace + "-" + wh.Name,
			},

			Spec: v1.WebhookReferenceSpec{
				WebhookName:      wh.Name,
				WebhookNamespace: wh.Namespace,
			},
		}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	} else if err != nil {
		return err
	}

	wh.Status.External.RefNameAssigned = false
	wh.Status.External.RefName = wh.Namespace + "-" + wh.Name

	if wh.Spec.RefName == "" {
		return nil
	}

	ref := v1.WebhookReference{
		ObjectMeta: metav1.ObjectMeta{
			Name: wh.Spec.RefName,
		},

		Spec: v1.WebhookReferenceSpec{
			WebhookName:      wh.Name,
			WebhookNamespace: wh.Namespace,
		},
	}

	var existingRef v1.WebhookReference
	if err := req.Get(&existingRef, ref.Namespace, ref.Name); apierrors.IsNotFound(err) {
		if err = req.Client.Create(req.Ctx, &ref); err != nil {
			return err
		}
	} else if err != nil {
		return nil
	}

	wh.Status.External.RefNameAssigned = existingRef.Spec == ref.Spec
	if wh.Status.External.RefNameAssigned {
		wh.Status.External.RefName = existingRef.Name
	}
	return nil
}

func CleanupWebhook(req router.Request, _ router.Response) error {
	whr := req.Object.(*v1.WebhookReference)
	if whr.Spec.WebhookName == "" || whr.Spec.WebhookNamespace == "" {
		return kclient.IgnoreNotFound(req.Delete(whr))
	}

	var webhook v1.Webhook
	if err := req.Get(&webhook, whr.Spec.WebhookNamespace, whr.Spec.WebhookName); apierrors.IsNotFound(err) {
		return kclient.IgnoreNotFound(req.Delete(whr))
	} else if err != nil {
		return err
	}

	// If the reference no longer matches this webhook, then delete it.
	if whr.Spec.Custom && webhook.Spec.RefName != whr.Name {
		return kclient.IgnoreNotFound(req.Delete(whr))
	}

	return nil
}
