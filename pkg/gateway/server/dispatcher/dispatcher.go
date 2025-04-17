package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/engine"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
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
		d.authProviderExtraEnv = []string{"POSTGRES_DSN=" + postgresDSN}
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

func (d *Dispatcher) urlForModelProvider(ctx context.Context, namespace, modelProviderName string) (url.URL, error) {
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
		return url.URL{}, fmt.Errorf("failed to get model provider: %w", err)
	}

	credCtx := []string{string(providerToolRef.UID), providerTypeToGenericCredContext[providerType]}
	if providerToolRef.Status.Tool == nil {
		return url.URL{}, fmt.Errorf("provider tool reference %q has not been resolved", providerName)
	}

	// Ensure that the provider has been configured so that we don't get stuck waiting on a prompt.
	mps, err := providers.ConvertProviderToolRef(providerToolRef, nil)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to convert model provider: %w", err)
	}
	if len(mps.RequiredConfigurationParameters) > 0 {
		cred, err := d.gptscript.RevealCredential(ctx, credCtx, providerName)
		if err != nil {
			return url.URL{}, fmt.Errorf("model provider is not configured: %w", err)
		}

		mps, err = providers.ConvertProviderToolRef(providerToolRef, cred.Env)
		if err != nil {
			return url.URL{}, fmt.Errorf("failed to convert model provider: %w", err)
		}

		if len(mps.MissingConfigurationParameters) > 0 {
			return url.URL{}, fmt.Errorf("model provider is not configured: missing configuration parameters %s", strings.Join(mps.MissingConfigurationParameters, ", "))
		}
	}

	task, err := d.invoker.SystemTask(ctx, thread, providerName, "", invoke.SystemTaskOptions{
		CredentialContextIDs: credCtx,
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

func (d *Dispatcher) TransformRequest(req *http.Request, namespace string) error {
	body, err := readBody(req)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}

	modelStr, ok := body["model"].(string)
	if !ok {
		return fmt.Errorf("missing model in body")
	}

	model, err := d.getModelProviderForModel(req.Context(), namespace, modelStr)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	u, err := d.urlForModelProvider(req.Context(), namespace, model.Spec.Manifest.ModelProvider)
	if err != nil {
		return fmt.Errorf("failed to get model provider: %w", err)
	}

	return d.transformRequest(req, u, body, model.Spec.Manifest.TargetModel)
}

func (d *Dispatcher) getModelProviderForModel(ctx context.Context, namespace, model string) (*v1.Model, error) {
	m, err := alias.GetFromScope(ctx, d.client, "Model", namespace, model)
	if err != nil {
		return nil, err
	}

	var respModel *v1.Model
	switch m := m.(type) {
	case *v1.DefaultModelAlias:
		if m.Spec.Manifest.Model == "" {
			return nil, fmt.Errorf("default model alias %q is not configured", model)
		}
		var model v1.Model
		if err := alias.Get(ctx, d.client, &model, namespace, m.Spec.Manifest.Model); err != nil {
			return nil, err
		}
		respModel = &model
	case *v1.Model:
		respModel = m
	}

	if respModel != nil {
		if !respModel.Spec.Manifest.Active {
			return nil, fmt.Errorf("model %q is not active", respModel.Spec.Manifest.Name)
		}

		return respModel, nil
	}

	return nil, fmt.Errorf("model %q not found", model)
}

func (d *Dispatcher) transformRequest(req *http.Request, u url.URL, body map[string]any, targetModel string) error {
	if u.Path == "" {
		u.Path = "/v1"
	}
	u.Path = path.Join(u.Path, req.PathValue("path"))
	req.URL = &u
	req.Host = u.Host

	body["model"] = targetModel
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req.Body = io.NopCloser(bytes.NewReader(b))
	req.ContentLength = int64(len(b))

	return nil
}

func readBody(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		return nil, err
	}

	return m, nil
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
