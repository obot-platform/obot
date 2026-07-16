package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	postgresConnectionEnvVar      = "OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN"
	postgresMaxIdleConnsEnvVar    = "OBOT_AUTH_PROVIDER_POSTGRES_MAX_IDLE_CONNECTIONS"
	postgresMaxOpenConnsEnvVar    = "OBOT_AUTH_PROVIDER_POSTGRES_MAX_CONNECTIONS"
	postgresConnMaxLifetimeEnvVar = "OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_LIFETIME_SECONDS"
)

type Dispatcher struct {
	sessionManager       *mcp.SessionManager
	client               kclient.Client
	gatewayClient        *client.Client
	licenseProvider      *license.Provider
	serverURL            string
	internalServerURL    string
	authProviderExtraEnv map[string]string
	ports                *ports

	builtinLock         sync.RWMutex
	builtinAuthProvider map[string]url.URL
}

func New(sessionManager *mcp.SessionManager, c kclient.Client, gatewayClient *client.Client, licenseProvider *license.Provider, serverURL, internalServerURL, postgresDSN string) *Dispatcher {
	d := &Dispatcher{
		sessionManager:      sessionManager,
		client:              c,
		gatewayClient:       gatewayClient,
		licenseProvider:     licenseProvider,
		serverURL:           serverURL,
		internalServerURL:   internalServerURL,
		ports:               newPorts(),
		builtinAuthProvider: map[string]url.URL{},
	}

	if postgresDSN != "" {
		d.authProviderExtraEnv = map[string]string{
			postgresConnectionEnvVar:      postgresDSN,
			postgresMaxIdleConnsEnvVar:    os.Getenv(postgresMaxIdleConnsEnvVar),
			postgresMaxOpenConnsEnvVar:    os.Getenv(postgresMaxOpenConnsEnvVar),
			postgresConnMaxLifetimeEnvVar: os.Getenv(postgresConnMaxLifetimeEnvVar),
		}
	}

	return d
}

func (d *Dispatcher) Close() {
	d.closeDaemons()
}

// RegisterBuiltinAuthProvider registers an auth provider that runs inside the Obot process,
// rather than as a daemon launched from the provider registry.
func (d *Dispatcher) RegisterBuiltinAuthProvider(namespace, authProviderName string, u url.URL) {
	d.builtinLock.Lock()
	defer d.builtinLock.Unlock()

	d.builtinAuthProvider[providerKeyForAuthProvider(namespace, authProviderName)] = u
}

func (d *Dispatcher) builtinAuthProviderURL(key string) (url.URL, bool) {
	d.builtinLock.RLock()
	defer d.builtinLock.RUnlock()

	u, ok := d.builtinAuthProvider[key]
	return u, ok
}

func (d *Dispatcher) URLForAuthProvider(ctx context.Context, namespace, authProviderName string) (url.URL, error) {
	key := providerKeyForAuthProvider(namespace, authProviderName)

	if u, ok := d.builtinAuthProviderURL(key); ok {
		return u, nil
	}

	d.ports.daemonLock.RLock()
	if port := d.ports.daemonPorts[key]; port != 0 {
		d.ports.daemonLock.RUnlock()
		return url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}, nil
	}
	d.ports.daemonLock.RUnlock()

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

	maps.Copy(credEnv, d.authProviderExtraEnv)

	return d.startDaemon(credEnv, key, authProvider.Spec.Command, authProvider.Spec.Args...)
}

func (d *Dispatcher) URLForModelProvider(ctx context.Context, namespace, modelProviderName string) (url.URL, error) {
	key := providerKeyForModelProvider(namespace, modelProviderName)

	d.ports.daemonLock.RLock()
	if port := d.ports.daemonPorts[key]; port != 0 {
		d.ports.daemonLock.RUnlock()
		return url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}, nil
	}
	d.ports.daemonLock.RUnlock()

	var modelProvider v1.ModelProvider
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: modelProviderName}, &modelProvider); err != nil {
		return url.URL{}, fmt.Errorf("failed to get provider: %w", err)
	}

	return d.urlForModelProvider(ctx, key, modelProvider)
}

func (d *Dispatcher) ValidateModelProvider(ctx context.Context, namespace, modelProviderName string, env map[string]string) error {
	var modelProvider v1.ModelProvider
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: modelProviderName}, &modelProvider); err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	return d.runCommand(ctx, env, modelProvider.Spec.Command, modelProvider.Spec.ValidateArgs...)
}

func (d *Dispatcher) urlForModelProvider(ctx context.Context, key string, modelProvider v1.ModelProvider) (url.URL, error) {
	if len(modelProvider.Status.MissingConfigurationParameters) > 0 {
		return url.URL{}, fmt.Errorf("provider %q is not configured, missing configuration parameters: %s", modelProvider.Name, strings.Join(modelProvider.Status.MissingConfigurationParameters, ", "))
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

	credEnv["LOG_LEVEL"] = modelProviderLogLevel()

	return d.startDaemon(credEnv, key, modelProvider.Spec.Command, modelProvider.Spec.Args...)
}

func modelProviderLogLevel() string {
	if logger.IsDebug() {
		return "DEBUG"
	}
	return "INFO"
}

func (d *Dispatcher) StopModelProvider(namespace, modelProviderName string) {
	d.stopProvider("model-provider", namespace, modelProviderName)
}

func (d *Dispatcher) StopAuthProvider(namespace, authProviderName string) {
	d.stopProvider("auth-provider", namespace, authProviderName)
}

func (d *Dispatcher) stopProvider(providerType, namespace, providerName string) {
	d.stopDaemon(providerKey(providerType, namespace, providerName))
}

func (d *Dispatcher) GetConfiguredAuthProvider(ctx context.Context) (string, error) {
	var authProviders v1.AuthProviderList
	// First check for an auth provider whose status field is configured.
	if err := d.client.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace:     system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{"status.configured": "true"}),
	}); err != nil {
		return "", fmt.Errorf("failed to list auth providers: %w", err)
	}

	for _, authProvider := range authProviders.Items {
		if d.isAuthProviderConfigured(ctx, authProvider) {
			return authProvider.Name, nil
		}
	}

	// If no auth provider is configured, then check all of them in case the controller hasn't updated yet.
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

func providerKeyForAuthProvider(namespace, providerName string) string {
	return providerKey("auth-provider", namespace, providerName)
}

func providerKeyForModelProvider(namespace, providerName string) string {
	return providerKey("model-provider", namespace, providerName)
}

func providerKey(providerType, namespace, providerName string) string {
	return fmt.Sprintf("%s/%s/%s", providerType, namespace, providerName)
}
