package dispatcher

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/engine"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Dispatcher struct {
	invoker              *invoke.Invoker
	client               kclient.Client
	gptscriptClient      *gptscript.GPTScript
	gatewayClient        *client.Client
	authLock             *sync.RWMutex
	authURLs             map[string]url.URL
	authProviderExtraEnv []string
	modelLock            *sync.RWMutex
	modelURLs            map[string]url.URL
	fileScannerLock      *sync.RWMutex
	fileScannerURLs      map[string]url.URL
}

func New(invoker *invoke.Invoker, c kclient.Client, gptscriptClient *gptscript.GPTScript, gatewayClient *client.Client, postgresDSN string) *Dispatcher {
	d := &Dispatcher{
		invoker:         invoker,
		client:          c,
		gatewayClient:   gatewayClient,
		gptscriptClient: gptscriptClient,
		modelLock:       new(sync.RWMutex),
		modelURLs:       make(map[string]url.URL),
		authLock:        new(sync.RWMutex),
		authURLs:        make(map[string]url.URL),
		fileScannerLock: new(sync.RWMutex),
		fileScannerURLs: make(map[string]url.URL),
	}

	if postgresDSN != "" {
		d.authProviderExtraEnv = []string{providers.PostgresConnectionEnvVar + "=" + postgresDSN}
	}

	return d
}

func (d *Dispatcher) URLForAuthProvider(ctx context.Context, gptClient *gptscript.GPTScript, namespace, authProviderName string) (url.URL, error) {
	u, err := d.urlForProvider(ctx, gptClient, types.ToolReferenceTypeAuthProvider, namespace, authProviderName, d.authURLs, d.authLock, d.authProviderExtraEnv...)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to get auth provider url: %w", err)
	}
	return u, nil
}

func (d *Dispatcher) URLForModelProvider(ctx context.Context, gptClient *gptscript.GPTScript, namespace, modelProviderName string) (url.URL, error) {
	u, err := d.urlForProvider(ctx, gptClient, types.ToolReferenceTypeModelProvider, namespace, modelProviderName, d.modelURLs, d.modelLock)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to get model provider url: %w", err)
	}
	return u, nil
}

func (d *Dispatcher) urlForFileScannerProvider(ctx context.Context, gptClient *gptscript.GPTScript, namespace, fileScannerProviderName string) (url.URL, error) {
	u, err := d.urlForProvider(ctx, gptClient, types.ToolReferenceTypeFileScannerProvider, namespace, fileScannerProviderName, d.fileScannerURLs, d.fileScannerLock)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to get file scanner provider url: %w", err)
	}
	return u, nil
}

var providerTypeToGenericCredContext = map[types.ToolReferenceType]string{
	types.ToolReferenceTypeModelProvider:       system.GenericModelProviderCredentialContext,
	types.ToolReferenceTypeAuthProvider:        system.GenericAuthProviderCredentialContext,
	types.ToolReferenceTypeFileScannerProvider: system.GenericFileScannerProviderCredentialContext,
}

func (d *Dispatcher) urlForProvider(ctx context.Context, gptClient *gptscript.GPTScript, providerType types.ToolReferenceType, namespace, name string, urlMap map[string]url.URL, lock *sync.RWMutex, extraEnv ...string) (url.URL, error) {
	key := namespace + "/" + name
	// Check the map with the read lock.
	lock.RLock()
	u, ok := urlMap[key]
	lock.RUnlock()
	if ok && (u.Hostname() != "127.0.0.1" || engine.IsDaemonRunning(u.String())) {
		return u, nil
	}

	lock.Lock()
	defer lock.Unlock()

	// If we didn't find anything with the read lock, check with the write lock.
	// It could be that another thread beat us to the write lock and added the provider we desire.
	u, ok = urlMap[key]
	if ok && (u.Hostname() != "127.0.0.1" || engine.IsDaemonRunning(u.String())) {
		return u, nil
	}

	// We didn't find the provider (or the daemon stopped for some reason), so start it and add it to the map.
	u, err := d.startProvider(ctx, gptClient, providerType, namespace, name, extraEnv...)
	if err != nil {
		return url.URL{}, err
	}

	urlMap[key] = u
	return u, nil
}

