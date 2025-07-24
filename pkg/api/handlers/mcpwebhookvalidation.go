package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MCPWebhookValidationHandler struct{}

func NewMCPWebhookValidationHandler() *MCPWebhookValidationHandler {
	return &MCPWebhookValidationHandler{}
}

func (m *MCPWebhookValidationHandler) List(req api.Context) error {
	var list v1.MCPWebhookValidationList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list mcp webhook validations: %w", err)
	}

	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{CredentialContexts: []string{system.MCPWebhookValidationCredentialContext}})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credMap := make(map[string]struct{}, len(creds))
	for _, cred := range creds {
		credMap[cred.ToolName] = struct{}{}
	}

	items := make([]types.MCPWebhookValidation, 0, len(list.Items))
	for _, item := range list.Items {
		_, hasSecret := credMap[item.Name]
		items = append(items, convertMCPWebhookValidation(item, hasSecret))
	}

	return req.Write(types.MCPWebhookValidationList{Items: items})
}

func (m *MCPWebhookValidationHandler) Get(req api.Context) error {
	var validation v1.MCPWebhookValidation
	if err := req.Get(&validation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	secretCred, err := req.GPTClient.RevealCredential(req.Context(), []string{system.MCPWebhookValidationCredentialContext}, validation.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(validation, secretCred.Env != nil))
}

func (m *MCPWebhookValidationHandler) Create(req api.Context) error {
	var manifest types.MCPWebhookValidationManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid manifest: %v", err)
	}

	var secretCred map[string]string
	if manifest.Secret != "" {
		secretCred = map[string]string{
			"secret": manifest.Secret,
		}

		// Don't save the secrets in the database.
		manifest.Secret = ""
	}

	validation := v1.MCPWebhookValidation{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MCPWebhookValidationPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.MCPWebhookValidationSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&validation); err != nil {
		return fmt.Errorf("failed to create mcp webhook validation: %w", err)
	}

	if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  system.MCPWebhookValidationCredentialContext,
		ToolName: validation.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      secretCred,
	}); err != nil {
		_ = req.Delete(&validation)
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(validation, secretCred != nil))
}

func (m *MCPWebhookValidationHandler) Update(req api.Context) error {
	var validation v1.MCPWebhookValidation
	if err := req.Get(&validation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	var manifest types.MCPWebhookValidationManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid manifest: %v", err)
	}

	var secretCred map[string]string
	if manifest.Secret != "" {
		secretCred = map[string]string{
			"secret": manifest.Secret,
		}
		// Don't save the secrets in the database.
		manifest.Secret = ""
	}

	validation.Spec.Manifest = manifest

	if err := req.Update(&validation); err != nil {
		return fmt.Errorf("failed to update mcp webhook validation: %w", err)
	}

	if secretCred != nil {
		// The only way to update a credential is to delete it and recreate it.
		if err := req.GPTClient.DeleteCredential(req.Context(), system.MCPWebhookValidationCredentialContext, validation.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to delete credential: %w", err)
		}

		if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
			Context:  system.MCPWebhookValidationCredentialContext,
			ToolName: validation.Name,
			Type:     gptscript.CredentialTypeTool,
			Env:      secretCred,
		}); err != nil {
			return fmt.Errorf("failed to create credential: %w", err)
		}
	} else {
		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{system.MCPWebhookValidationCredentialContext}, validation.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to reveal credential: %w", err)
		}

		secretCred = cred.Env
	}

	return req.Write(convertMCPWebhookValidation(validation, secretCred != nil))
}

func (m *MCPWebhookValidationHandler) Delete(req api.Context) error {
	var validation v1.MCPWebhookValidation
	if err := req.Get(&validation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	if err := req.GPTClient.DeleteCredential(req.Context(), system.MCPWebhookValidationCredentialContext, validation.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	if err := req.Delete(&validation); err != nil {
		return fmt.Errorf("failed to delete mcp webhook validation: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(validation, false))
}

func (m *MCPWebhookValidationHandler) RemoveSecret(req api.Context) error {
	var validation v1.MCPWebhookValidation
	if err := req.Get(&validation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	if err := req.GPTClient.DeleteCredential(req.Context(), system.MCPWebhookValidationCredentialContext, validation.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func convertMCPWebhookValidation(validation v1.MCPWebhookValidation, hasSecret bool) types.MCPWebhookValidation {
	return types.MCPWebhookValidation{
		Metadata:                     MetadataFrom(&validation),
		MCPWebhookValidationManifest: validation.Spec.Manifest,
		HasSecret:                    hasSecret,
	}
}
