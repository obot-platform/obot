package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const cookieSecretEnvVar = "OBOT_AUTH_PROVIDER_COOKIE_SECRET"

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
		if envVars := ref.Status.Tool.Metadata["envVars"]; envVars != "" {
			cred, err := ap.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for auth provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	return req.Write(convertToolReferenceToAuthProvider(ref, credEnvVars))
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
		resp = append(resp, convertToolReferenceToAuthProvider(ref, env))
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

	// Check to see if there are any other configured auth providers.
	// For now, we only support one auth provider at a time to be configured.
	allAuthProviders, err := ap.listAuthProviders(req)
	if err != nil {
		return err
	}

	for _, ap := range allAuthProviders {
		if ap.Configured && (ap.Name != authProviderNameFromToolRef(ref) || ap.Namespace != ref.Namespace) {
			return types.NewErrBadRequest("another auth provider is already configured")
		}
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	cookieSecret, err := generateCookieSecret()
	if err != nil {
		return err
	}
	envVars[cookieSecretEnvVar] = cookieSecret

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

func convertToolReferenceToAuthProvider(ref v1.ToolReference, credEnvVars map[string]string) types.AuthProvider {
	ap := types.AuthProvider{
		Metadata: MetadataFrom(&ref),
		AuthProviderManifest: types.AuthProviderManifest{
			Name:          authProviderNameFromToolRef(ref),
			Namespace:     ref.Namespace,
			ToolReference: ref.Spec.Reference,
		},
		AuthProviderStatus: *convertAuthProviderToolRef(ref, credEnvVars),
	}

	ap.Type = "authprovider"

	return ap
}

func convertAuthProviderToolRef(toolRef v1.ToolReference, cred map[string]string) *types.AuthProviderStatus {
	var (
		requiredEnvVars, missingEnvVars, optionalEnvVars []string
		icon                                             string
	)
	if toolRef.Status.Tool != nil {
		if toolRef.Status.Tool.Metadata["envVars"] != "" {
			requiredEnvVars = strings.Split(toolRef.Status.Tool.Metadata["envVars"], ",")

			// Remove the cookie secret environment variable if it's there.
			idx := slices.Index(requiredEnvVars, cookieSecretEnvVar)
			if idx != -1 {
				requiredEnvVars = append(requiredEnvVars[:idx], requiredEnvVars[idx+1:]...)
			}
		}

		for _, envVar := range requiredEnvVars {
			if _, ok := cred[envVar]; !ok {
				missingEnvVars = append(missingEnvVars, envVar)
			}
		}

		icon = toolRef.Status.Tool.Metadata["icon"]

		if optionalEnvVarMetadata := toolRef.Status.Tool.Metadata["optionalEnvVars"]; optionalEnvVarMetadata != "" {
			optionalEnvVars = strings.Split(optionalEnvVarMetadata, ",")
		}
	}

	return &types.AuthProviderStatus{
		Icon:                            icon,
		Configured:                      toolRef.Status.Tool != nil && len(missingEnvVars) == 0,
		RequiredConfigurationParameters: requiredEnvVars,
		MissingConfigurationParameters:  missingEnvVars,
		OptionalConfigurationParameters: optionalEnvVars,
	}
}

func generateCookieSecret() (string, error) {
	const length = 32

	var bytes = make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	return base64.StdEncoding.EncodeToString(bytes), nil
}
