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
	invoker                     *invoke.Invoker
	gptscript                   *gptscript.GPTScript
	client                      kclient.Client
	gatewayClient               *client.Client
	authLock                    *sync.RWMutex
	authURLs                    map[string]url.URL
	authProviderExtraEnv        []string
	modelLock                   *sync.RWMutex
	modelURLs                   map[string]url.URL
	fileScannerLock             *sync.RWMutex
	fileScannerURLs             map[string]url.URL
	configuredAuthProvidersLock *sync.RWMutex
	configuredAuthProviders     []string
}

func New(ctx context.Context, invoker *invoke.Invoker, c kclient.Client, gClient *gptscript.GPTScript, gatewayClient *client.Client, postgresDSN string) *Dispatcher {
	d := &Dispatcher{
		invoker:                     invoker,
		gptscript:                   gClient,
		client:                      c,
		gatewayClient:               gatewayClient,
		modelLock:                   new(sync.RWMutex),
		modelURLs:                   make(map[string]url.URL),
		authLock:                    new(sync.RWMutex),
		authURLs:                    make(map[string]url.URL),
		fileScannerLock:             new(sync.RWMutex),
		fileScannerURLs:             make(map[string]url.URL),
		configuredAuthProvidersLock: new(sync.RWMutex),
		configuredAuthProviders:     make([]string, 0),
	}

	if postgresDSN != "" {
		d.authProviderExtraEnv = []string{providers.PostgresConnectionEnvVar + "=" + postgresDSN}
	}

	d.UpdateConfiguredAuthProviders(ctx)

	return d
}

func (d *Dispatcher) URLForAuthProvider(ctx context.Context, namespace, authProviderName string) (url.URL, error) {
	u, err := d.urlForProvider(ctx, types.ToolReferenceTypeAuthProvider, namespace, authProviderName, d.authURLs, d.authLock, d.authProviderExtraEnv...)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to get auth provider url: %w", err)
	}
	return u, nil
}

func (d *Dispatcher) URLForModelProvider(ctx context.Context, namespace, modelProviderName string) (url.URL, error) {
	u, err := d.urlForProvider(ctx, types.ToolReferenceTypeModelProvider, namespace, modelProviderName, d.modelURLs, d.modelLock)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to get model provider url: %w", err)
	}
	return u, nil
}

func (d *Dispatcher) urlForFileScannerProvider(ctx context.Context, namespace, fileScannerProviderName string) (url.URL, error) {
	u, err := d.urlForProvider(ctx, types.ToolReferenceTypeFileScannerProvider, namespace, fileScannerProviderName, d.fileScannerURLs, d.fileScannerLock)
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

func (d *Dispatcher) urlForProvider(ctx context.Context, providerType types.ToolReferenceType, namespace, name string, urlMap map[string]url.URL, lock *sync.RWMutex, extraEnv ...string) (url.URL, error) {
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
	u, err := d.startProvider(ctx, providerType, namespace, name, extraEnv...)
	if err != nil {
		return url.URL{}, err
	}

	urlMap[key] = u
	return u, nil
}

func (d *Dispatcher) startProvider(ctx context.Context, providerType types.ToolReferenceType, namespace, providerName string, extraEnv ...string) (url.URL, error) {
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

	task, err := d.invoker.SystemTask(ctx, thread, providerName, "", invoke.SystemTaskOptions{
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

func (d *Dispatcher) TransformRequest(u url.URL, credEnv map[string]string) func(req *http.Request) {
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

func (d *Dispatcher) ListConfiguredAuthProviders(namespace string) []string {
	// For now, the only supported namespace for auth providers is the default namespace.
	if namespace != system.DefaultNamespace {
		return nil
	}

	d.configuredAuthProvidersLock.RLock()
	defer d.configuredAuthProvidersLock.RUnlock()

	return d.configuredAuthProviders
}

func (d *Dispatcher) UpdateConfiguredAuthProviders(ctx context.Context) {
	d.configuredAuthProvidersLock.Lock()
	defer d.configuredAuthProvidersLock.Unlock()

	var authProviders v1.ToolReferenceList
	if err := d.client.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeAuthProvider),
		}),
	}); err != nil {
		fmt.Printf("WARNING: dispatcher failed to list auth providers: %v\n", err)
		return
	}

	var result []string
	for _, authProvider := range authProviders.Items {
		if d.isAuthProviderConfigured(ctx, []string{string(authProvider.UID), system.GenericAuthProviderCredentialContext}, authProvider) {
			result = append(result, authProvider.Name)
		}
	}

	d.configuredAuthProviders = result
}

// isAuthProviderConfigured checks an auth provider to see if all of its required environment variables are set.
// Errors are ignored and reported as the auth provider is not configured.
// Returns: isConfigured (bool)
func (d *Dispatcher) isAuthProviderConfigured(ctx context.Context, credCtx []string, toolRef v1.ToolReference) bool {
	if toolRef.Status.Tool == nil {
		return false
	}

	cred, err := d.gptscript.RevealCredential(ctx, credCtx, toolRef.Name)
	if err != nil {
		return false
	}

	aps, err := providers.ConvertAuthProviderToolRef(toolRef, cred.Env)
	if err != nil {
		return false
	}

	return aps.Configured
}
