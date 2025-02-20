package controller

import (
	"strings"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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

func setWorkflowAdditionalCredentialContexts(req router.Request, _ router.Response) error {
	wf := req.Object.(*v1.Workflow)

	if len(wf.Spec.AdditionalCredentialContexts) != 0 || wf.Spec.ThreadName == "" {
		return nil
	}

	var thread v1.Thread
	if err := req.Client.Get(req.Ctx, kclient.ObjectKey{Namespace: wf.Namespace, Name: wf.Spec.ThreadName}, &thread); err != nil {
		return err
	}

	if thread.Spec.AgentName == "" {
		return nil
	}

	wf.Spec.AdditionalCredentialContexts = []string{thread.Spec.AgentName}
	if err := req.Client.Update(req.Ctx, wf); err != nil {
		return err
	}

	return nil
}
