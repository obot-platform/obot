package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/tools"
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
			Commit:    toolRef.Status.Commit,
		},
		Builtin:        toolRef.Spec.Builtin,
		Bundle:         toolRef.Spec.Bundle,
		BundleToolName: toolRef.Spec.BundleToolName,
		Error:          toolRef.Status.Error,
		Resolved:       toolRef.Generation == toolRef.Status.ObservedGeneration,
	}
	if toolRef.Spec.Active == nil {
		tf.Active = true
	} else {
		tf.Active = *toolRef.Spec.Active
	}
	if toolRef.Status.Tool != nil {
		tf.Params = toolRef.Status.Tool.Params
		tf.Name = toolRef.Status.Tool.Name
		tf.Description = toolRef.Status.Tool.Description
		tf.Metadata.Metadata = toolRef.Status.Tool.Metadata
		tf.Credentials = toolRef.Status.Tool.Credentials
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

var validCharsRegexp = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

func normalizeName(reference string) string {
	newName := validCharsRegexp.ReplaceAllString(strings.ToLower(reference), "-") // Replace invalid characters with '-'
	newName = regexp.MustCompile(`-+`).ReplaceAllString(newName, "-")
	newName = strings.Trim(newName, "-")
	if newName == "" {
		return "tool" // Fallback name if all characters are invalid
	}
	return newName
}

func (a *ToolReferenceHandler) pickNameForReference(req api.Context, reference string) (string, error) {
	newName := normalizeName(reference)
	for i := 0; i < 100; i++ {
		testName := name.SafeConcatName(newName)
		if i > 0 {
			testName = name.SafeConcatName(newName, strconv.Itoa(i))
		}
		if err := req.Get(&v1.ToolReference{}, testName); api.IsHTTPCode(err, http.StatusNotFound) {
			return testName, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not generate unique name for %s", reference)
}

func (a *ToolReferenceHandler) Create(req api.Context) (err error) {
	var (
		newToolReference types.ToolReferenceManifest
	)

	if err := req.Read(&newToolReference); err != nil {
		return err
	}

	if newToolReference.Reference == "" {
		return apierrors.NewBadRequest("reference is required")
	}

	if newToolReference.Name == "" {
		newToolReference.Name, err = a.pickNameForReference(req, newToolReference.Reference)
		if err != nil {
			return err
		}
	}

	switch newToolReference.ToolType {
	case "":
		newToolReference.ToolType = types.ToolReferenceTypeTool
	case types.ToolReferenceTypeTool:
	default:
		return apierrors.NewBadRequest(fmt.Sprintf("invalid tool type %s", newToolReference.ToolType))
	}

	toolRefs, err := tools.ResolveToolReferences(req.Context(), req.GPTClient, newToolReference.Name, newToolReference.Reference, false, newToolReference.ToolType)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("failed to resolve tool references for %s: %v", newToolReference.Reference, err))
	}

	if len(toolRefs) == 0 {
		return apierrors.NewBadRequest(fmt.Sprintf("no tool references found for %s", newToolReference.Reference))
	}

	for _, toolRef := range toolRefs {
		if err := req.Create(toolRef); err != nil && !apierrors.IsAlreadyExists(err) {
			return apierrors.NewInternalError(fmt.Errorf("failed to create tool reference %s: %w", toolRef.GetName(), err))
		}
	}

	return req.Write(convertToolReference(*toolRefs[0]))
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
	var toolRef v1.ToolReference
	if err := req.Get(&toolRef, id); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	if toolRef.Spec.Builtin {
		return types.NewErrBadRequest("cannot delete builtin tool reference %s", id)
	}

	if !toolRef.Spec.Bundle && toolRef.Spec.BundleToolName != "" {
		return types.NewErrBadRequest("cannot delete child tool that belongs to a bundle tool")
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
	existing.Spec.Active = &newToolReference.Active

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

func (a *ToolReferenceHandler) ForceRefresh(req api.Context) error {
	var (
		id       = req.PathValue("id")
		existing v1.ToolReference
	)

	if err := req.Get(&existing, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	existing.Spec.ForceRefresh = metav1.Now()

	if err := req.Update(&existing); err != nil {
		return err
	}

	return req.Write(convertToolReference(existing))
}
