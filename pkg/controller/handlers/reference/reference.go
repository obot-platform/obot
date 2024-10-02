package reference

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func AssociateWithReference(req router.Request, _ router.Response) error {
	ref := v1.Reference{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: req.Namespace,
		},
	}

	var assigned *bool
	switch v := req.Object.(type) {
	case *v1.Workflow:
		ref.Name = v.Spec.Manifest.RefName
		ref.Spec.WorkflowName = v.Name
		assigned = &v.Status.External.RefNameAssigned
	case *v1.Agent:
		ref.Name = v.Spec.Manifest.RefName
		ref.Spec.AgentName = v.Name
		assigned = &v.Status.External.RefNameAssigned
	}

	if ref.Name == "" {
		if assigned != nil {
			*assigned = false
		}
		return nil
	}

	var existingRef v1.Reference
	if err := req.Get(&existingRef, ref.Namespace, ref.Name); apierrors.IsNotFound(err) {
		if err := req.Client.Create(req.Ctx, &ref); err != nil {
			return err
		}
	} else if err != nil {
		return nil
	}

	*assigned = existingRef.Spec == ref.Spec

	return nil
}

func Cleanup(req router.Request, _ router.Response) error {
	ref := req.Object.(*v1.Reference)
	if ref.Spec.AgentName == "" && ref.Spec.WorkflowName == "" {
		return kclient.IgnoreNotFound(req.Delete(ref))
	}

	if ref.Spec.AgentName != "" {
		var agent v1.Agent
		if err := req.Get(&agent, ref.Namespace, ref.Spec.AgentName); apierrors.IsNotFound(err) {
			return kclient.IgnoreNotFound(req.Delete(ref))
		} else if err != nil {
			return err
		} else if agent.Spec.Manifest.RefName != ref.Name {
			return kclient.IgnoreNotFound(req.Delete(ref))
		}
	} else {
		var workflow v1.Workflow
		if err := req.Get(&workflow, ref.Namespace, ref.Spec.WorkflowName); apierrors.IsNotFound(err) {
			return kclient.IgnoreNotFound(req.Delete(ref))
		} else if err != nil {
			return err
		} else if workflow.Spec.Manifest.RefName != ref.Name {
			return kclient.IgnoreNotFound(req.Delete(ref))
		}
	}

	return nil
}
