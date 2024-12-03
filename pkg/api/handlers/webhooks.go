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

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/alias"
	"github.com/otto8-ai/otto8/pkg/api"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"golang.org/x/crypto/bcrypt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	WebhookTokenHTTPHeader = "X-Otto8-Webhook-Token"
	WebhookTokenQueryParam = "token"
)

type WebhookHandler struct{}

func NewWebhookHandler() *WebhookHandler {
	return new(WebhookHandler)
}

type webhookRequest struct {
	types.WebhookManifest `json:",inline"`
	Token                 string `json:"token"`
}

func (a *WebhookHandler) Update(req api.Context) error {
	var (
		id = req.PathValue("id")
		wh v1.Webhook
	)

	if err := req.Get(&wh, id); err != nil {
		return err
	}

	var webhookReq webhookRequest
	if err := req.Read(&webhookReq); err != nil {
		return err
	}

	if webhookReq.WebhookManifest.ValidationHeader != "" && webhookReq.WebhookManifest.Secret == "" {
		webhookReq.WebhookManifest.Secret = wh.Spec.Secret
	}

	if err := validateManifest(req, webhookReq.WebhookManifest); err != nil {
		return err
	}

	if webhookReq.Token != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(webhookReq.Token), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		wh.Spec.TokenHash = hash
		webhookReq.Token = ""
	}

	wh.Spec.WebhookManifest = webhookReq.WebhookManifest
	for i, h := range wh.Spec.Headers {
		wh.Spec.Headers[i] = textproto.CanonicalMIMEHeaderKey(h)
	}

	if err := req.Update(&wh); err != nil {
		return err
	}

	return req.Write(convertWebhook(wh, req.APIBaseURL))
}

func (a *WebhookHandler) Delete(req api.Context) error {
	return req.Delete(&v1.Webhook{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("id"),
			Namespace: req.Namespace(),
		},
	})
}

func (a *WebhookHandler) Create(req api.Context) error {
	var webhookReq webhookRequest
	if err := req.Read(&webhookReq); err != nil {
		return err
	}

	if err := validateManifest(req, webhookReq.WebhookManifest); err != nil {
		return err
	}

	wh := &v1.Webhook{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WebhookPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WebhookSpec{
			WebhookManifest: webhookReq.WebhookManifest,
		},
	}

	if webhookReq.Token != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(webhookReq.Token), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		webhookReq.Token = ""
		wh.Spec.TokenHash = hash
	}

	for i, h := range wh.Spec.Headers {
		wh.Spec.Headers[i] = textproto.CanonicalMIMEHeaderKey(h)
	}

	if err := req.Create(wh); err != nil {
		return err
	}

	return req.WriteCreated(convertWebhook(*wh, req.APIBaseURL))
}

func convertWebhook(webhook v1.Webhook, urlPrefix string) *types.Webhook {
	var links []string
	if urlPrefix != "" {
		path := webhook.Name
		if webhook.Status.AliasAssigned {
			path = webhook.Spec.Alias
		}
		links = []string{"invoke", fmt.Sprintf("%s/webhooks/%s/%s", urlPrefix, webhook.Namespace, path)}
	}

	var aliasAssigned *bool
	if webhook.Generation == webhook.Status.AliasObservedGeneration {
		aliasAssigned = &webhook.Status.AliasAssigned
	}

	manifest := webhook.Spec.WebhookManifest
	wh := &types.Webhook{
		Metadata:                   MetadataFrom(&webhook, links...),
		WebhookManifest:            manifest,
		AliasAssigned:              aliasAssigned,
		LastSuccessfulRunCompleted: v1.NewTime(webhook.Status.LastSuccessfulRunCompleted),
		HasToken:                   len(webhook.Spec.TokenHash) > 0,
	}

	if webhook.Spec.Secret != "" {
		wh.Secret = fmt.Sprintf("%x", sha256.Sum256([]byte(webhook.Spec.Secret)))
	}

	return wh
}

func (a *WebhookHandler) ByID(req api.Context) error {
	var wh v1.Webhook
	if err := alias.Get(req.Context(), req.Storage, &wh, req.Namespace(), req.PathValue("id")); err != nil {
		return err
	}

	return req.Write(convertWebhook(wh, req.APIBaseURL))
}

func (a *WebhookHandler) List(req api.Context) error {
	var webhookList v1.WebhookList
	if err := req.List(&webhookList); err != nil {
		return err
	}

	var resp types.WebhookList
	for _, wh := range webhookList.Items {
		resp.Items = append(resp.Items, *convertWebhook(wh, req.APIBaseURL))
	}

	return req.Write(resp)
}

func (a *WebhookHandler) RemoveToken(req api.Context) error {
	// There is a chance that an unauthorized user could sneak through our authorization because of the pattern matching we are using.
	// Check that the user is an admin here.
	if !req.UserIsAdmin() {
		return types.NewErrHttp(http.StatusForbidden, "unauthorized")
	}

	var wh v1.Webhook
	if err := req.Get(&wh, req.PathValue("id")); err != nil {
		return err
	}

	wh.Spec.TokenHash = nil
	if err := req.Update(&wh); err != nil {
		return fmt.Errorf("failed to remove token: %w", err)
	}

	return req.Write(convertWebhook(wh, req.APIBaseURL))
}

func (a *WebhookHandler) Execute(req api.Context) error {
	var webhook v1.Webhook
	if err := alias.Get(req.Context(), req.Storage, &webhook, req.PathValue("namespace"), req.PathValue("id")); err != nil {
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

	if webhook.Spec.TokenHash != nil {
		password := req.Request.Header.Get(WebhookTokenHTTPHeader)
		if password == "" {
			password = req.Request.URL.Query().Get(WebhookTokenQueryParam)
		}

		if err := bcrypt.CompareHashAndPassword(webhook.Spec.TokenHash, []byte(password)); err != nil {
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

	var workflow v1.Workflow
	if err := alias.Get(req.Context(), req.Storage, &workflow, req.Namespace(), webhook.Spec.WebhookManifest.Workflow); err != nil {
		return err
	}

	if err = req.Create(&v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			// The name here is the sha256 hash of the body to handle multiple executions of the same webhook.
			// That is, if the webhook is called twice with the same body, it will only be executed once.
			Name:      system.WorkflowExecutionPrefix + fmt.Sprintf("%x", sha256.Sum256(body)),
			Namespace: req.Namespace(),
		},
		Spec: v1.WorkflowExecutionSpec{
			WorkflowName: workflow.Name,
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
	// Ensure that the WorkflowID is set and the workflow exists
	if manifest.Workflow == "" {
		return apierrors.NewBadRequest("webhook manifest must have a workflow name")
	}

	var workflow v1.Workflow
	if system.IsWorkflowID(manifest.Workflow) {
		if err := req.Get(&workflow, manifest.Workflow); err != nil {
			return err
		}
	}

	// On creation, the user must set both the validation header and secret or set neither.
	if (manifest.ValidationHeader != "") != (manifest.Secret != "") {
		return apierrors.NewBadRequest("webhook must have secret and header set together")
	}

	return nil
}
