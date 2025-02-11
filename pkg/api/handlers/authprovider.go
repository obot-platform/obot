package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AuthProviderHandler struct {
	gptscript  *gptscript.GPTScript
	dispatcher *dispatcher.Dispatcher
}

func NewAuthProviderHandler(gClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher) *AuthProviderHandler {
	return &AuthProviderHandler{
		gptscript:  gClient,
		dispatcher: dispatcher,
	}
}

func (ap *AuthProviderHandler) ByID(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeAuthProvider {
		return types.NewErrNotFound(
			"auth provider %q not found",
			ref.Name,
		)
	}

	var credEnvVars map[string]string
	if ref.Status.Tool != nil {
		aps, err := providers.ConvertModelProviderToolRef(ref, nil)
		if err != nil {
			return err
		}
		if len(aps.RequiredConfigurationParameters) > 0 {
			cred, err := ap.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for auth provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	authProvider, err := convertToolReferenceToAuthProvider(ref, credEnvVars)
	if err != nil {
		return err
	}

	return req.Write(authProvider)
}

func (ap *AuthProviderHandler) List(req api.Context) error {
	resp, err := ap.listAuthProviders(req)
	if err != nil {
		return err
	}

	return req.Write(types.AuthProviderList{Items: resp})
}

func (ap *AuthProviderHandler) listAuthProviders(req api.Context) ([]types.AuthProvider, error) {
	var refList v1.ToolReferenceList
	if err := req.List(&refList, &kclient.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeAuthProvider),
		}),
	}); err != nil {
		return nil, err
	}

	credCtxs := make([]string, 0, len(refList.Items)+1)
	for _, ref := range refList.Items {
		credCtxs = append(credCtxs, string(ref.UID))
	}
	credCtxs = append(credCtxs, system.GenericAuthProviderCredentialContext)

	creds, err := ap.gptscript.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list auth provider credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		credMap[cred.Context+cred.ToolName] = cred.Env
	}

	resp := make([]types.AuthProvider, 0, len(refList.Items))
	for _, ref := range refList.Items {
		env, ok := credMap[string(ref.UID)+ref.Name]
		if !ok {
			env = credMap[system.GenericAuthProviderCredentialContext+ref.Name]
		}
		authProvider, err := convertToolReferenceToAuthProvider(ref, env)
		if err != nil {
			log.Warnf("failed to convert auth provider %q: %v", ref.Name, err)
			continue
		}
		resp = append(resp, authProvider)
	}
	return resp, nil
}

func (ap *AuthProviderHandler) Configure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeAuthProvider {
		return types.NewErrBadRequest("%q is not an auth provider", ref.Name)
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	cookieSecret, err := generateCookieSecret()
	if err != nil {
		return err
	}
	envVars[providers.CookieSecretEnvVar] = cookieSecret

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := ap.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = ap.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	defer func() {
		go ap.dispatcher.UpdateConfiguredAuthProviders(context.Background())
	}()

	if err := ap.gptscript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  string(ref.UID),
		ToolName: ref.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential for auth provider %q: %w", ref.Name, err)
	}

	ap.dispatcher.StopAuthProvider(ref.Namespace, ref.Name)

	if ref.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.AuthProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.AuthProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

func (ap *AuthProviderHandler) Deconfigure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeAuthProvider {
		return types.NewErrBadRequest("%q is not an auth provider", ref.Name)
	}

	cred, err := ap.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = ap.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	defer func() {
		go ap.dispatcher.UpdateConfiguredAuthProviders(context.Background())
	}()

	// Stop the auth provider so that the credential is completely removed from the system.
	ap.dispatcher.StopAuthProvider(ref.Namespace, ref.Name)

	if ref.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.AuthProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.AuthProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

func (ap *AuthProviderHandler) Reveal(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeAuthProvider {
		return types.NewErrBadRequest("%q is not an auth provider", ref.Name)
	}

	cred, err := ap.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal credential for auth provider %q: %w", ref.Name, err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", ref.Name)
}

func authProviderNameFromToolRef(ref v1.ToolReference) string {
	name := ref.Name
	if ref.Status.Tool != nil {
		name = ref.Status.Tool.Name
	}
	return name
}

func convertToolReferenceToAuthProvider(ref v1.ToolReference, credEnvVars map[string]string) (types.AuthProvider, error) {
	aps, err := providers.ConvertAuthProviderToolRef(ref, credEnvVars)
	if err != nil {
		return types.AuthProvider{}, err
	}
	ap := types.AuthProvider{
		Metadata: MetadataFrom(&ref),
		AuthProviderManifest: types.AuthProviderManifest{
			Name:          authProviderNameFromToolRef(ref),
			Namespace:     ref.Namespace,
			ToolReference: ref.Spec.Reference,
		},
		AuthProviderStatus: *aps,
	}

	ap.Type = "authprovider"

	return ap, nil
}

func generateCookieSecret() (string, error) {
	const length = 32

	// Generate a random token. Repeat until we have one that is 32 bytes long after trimming.
	// This only takes one try in the vast majority of circumstances, but could occasionally take a second.
	var b = make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	for len(bytes.TrimSpace(b)) != length {
		_, err := rand.Read(b)
		if err != nil {
			return "", fmt.Errorf("failed to generate random token: %w", err)
		}
	}

	return base64.StdEncoding.EncodeToString(b), nil
}
