package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const PostgresConnectionEnvVar = "OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN"

var log = logger.Package()

type Dispatcher struct {
	sessionManager       *mcp.SessionManager
	client               kclient.Client
	gatewayClient        *client.Client
	licenseProvider      *license.KeygenProvider
	serverURL            string
	internalServerURL    string
	replaceImageRepo     string
	authProviderExtraEnv map[string]string
}

func New(sessionManager *mcp.SessionManager, c kclient.Client, gatewayClient *client.Client, licenseProvider *license.KeygenProvider, serverURL, internalServerURL, postgresDSN string) *Dispatcher {
	d := &Dispatcher{
		sessionManager:    sessionManager,
		client:            c,
		gatewayClient:     gatewayClient,
		licenseProvider:   licenseProvider,
		serverURL:         serverURL,
		internalServerURL: internalServerURL,
		replaceImageRepo:  os.Getenv("OBOT_PROVIDER_IMAGE_REPO_OVERRIDE"),
	}

	if postgresDSN != "" {
		d.authProviderExtraEnv = map[string]string{PostgresConnectionEnvVar: postgresDSN}
	}

	return d
}

func (d *Dispatcher) URLForAuthProvider(ctx context.Context, namespace, authProviderName string) (url.URL, error) {
	var authProvider v1.AuthProvider
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: authProviderName}, &authProvider); err != nil {
		return url.URL{}, fmt.Errorf("failed to get provider: %w", err)
	}

	if len(authProvider.Status.MissingConfigurationParameters) > 0 {
		return url.URL{}, fmt.Errorf("provider %q is not configured, missing configuration parameters: %s", authProviderName, strings.Join(authProvider.Status.MissingConfigurationParameters, ", "))
	}

	credEnv := map[string]string{}
	cred, err := d.gatewayClient.RevealCredential(ctx, []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
	if err != nil {
		if !errors.As(err, &client.CredentialNotFoundError{}) {
			return url.URL{}, fmt.Errorf("failed to reveal provider credential: %w", err)
		}
	} else if cred.Secrets != nil {
		credEnv = cred.Secrets
	}

	image := authProvider.Spec.Image
	if d.replaceImageRepo != "" {
		image = strings.Replace(image, "ghcr.io/obot-platform/providers/", d.replaceImageRepo, 1)
	}

	maps.Copy(credEnv, d.authProviderExtraEnv)

	providerURL, err := d.sessionManager.LaunchServer(ctx, mcp.ServerConfig{
		Runtime:              types.RuntimeContainerized,
		Env:                  d.providerEnv(credEnv),
		ContainerImage:       image,
		ContainerPort:        authProvider.Spec.Port,
		ContainerPath:        authProvider.Spec.Path,
		HealthzPath:          "/",
		MCPServerNamespace:   namespace,
		MCPServerName:        providerServerName("auth-provider", namespace, authProviderName, false),
		MCPServerDisplayName: authProviderName,
		Provider:             true,
		StartupTimeout:       time.Minute,
	})
	if err != nil {
		return url.URL{}, err
	}

	u, err := url.Parse(strings.TrimSpace(providerURL))
	if err != nil {
		return url.URL{}, err
	}

	return *u, nil
}

func (d *Dispatcher) URLForModelProvider(ctx context.Context, namespace, modelProviderName string) (url.URL, error) {
	return d.urlForModelProviderValidation(ctx, namespace, modelProviderName, nil, false)
}

func (d *Dispatcher) URLForModelProviderValidation(ctx context.Context, namespace, modelProviderName string, credEnv map[string]string) (url.URL, error) {
	if credEnv == nil {
		credEnv = make(map[string]string, 1)
	}
	credEnv["OBOT_PROVIDER_VALIDATION_MODE"] = "true"
	return d.urlForModelProviderValidation(ctx, namespace, modelProviderName, credEnv, true)
}

func (d *Dispatcher) urlForModelProviderValidation(ctx context.Context, namespace, modelProviderName string, extraEnv map[string]string, isValidate bool) (url.URL, error) {
	var modelProvider v1.ModelProvider
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: modelProviderName}, &modelProvider); err != nil {
		return url.URL{}, fmt.Errorf("failed to get provider: %w", err)
	}

	if len(modelProvider.Status.MissingConfigurationParameters) > 0 {
		return url.URL{}, fmt.Errorf("provider %q is not configured, missing configuration parameters: %s", modelProviderName, strings.Join(modelProvider.Status.MissingConfigurationParameters, ", "))
	}

	credEnv := map[string]string{}
	cred, err := d.gatewayClient.RevealCredential(ctx, []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProvider.Name)
	if err != nil {
		if !errors.As(err, &client.CredentialNotFoundError{}) {
			return url.URL{}, fmt.Errorf("failed to reveal provider credential: %w", err)
		}
	} else if cred.Secrets != nil {
		credEnv = cred.Secrets
	}

	maps.Copy(credEnv, extraEnv)
	maps.Copy(credEnv, modelProviderLogLevelEnv())

	image := modelProvider.Spec.Image
	if d.replaceImageRepo != "" {
		image = strings.Replace(image, "ghcr.io/obot-platform/providers/", d.replaceImageRepo, 1)
	}

	providerURL, err := d.sessionManager.LaunchServer(ctx, mcp.ServerConfig{
		Runtime:              types.RuntimeContainerized,
		Env:                  d.providerEnv(credEnv),
		ContainerImage:       image,
		ContainerPort:        modelProvider.Spec.Port,
		ContainerPath:        modelProvider.Spec.Path,
		HealthzPath:          "/",
		MCPServerNamespace:   namespace,
		MCPServerName:        providerServerName("model-provider", namespace, modelProviderName, isValidate),
		MCPServerDisplayName: modelProviderName,
		Provider:             true,
		StartupTimeout:       time.Minute,
	})
	if err != nil {
		return url.URL{}, err
	}

	u, err := url.Parse(strings.TrimSpace(providerURL))
	if err != nil {
		return url.URL{}, err
	}

	return *u, nil
}

