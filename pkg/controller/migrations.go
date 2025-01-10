package controller

import (
	"strings"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func removeOldFinalizers(req router.Request, _ router.Response) error {
	finalizers := req.Object.GetFinalizers()
	originalCount := len(finalizers)
	for i := 0; i < len(finalizers); i++ {
		if strings.HasPrefix(finalizers[i], "otto.otto8.ai/") {
			finalizers = append(finalizers[:i], finalizers[i+1:]...)
			i--
		}
	}

	if len(finalizers) != originalCount {
		req.Object.SetFinalizers(finalizers)
		return req.Client.Update(req.Ctx, req.Object)
	}

	return nil
}

func deleteOldModel(req router.Request, _ router.Response) error {
	if ownerGVK, ok := req.Object.GetAnnotations()[apply.LabelGVK]; ok && strings.HasPrefix(ownerGVK, "otto.otto8.ai/") {
		return req.Client.Delete(req.Ctx, req.Object)
	}

	return nil
}

func changeWorkflowStepOwnerGVK(req router.Request, _ router.Response) error {
	var update bool
	annotations := req.Object.GetAnnotations()
	if ownerGVK, ok := req.Object.GetAnnotations()[apply.LabelGVK]; ok && strings.HasPrefix(ownerGVK, "otto.otto8.ai/v1") {
		annotations[apply.LabelGVK] = strings.Replace(ownerGVK, "otto.otto8.ai/v1", v1.SchemeGroupVersion.String(), 1)
		update = true
	}

	ownerReferences := req.Object.GetOwnerReferences()
	for i, owner := range ownerReferences {
		if owner.APIVersion == "otto.otto8.ai/v1" {
			owner.APIVersion = v1.SchemeGroupVersion.String()
			ownerReferences[i] = owner
			update = true
		}
	}

	if update {
		return req.Client.Update(req.Ctx, req.Object)
	}
	return nil
}