func (d *Dispatcher) startProvider(ctx context.Context, gptClient *gptscript.GPTScript, providerType types.ToolReferenceType, namespace, providerName string, extraEnv ...string) (url.URL, error) {
	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.ThreadPrefix + providerName,
			Namespace: namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
		},
	}

	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Name}, thread); apierrors.IsNotFound(err) {
		if err = d.client.Create(ctx, thread); err != nil {
			return url.URL{}, fmt.Errorf("failed to create thread: %w", err)
		}
	} else if err != nil {
		return url.URL{}, fmt.Errorf("failed to get thread: %w", err)
	}

	var providerToolRef v1.ToolReference
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: providerName}, &providerToolRef); err != nil || providerToolRef.Spec.Type != providerType {
		return url.URL{}, fmt.Errorf("failed to get provider: %w", err)
	}

	task, err := d.invoker.SystemTask(ctx, gptClient, thread, providerName, "", invoke.SystemTaskOptions{
		CredentialContextIDs: []string{string(providerToolRef.UID), providerTypeToGenericCredContext[providerType]},
		Env:                  extraEnv,
	})
	if err != nil {
		return url.URL{}, err
	}

	result, err := task.Result(ctx)
	if err != nil {
		return url.URL{}, err
	}

	u, err := url.Parse(strings.TrimSpace(result.Output))
	if err != nil {
		return url.URL{}, err
	}

	return *u, nil
}

func (d *Dispatcher) StopModelProvider(namespace, modelProviderName string) {
	stopProvider(namespace, modelProviderName, d.modelURLs, d.modelLock)
}

func (d *Dispatcher) StopAuthProvider(namespace, authProviderName string) {
	stopProvider(namespace, authProviderName, d.authURLs, d.authLock)
}

func (d *Dispatcher) StopFileScannerProvider(namespace, fileScannerProviderName string) {
	stopProvider(namespace, fileScannerProviderName, d.fileScannerURLs, d.fileScannerLock)
}

func stopProvider(namespace, name string, urlMap map[string]url.URL, lock *sync.RWMutex) {
	key := namespace + "/" + name
	lock.Lock()
	defer lock.Unlock()

	u, ok := urlMap[key]
	if ok && u.Hostname() == "127.0.0.1" && engine.IsDaemonRunning(u.String()) {
		engine.StopDaemon(u.String())
	}

	delete(urlMap, key)
}

func TransformRequest(u url.URL, credEnv map[string]string) func(req *http.Request) {
	return func(req *http.Request) {
		if u.Path == "" {
			u.Path = "/v1"
		}
		u.Path = path.Join(u.Path, req.PathValue("path"))
		req.URL = &u
		req.Host = u.Host

		addCredHeaders(req, credEnv)
	}
}

func (d *Dispatcher) GetConfiguredAuthProvider(ctx context.Context) (string, error) {
	var authProviders v1.ToolReferenceList
	if err := d.client.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeAuthProvider),
		}),
	}); err != nil {
		return "", fmt.Errorf("failed to list auth providers: %w", err)
	}

	for _, authProvider := range authProviders.Items {
		if d.isAuthProviderConfigured(ctx, d.gptscriptClient, authProvider) {
			return authProvider.Name, nil
		}
	}

	return "", nil
}

// isAuthProviderConfigured checks an auth provider to see if all of its required environment variables are set.
// Errors are ignored and reported as the auth provider is not configured.
// Returns: isConfigured (bool)
func (d *Dispatcher) isAuthProviderConfigured(ctx context.Context, gptscriptClient *gptscript.GPTScript, toolRef v1.ToolReference) bool {
	if toolRef.Status.Tool == nil {
		return false
	}

	credEnv, err := CredentialEnvForAuthProvider(ctx, gptscriptClient, toolRef)
	if err != nil {
		return false
	}

	aps, err := providers.ConvertAuthProviderToolRef(toolRef, credEnv)
	if err != nil {
		return false
	}

	return aps.Configured
}

// GetAuthProviderConfigEnv returns the value of the specified environment variable from the given auth providers config.
func (d *Dispatcher) GetAuthProviderConfigEnv(ctx context.Context, gptscriptClient *gptscript.GPTScript, namespace, authProviderName, envName string) (string, error) {
	var authProvider v1.ToolReference
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: authProviderName}, &authProvider); err != nil {
		return "", fmt.Errorf("failed to get auth provider: %w", err)
	}

	cred, err := credentialEnvForProvider(ctx, gptscriptClient, authProvider, system.GenericAuthProviderCredentialContext)
	if err != nil {
		return "", fmt.Errorf("failed to reveal credential: %w", err)
	}

	return cred[envName], nil
}

func CredentialEnvForAuthProvider(ctx context.Context, gptClient *gptscript.GPTScript, authProvider v1.ToolReference) (map[string]string, error) {
	return credentialEnvForProvider(ctx, gptClient, authProvider, system.GenericAuthProviderCredentialContext)
}

func CredentialEnvForModelProvider(ctx context.Context, gptClient *gptscript.GPTScript, modelProvider v1.ToolReference) (map[string]string, error) {
	return credentialEnvForProvider(ctx, gptClient, modelProvider, system.GenericModelProviderCredentialContext)
}

func credentialEnvForProvider(ctx context.Context, gptClient *gptscript.GPTScript, provider v1.ToolReference, genericCredentialContext string) (map[string]string, error) {
	cred, err := gptClient.RevealCredential(ctx, []string{string(provider.UID), genericCredentialContext}, provider.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to reveal credential: %w", err)
	}

	return cred.Env, nil
}