func modelProviderLogLevelEnv() map[string]string {
	if logger.IsDebug() {
		return map[string]string{"LOG_LEVEL": "DEBUG"}
	}
	return map[string]string{"LOG_LEVEL": "INFO"}
}

func (d *Dispatcher) StopModelProvider(ctx context.Context, namespace, modelProviderName string) {
	d.stopProvider(ctx, "model-provider", namespace, modelProviderName, false)
}

func (d *Dispatcher) StopModelProviderValidation(ctx context.Context, namespace, modelProviderName string) {
	d.stopProvider(ctx, "model-provider", namespace, modelProviderName, true)
}

func (d *Dispatcher) StopAuthProvider(ctx context.Context, namespace, authProviderName string) {
	d.stopProvider(ctx, "auth-provider", namespace, authProviderName, false)
}

func (d *Dispatcher) stopProvider(ctx context.Context, providerType, namespace, providerName string, isValidate bool) {
	if err := d.sessionManager.ShutdownServer(ctx, providerServerName(providerType, namespace, providerName, isValidate)); err != nil {
		log.Warnf("failed to stop provider %s/%s: %v", namespace, providerName, err)
	}
}

func providerServerName(providerType, namespace, providerName string, isValidate bool) string {
	if isValidate {
		providerName += "-validate"
	}
	return name.SafeConcatName("provider", string(providerType), namespace, providerName)
}

func (d *Dispatcher) providerEnv(credEnv map[string]string) []string {
	env := make([]string, 0, len(credEnv)+3)
	for key, val := range credEnv {
		env = append(env, key+"="+val)
	}
	sort.Strings(env)

	publicURL, internalURL := d.serverURL, d.internalServerURL
	if d.sessionManager != nil {
		internalURL = d.sessionManager.TransformObotHostname(internalURL)
	}

	return append(env,
		"OBOT_PROVIDER_LISTEN_HOST=0.0.0.0",
		"OBOT_SERVER_PUBLIC_URL="+publicURL,
		"OBOT_SERVER_URL="+internalURL,
	)
}

func TransformRequest(u url.URL, credEnv map[string]string) func(req *http.Request) {
	return func(req *http.Request) {
		reqPath := req.PathValue("path")
		switch {
		case u.Path == "":
			// Upstream base has no path. Ensure exactly one /v1 prefix in the
			// final URL regardless of whether the client supplied it.
			if strings.HasPrefix(reqPath, "v1/") || reqPath == "v1" {
				u.Path = "/"
			} else {
				u.Path = "/v1"
			}
		case strings.HasSuffix(u.Path, "/v1"):
			// Upstream base already ends in /v1 (the openai/anthropic
			// passthrough routes). Strip a leading v1/ from the client-supplied
			// path so we don't produce /v1/v1/...
			reqPath = strings.TrimPrefix(reqPath, "v1/")
			if reqPath == "v1" {
				reqPath = ""
			}
		}
		u.Path = path.Join(u.Path, reqPath)
		req.URL = &u
		req.Host = u.Host

		addCredHeaders(req, credEnv)
	}
}

func (d *Dispatcher) GetConfiguredAuthProvider(ctx context.Context) (string, error) {
	var authProviders v1.AuthProviderList
	if err := d.client.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return "", fmt.Errorf("failed to list auth providers: %w", err)
	}

	for _, authProvider := range authProviders.Items {
		if d.isAuthProviderConfigured(ctx, authProvider) {
			return authProvider.Name, nil
		}
	}

	return "", nil
}

// isAuthProviderConfigured checks an auth provider to see if all of its required environment variables are set.
// Errors are ignored and reported as the auth provider is not configured.
// We need to check this way instead of using the status fields to avoid race conditions with the controller.
// Returns: isConfigured (bool)
func (d *Dispatcher) isAuthProviderConfigured(ctx context.Context, authProvider v1.AuthProvider) bool {
	credEnv, err := CredentialEnvForAuthProvider(ctx, d.gatewayClient, authProvider)
	if err != nil {
		return false
	}

	for _, envVar := range authProvider.Spec.RequiredConfigurationParameters {
		if _, ok := credEnv[envVar.Name]; !ok {
			return false
		}
	}

	return true
}

func CredentialEnvForAuthProvider(ctx context.Context, gatewayClient *client.Client, authProvider v1.AuthProvider) (map[string]string, error) {
	return credentialEnvForProvider(ctx, gatewayClient, &authProvider, system.GenericAuthProviderCredentialContext)
}

func CredentialEnvForModelProvider(ctx context.Context, gatewayClient *client.Client, modelProvider v1.ModelProvider) (map[string]string, error) {
	return credentialEnvForProvider(ctx, gatewayClient, &modelProvider, system.GenericModelProviderCredentialContext)
}

func credentialEnvForProvider(ctx context.Context, gatewayClient *client.Client, provider kclient.Object, genericCredentialContext string) (map[string]string, error) {
	cred, err := gatewayClient.RevealCredential(ctx, []string{provider.GetName(), genericCredentialContext}, provider.GetName())
	if err != nil {
		return nil, fmt.Errorf("failed to reveal credential: %w", err)
	}

	return cred.Secrets, nil
}
