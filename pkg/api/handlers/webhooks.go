package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/textproto"
	"slices"
	"strings"

	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/api"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type WebhookHandler struct{}

func NewWebhookHandler() *WebhookHandler {
	return new(WebhookHandler)
}

func (a *WebhookHandler) Update(req api.Context) error {
	var (
		id = req.PathValue("id")
		wh v1.Webhook
	)

	if err := req.Get(&wh, id); err != nil {
		return err
	}

	var manifest types.WebhookManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	if err := validateManifest(req, manifest); err != nil {
		return err
	}

	wh.Spec.WebhookManifest = manifest
	for i, h := range wh.Spec.Headers {
		wh.Spec.Headers[i] = textproto.CanonicalMIMEHeaderKey(h)
	}

	if err := req.Update(&wh); err != nil {
		return err
	}

	return req.Write(convertWebhook(wh, api.GetURLPrefix(req)))
}

func (a *WebhookHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	return req.Delete(&v1.Webhook{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *WebhookHandler) Create(req api.Context) error {
	var manifest types.WebhookManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	if err := validateManifest(req, manifest); err != nil {
		return err
	}

	wh := v1.Webhook{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WebhookPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WebhookSpec{
			WebhookManifest: manifest,
		},
	}

	for i, h := range wh.Spec.Headers {
		wh.Spec.Headers[i] = textproto.CanonicalMIMEHeaderKey(h)
	}

	if err := req.Create(&wh); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return req.Write(convertWebhook(wh, api.GetURLPrefix(req)))
}

func convertWebhook(webhook v1.Webhook, urlPrefix string) *types.Webhook {
	var links []string
	if urlPrefix != "" && webhook.Status.External.RefName != "" {
		links = []string{"invoke", fmt.Sprintf("%s/invoke/%s", urlPrefix, webhook.Status.External.RefName)}
	}

	wh := &types.Webhook{
		Metadata:                   MetadataFrom(&webhook, links...),
		WebhookManifest:            webhook.Spec.WebhookManifest,
		WebhookExternalStatus:      webhook.Status.External,
		LastSuccessfulRunCompleted: v1.NewTime(webhook.Status.LastSuccessfulRunCompleted),
	}

	wh.Secret = fmt.Sprintf("%x", sha256.Sum256([]byte(webhook.Spec.Secret)))

	return wh
}

func (a *WebhookHandler) ByID(req api.Context) error {
	var wh v1.Webhook
	if err := req.Get(&wh, req.PathValue("id")); err != nil {
		return err
	}

	return req.Write(convertWebhook(wh, api.GetURLPrefix(req)))
}

func (a *WebhookHandler) List(req api.Context) error {
	var webhookList v1.WebhookList
	if err := req.List(&webhookList); err != nil {
		return err
	}

	var resp types.WebhookList
	for _, wh := range webhookList.Items {
		resp.Items = append(resp.Items, *convertWebhook(wh, api.GetURLPrefix(req)))
	}

	return req.Write(resp)
}

func (a *WebhookHandler) Execute(req api.Context) error {
	var ref v1.WebhookReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	var webhook v1.Webhook
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: ref.Spec.WebhookNamespace, Name: ref.Spec.WebhookName}, &webhook); err != nil {
		return err
	}

	body, err := req.Body()
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	if webhook.Spec.ValidationHeader != "" {
		if err = validateSecretHeader(webhook.Spec.Secret, body, req.Request.Header.Values(webhook.Spec.ValidationHeader)); err != nil {
			req.WriteHeader(http.StatusForbidden)
			return nil
		}
	}

	var input strings.Builder
	_, _ = input.WriteString("You are being called from a webhook.\n\n")
	if len(body) > 0 {
		_, _ = input.WriteString("Here is the payload of the webhook:\n")
		_, _ = input.Write(body)
	}

	_, _ = input.WriteString("\nHere are the headers of the webhook:\n")
	allHeaders := slices.Contains(webhook.Spec.Headers, "*")
	for k := range req.Request.Header {
		if !allHeaders && !slices.Contains(webhook.Spec.Headers, k) {
			continue
		}

		input.WriteString("\n")
		input.WriteString(k)
		input.WriteString(": ")
		input.WriteString(req.Request.Header.Get(k))
	}

	workflowID := webhook.Spec.WebhookManifest.WorkflowName
	if !system.IsWorkflowID(workflowID) {
		var ref v1.Reference
		if err = req.Get(&ref, workflowID); err != nil || ref.Spec.WorkflowName == "" {
			return fmt.Errorf("failed to get workflow with ref %s: %w", workflowID, err)
		}

		workflowID = ref.Spec.WorkflowName
	}

	if err = req.Create(&v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			// The name here is the sha256 hash of the body to handle multiple executions of the same webhook.
			// That is, if the webhook is called twice with the same body, it will only be executed once.
			Name:      system.WorkflowExecutionPrefix + fmt.Sprintf("%x", sha256.Sum256(body)),
			Namespace: req.Namespace(),
		},
		Spec: v1.WorkflowExecutionSpec{
			WorkflowName: workflowID,
			WebhookName:  webhook.Name,
			Input:        input.String(),
		},
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func validateSecretHeader(secret string, body []byte, values []string) error {
	h := hmac.New(sha256.New, []byte(secret))
	for _, v := range values {
		for _, val := range strings.Split(v, ",") {
			_, val, _ = strings.Cut(val, "=")
			b, err := hex.DecodeString(strings.TrimSpace(val))
			if err != nil {
				continue
			}

			h.Reset()
			_, _ = h.Write(body)

			if hmac.Equal(h.Sum(nil), b) {
				return nil
			}
		}
	}

	return fmt.Errorf("invalid secret header")
}

func validateManifest(req api.Context, manifest types.WebhookManifest) error {
	// Ensure that the WorkflowName is set and the workflow exists
	if manifest.WorkflowName == "" {
		return apierrors.NewBadRequest(fmt.Sprintf("webhook manifest must have a workflow name"))
	}

	var workflow v1.Workflow
	if err := req.Get(&workflow, manifest.WorkflowName); types.IsNotFound(err) {
		var ref v1.Reference
		if err = req.Get(&ref, manifest.WorkflowName); err != nil || ref.Spec.WorkflowName == "" {
			return apierrors.NewBadRequest(fmt.Sprintf("workflow %s does not exist", manifest.WorkflowName))
		}
	} else if err != nil {
		return err
	}

	if (manifest.ValidationHeader != "") != (manifest.Secret != "") {
		return apierrors.NewBadRequest(fmt.Sprintf("webhook must have secret and header set together"))
	}

	return nil
}
