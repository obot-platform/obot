package handlers

import (
	"fmt"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ToolReferenceHandler struct{}

func NewToolReferenceHandler() *ToolReferenceHandler {
	return &ToolReferenceHandler{}
}

func convertToolReference(toolRef v1.ToolReference) types.ToolReference {
	tf := types.ToolReference{
		Metadata: MetadataFrom(&toolRef),
		ToolReferenceManifest: types.ToolReferenceManifest{
			Name:      toolRef.Name,
			ToolType:  toolRef.Spec.Type,
			Reference: toolRef.Spec.Reference,
		},
		Error: toolRef.Status.Error,
	}
	if toolRef.Status.Tool != nil {
		tf.Params = toolRef.Status.Tool.Params
		tf.Name = toolRef.Status.Tool.Name
		tf.Description = toolRef.Status.Tool.Description
		tf.Metadata.Metadata = toolRef.Status.Tool.Metadata
		tf.Credential = toolRef.Status.Tool.Credential
	}
	return tf
}

func (a *ToolReferenceHandler) ByID(req api.Context) error {
	var (
		id      = req.PathValue("id")
		toolRef v1.ToolReference
	)

	if err := req.Get(&toolRef, id); err != nil {
		return err
	}

	return req.Write(convertToolReference(toolRef))
}

func (a *ToolReferenceHandler) Create(req api.Context) error {
	var (
		newToolReference types.ToolReferenceManifest
	)

	if err := req.Read(&newToolReference); err != nil {
		return err
	}

	if newToolReference.Name == "" {
		return apierrors.NewBadRequest("name is required")
	}

	if newToolReference.Reference == "" {
		return apierrors.NewBadRequest("reference is required")
	}

	switch newToolReference.ToolType {
	case types.ToolReferenceTypeTool, types.ToolReferenceTypeStepTemplate:
	default:
		return apierrors.NewBadRequest(fmt.Sprintf("invalid tool type %s", newToolReference.ToolType))
	}

	toolRef := &v1.ToolReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      newToolReference.Name,
			Namespace: req.Namespace(),
		},
		Spec: v1.ToolReferenceSpec{
			Type:      newToolReference.ToolType,
			Reference: newToolReference.Reference,
		},
	}

	if err := req.Create(toolRef); err != nil {
		return err
	}

	return req.Write(convertToolReference(*toolRef))
}

func (a *ToolReferenceHandler) Delete(req api.Context) error {
	var (
		id       = req.PathValue("id")
		toolType = req.URL.Query().Get("type")
	)
	if toolType != "" {
		var toolRef v1.ToolReference
		if err := req.Get(&toolRef, id); apierrors.IsNotFound(err) {
			return nil
		}
		if toolRef.Spec.Type != types.ToolReferenceType(toolType) {
			return apierrors.NewBadRequest(fmt.Sprintf("tool reference %s is of type %s not requested type %s", id, toolRef.Spec.Type, toolType))
		}
	}
	return req.Delete(&v1.ToolReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *ToolReferenceHandler) Update(req api.Context) error {
	var (
		id               = req.PathValue("id")
		newToolReference types.ToolReferenceManifest
		existing         v1.ToolReference
	)

	if err := req.Get(&existing, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	if err := req.Read(&newToolReference); err != nil {
		return err
	}

	if newToolReference.Reference != "" {
		existing.Spec.Reference = newToolReference.Reference
	}

	if err := req.Update(&existing); err != nil {
		return err
	}

	return req.Write(convertToolReference(existing))
}

func (a *ToolReferenceHandler) List(req api.Context) error {
	var (
		toolType    = types.ToolReferenceType(req.URL.Query().Get("type"))
		toolRefList v1.ToolReferenceList
	)

	if err := req.List(&toolRefList); err != nil {
		return err
	}

	var resp types.ToolReferenceList
	for _, toolRef := range toolRefList.Items {
		if toolType == "" || toolRef.Spec.Type == toolType {
			resp.Items = append(resp.Items, convertToolReference(toolRef))
		}
	}

	return req.Write(resp)
}
