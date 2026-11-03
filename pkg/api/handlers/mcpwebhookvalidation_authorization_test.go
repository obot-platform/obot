package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func TestFilterAuthorizationSecretIsolation(t *testing.T) {
	manifest := types.MCPWebhookValidationManifest{
		Secret:        "new-content",
		Authorization: &types.MCPFilterAuthorization{Enabled: true, URL: "https://policy.example", Secret: "new-authorization"},
	}
	saved := map[string]string{"secret": "old", types.MCPFilterAuthorizationSecretKey: "old-authorization", "unrelated": "preserved"}
	secrets := extractFilterSecrets(&manifest, saved)
	if manifest.Secret != "" || manifest.Authorization.Secret != "" {
		t.Fatal("secrets persisted in manifest")
	}
	if secrets["secret"] != "new-content" || secrets[types.MCPFilterAuthorizationSecretKey] != "new-authorization" || secrets["unrelated"] != "preserved" || saved["secret"] != "old" {
		t.Fatal("credential replacement corrupted another credential")
	}
	manifest.Secret = "replacement"
	secrets = extractFilterSecrets(&manifest, secrets)
	if secrets[types.MCPFilterAuthorizationSecretKey] != "new-authorization" {
		t.Fatal("content secret erased authorization secret")
	}
	// Defense in depth for read responses, including unexpected stored secrets.
	manifest.Authorization.Secret = "must-not-return"
	result := convertMCPWebhookValidation(v1.MCPWebhookValidation{Spec: v1.MCPWebhookValidationSpec{Manifest: manifest}}, secrets)
	if !result.HasAuthorizationSecret || result.Authorization.Secret != "" {
		t.Fatal("secret exposed in response")
	}
	if manifest.Authorization.Secret != "must-not-return" {
		t.Fatal("conversion mutated stored object")
	}
}

func TestFilterAuthorizationURLValidation(t *testing.T) {
	for _, endpoint := range []string{"http://policy.example", "https://user:password@policy.example", "https://policy.example/#fragment", "/relative"} {
		t.Run(endpoint, func(t *testing.T) {
			manifest := types.MCPWebhookValidationManifest{URL: "https://content.example", Authorization: &types.MCPFilterAuthorization{Enabled: true, URL: endpoint}}
			if err := validateManifest(t.Context(), &manifest, mcp.ValidationOptions{}); err == nil {
				t.Fatal("unsafe endpoint accepted")
			}
		})
	}
}

