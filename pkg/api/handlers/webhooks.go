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

	// Ensure that the WorkflowName is set and the workflow exists
	if manifest.WorkflowName == "" {
		return apierrors.NewBadRequest(fmt.Sprintf("webhook manifest must have a workflow name"))
	}

	var workflow v1.Workflow
	if err := req.Get(&workflow, manifest.WorkflowName); apierrors.IsNotFound(err) {
		return apierrors.NewBadRequest(fmt.Sprintf("workflow %s does not exist", manifest.WorkflowName))
	} else if err != nil {
		return err
	}

	if (manifest.ValidationHeader != "") != (manifest.Secret != "") {
		return apierrors.NewBadRequest(fmt.Sprintf("webhook must have secret and header set together"))
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

	// Ensure that the WorkflowName is set and the workflow exists
	if manifest.WorkflowName == "" {
		return apierrors.NewBadRequest(fmt.Sprintf("webhook manifest must have a workflow name"))
	}

	var workflow v1.Workflow
	if err := req.Get(&workflow, manifest.WorkflowName); apierrors.IsNotFound(err) {
		return apierrors.NewBadRequest(fmt.Sprintf("workflow %s does not exist", manifest.WorkflowName))
	} else if err != nil {
		return err
	}

	if (manifest.ValidationHeader != "") != (manifest.Secret != "") {
		return apierrors.NewBadRequest(fmt.Sprintf("webhook must have secret and header set together"))
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

func convertWebhook(webhook v1.Webhook, prefix string) *types.Webhook {
	var links []string
	if prefix != "" {
		refName := webhook.Name
		if webhook.Status.External.RefNameAssigned && webhook.Spec.RefName != "" {
			refName = webhook.Spec.RefName
		}
		links = []string{"invoke", prefix + "/webhooks/" + refName}
	}

	wh := &types.Webhook{
		Metadata:              MetadataFrom(&webhook, links...),
		WebhookManifest:       webhook.Spec.WebhookManifest,
		WebhookExternalStatus: webhook.Status.External,
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
	whID := req.PathValue("id")
	if !system.IsWebhookID(whID) {
		var ref v1.WebhookReference
		if err := req.Get(&ref, whID); err != nil {
			return err
		}
		whID = ref.Spec.WebhookName
	}

	var webhook v1.Webhook
	if err := req.Get(&webhook, whID); err != nil {
		return err
	}

	input, err := req.Body()
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	if webhook.Spec.ValidationHeader != "" {
		if err = validateSecretHeader(webhook.Spec.Secret, input, req.Request.Header.Values(webhook.Spec.ValidationHeader)); err != nil {
			req.WriteHeader(http.StatusForbidden)
			return nil
		}
	}

	allHeaders := slices.Contains(webhook.Spec.Headers, "*")
	headers := make(map[string]string, len(webhook.Spec.Headers))
	for k := range req.Request.Header {
		if allHeaders {
			headers[k] = req.Request.Header.Get(k)
		} else if slices.Contains(webhook.Spec.Headers, k) {
			headers[k] = req.Request.Header.Get(k)
		}
	}

	if err = req.Create(&v1.WebhookExecution{
		ObjectMeta: metav1.ObjectMeta{
			// The name here is the sha256 hash of the input to handle multiple executions of the same webhook.
			// That is, if the webhook is called twice with the same input, it will only be executed once.
			Name:      fmt.Sprintf("%x", sha256.Sum256(input)),
			Namespace: req.Namespace(),
		},
		Spec: v1.WebhookExecutionSpec{
			WebhookName: webhook.Name,
			Payload:     string(input),
			Headers:     headers,
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
