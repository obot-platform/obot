package reference

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func AssociateWebhookWithReference(req router.Request, _ router.Response) error {
	wh := req.Object.(*v1.Webhook)
	if wh.Spec.RefName == "" {
		return nil
	}

	ref := v1.WebhookReference{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: req.Namespace,
			Name:      wh.Spec.RefName,
		},

		Spec: v1.WebhookReferenceSpec{
			WebhookName: wh.Name,
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
	return nil
}

func Cleanup(req router.Request, _ router.Response) error {
	whr := req.Object.(*v1.WebhookReference)
	if whr.Spec.WebhookName == "" {
		return kclient.IgnoreNotFound(req.Delete(whr))
	}

	var webhook v1.Webhook
	if err := req.Get(&webhook, whr.Namespace, whr.Spec.WebhookName); apierrors.IsNotFound(err) {
		return kclient.IgnoreNotFound(req.Delete(whr))
	} else if err != nil {
		return err
	}

	if webhook.Spec.RefName != whr.Name {
		return kclient.IgnoreNotFound(req.Delete(whr))
	}

	return nil
}