func TestAuthorizationCredentialLifecycle(t *testing.T) {
	storage := newFakeStorage(t)
	gatewayClient := newHandlerTestGateway(t)
	handler := NewMCPWebhookValidationHandler(&mcp.SessionManager{})
	manifest := types.MCPWebhookValidationManifest{
		Name:   "CRM",
		URL:    "https://content.example/filter",
		Secret: "content-secret",
		Authorization: &types.MCPFilterAuthorization{
			Enabled: true,
			URL:     "https://8.8.8.8/access/v1/evaluation",
			Secret:  "authorization-secret",
		},
	}
	req, recorder := catalogEntryRequest(t, storage, gatewayClient, http.MethodPost, "/api/mcp-webhook-validations", manifest, nil)
	if err := handler.Create(req); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(recorder.Body.String(), "authorization-secret") {
		t.Fatal("create leaked secret")
	}
	var created types.MCPWebhookValidation
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	pathValues := map[string]string{"mcp_webhook_validation_id": created.ID}
	// An old client edits content settings without knowing the authorization field.
	manifest.Authorization = nil
	manifest.Secret = "replacement-content"
	req, recorder = catalogEntryRequest(t, storage, gatewayClient, http.MethodPut, "/api/mcp-webhook-validations/"+created.ID, manifest, pathValues)
	if err := handler.Update(req); err != nil {
		t.Fatal(err)
	}
	var updated types.MCPWebhookValidation
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Authorization == nil || !updated.Authorization.Enabled || !updated.HasAuthorizationSecret {
		t.Fatal("old client erased authorization")
	}
	for _, operation := range []string{"reveal", "configure", "deconfigure"} {
		var body any
		if operation == "configure" {
			body = map[string]string{"secret": "configured-content", types.MCPFilterAuthorizationSecretKey: "spoofed", types.MCPFilterAuthorizationURLKey: "spoofed"}
		}
		req, recorder = catalogEntryRequest(t, storage, gatewayClient, http.MethodPost, "/api/mcp-webhook-validations/"+created.ID+"/"+operation, body, pathValues)
		var err error
		switch operation {
		case "reveal":
			err = handler.Reveal(req)
		case "configure":
			err = handler.Configure(req)
		case "deconfigure":
			err = handler.Deconfigure(req)
		}
		if err != nil {
			t.Fatalf("%s: %v", operation, err)
		}
		if strings.Contains(recorder.Body.String(), "authorization-secret") {
			t.Fatalf("%s leaked secret", operation)
		}
		cred, err := gatewayClient.RevealCredential(t.Context(), []string{system.MCPWebhookValidationCredentialContext}, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if cred.Secrets[types.MCPFilterAuthorizationSecretKey] != "authorization-secret" || cred.Secrets[types.MCPFilterAuthorizationURLKey] != "https://8.8.8.8/access/v1/evaluation" {
			t.Fatalf("%s corrupted authorization credential", operation)
		}
		if operation == "deconfigure" && cred.Secrets["secret"] != "" {
			t.Fatal("content secret not removed")
		}
	}
	manifest.Authorization = &types.MCPFilterAuthorization{Enabled: false}
	manifest.Secret = ""
	req, recorder = catalogEntryRequest(t, storage, gatewayClient, http.MethodPut, "/api/mcp-webhook-validations/"+created.ID, manifest, pathValues)
	if err := handler.Update(req); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Authorization.Enabled {
		t.Fatal("explicit disabling ignored")
	}
}

func TestAuthorizationOptionalSecretAndSigningState(t *testing.T) {
	storage := newFakeStorage(t)
	gatewayClient := newHandlerTestGateway(t)
	handler := NewMCPWebhookValidationHandler(&mcp.SessionManager{})
	manifest := types.MCPWebhookValidationManifest{
		Name: "Unsigned authorization",
		URL:  "https://content.example/filter",
		Authorization: &types.MCPFilterAuthorization{
			Enabled:        true,
			URL:            "https://8.8.8.8/evaluate",
			SigningEnabled: true, // Client cannot set server-managed state.
		},
	}
	req, recorder := catalogEntryRequest(t, storage, gatewayClient, http.MethodPost, "/api/mcp-webhook-validations", manifest, nil)
	if err := handler.Create(req); err != nil {
		t.Fatal(err)
	}
	var created types.MCPWebhookValidation
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Authorization.SigningEnabled || created.HasAuthorizationSecret {
		t.Fatal("unsigned create unexpectedly requires signing")
	}
	pathValues := map[string]string{"mcp_webhook_validation_id": created.ID}
	manifest.Authorization.Secret = "signing-secret"
	req, recorder = catalogEntryRequest(t, storage, gatewayClient, http.MethodPut, "/api/mcp-webhook-validations/"+created.ID, manifest, pathValues)
	if err := handler.Update(req); err != nil {
		t.Fatal(err)
	}
	var signed types.MCPWebhookValidation
	if err := json.Unmarshal(recorder.Body.Bytes(), &signed); err != nil {
		t.Fatal(err)
	}
	if !signed.Authorization.SigningEnabled || !signed.HasAuthorizationSecret {
		t.Fatal("signing state not recorded")
	}
	// Simulate a lost credential, followed by an older client's manifest update.
	if _, err := gatewayClient.DeleteCredential(t.Context(), system.MCPWebhookValidationCredentialContext, created.ID); err != nil {
		t.Fatal(err)
	}
	manifest.Authorization.Secret = ""
	manifest.Authorization.SigningEnabled = false
	req, recorder = catalogEntryRequest(t, storage, gatewayClient, http.MethodPut, "/api/mcp-webhook-validations/"+created.ID, manifest, pathValues)
	if err := handler.Update(req); err != nil {
		t.Fatal(err)
	}
	var missing types.MCPWebhookValidation
	if err := json.Unmarshal(recorder.Body.Bytes(), &missing); err != nil {
		t.Fatal(err)
	}
	if !missing.Authorization.SigningEnabled || missing.HasAuthorizationSecret {
		t.Fatal("missing credential silently downgraded signing")
	}
}
