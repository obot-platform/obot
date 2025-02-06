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
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/jwt"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Dispatcher struct {
	invoker      *invoke.Invoker
	gptscript    *gptscript.GPTScript
	client       kclient.Client
	tokenService *jwt.TokenService

	modelProviderLock *sync.RWMutex
	modelProviderURLs map[string]*url.URL

	authProviderLock            *sync.RWMutex
	authProviderURLs            map[string]*url.URL
	configuredAuthProvidersLock *sync.RWMutex
	configuredAuthProviders     []string

	daemonTriggerProviderLock *sync.RWMutex
	daemonTriggerProviderURLs map[string]*url.URL

	openAICred string
}

func New(ctx context.Context, invoker *invoke.Invoker, c kclient.Client, gClient *gptscript.GPTScript, tokenService *jwt.TokenService) *Dispatcher {
	d := &Dispatcher{
		invoker:                     invoker,
		gptscript:                   gClient,
		client:                      c,
		tokenService:                tokenService,
		modelProviderLock:           new(sync.RWMutex),
		modelProviderURLs:           make(map[string]*url.URL),
		authProviderLock:            new(sync.RWMutex),
		authProviderURLs:            make(map[string]*url.URL),
		configuredAuthProvidersLock: new(sync.RWMutex),
		configuredAuthProviders:     make([]string, 0),
		daemonTriggerProviderLock:   new(sync.RWMutex),
		daemonTriggerProviderURLs:   make(map[string]*url.URL),
	}

	d.UpdateConfiguredAuthProviders(ctx)

	return d
}

func (d *Dispatcher) URLForAuthProvider(ctx context.Context, namespace, authProviderName string) (*url.URL, error) {
	key := namespace + "/" + authProviderName
	// Check the map with the read lock.
	d.authProviderLock.RLock()
	u, ok := d.authProviderURLs[key]
	d.authProviderLock.RUnlock()
	if ok && engine.IsDaemonRunning(u.String()) {
		return u, nil
	}

	d.authProviderLock.Lock()
	defer d.authProviderLock.Unlock()

	// If we didn't find anything with the read lock, check with the write lock.
	// It could be that another thread beat us to the write lock and added the auth provider we desire.
	u, ok = d.authProviderURLs[key]
	if ok && engine.IsDaemonRunning(u.String()) {
		return u, nil
	}

	// We didn't find the auth provider (or the daemon stopped for some reason), so start it and add it to the map.
	u, err := d.startAuthProvider(ctx, namespace, authProviderName)
	if err != nil {
		return nil, err
	}

	d.authProviderURLs[key] = u
	return u, nil
}

func (d *Dispatcher) URLForModelProvider(ctx context.Context, namespace, modelProviderName string) (*url.URL, string, error) {
	key := namespace + "/" + modelProviderName
	// Check the map with the read lock.
	d.modelProviderLock.RLock()
	u, ok := d.modelProviderURLs[key]
	d.modelProviderLock.RUnlock()
	if ok && (u.Hostname() != "127.0.0.1" || engine.IsDaemonRunning(u.String())) {
		if u.Host == "api.openai.com" {
			return u, d.openAICred, nil
		}
		return u, "", nil
	}

	d.modelProviderLock.Lock()
	defer d.modelProviderLock.Unlock()

	// If we didn't find anything with the read lock, check with the write lock.
	// It could be that another thread beat us to the write lock and added the model provider we desire.
	u, ok = d.modelProviderURLs[key]
	if ok && (u.Hostname() != "127.0.0.1" || engine.IsDaemonRunning(u.String())) {
		if u.Host == "api.openai.com" {
			return u, d.openAICred, nil
		}
		return u, "", nil
	}

	// We didn't find the model provider (or the daemon stopped for some reason), so start it and add it to the map.
	u, err := d.startModelProvider(ctx, namespace, modelProviderName)
	if err != nil {
		return nil, "", err
	}

	d.modelProviderURLs[key] = u
	if u.Host == "api.openai.com" {
		return u, d.openAICred, nil
	}

	return u, "", nil
}

func (d *Dispatcher) URLForDaemonTriggerProvider(ctx context.Context, namespace, daemonTriggerProviderName string) (*url.URL, error) {
	key := namespace + "/" + daemonTriggerProviderName
	// Check the map with the read lock.
	d.daemonTriggerProviderLock.RLock()
	u, ok := d.daemonTriggerProviderURLs[key]
	d.daemonTriggerProviderLock.RUnlock()
	if ok && engine.IsDaemonRunning(u.String()) {
		return u, nil
	}

	d.daemonTriggerProviderLock.Lock()
	defer d.daemonTriggerProviderLock.Unlock()

	// If we didn't find anything with the read lock, check with the write lock.
	// It could be that another thread beat us to the write lock and added the daemon trigger provider we desire.
	u, ok = d.daemonTriggerProviderURLs[key]
	if ok && engine.IsDaemonRunning(u.String()) {
		return u, nil
	}

	// We didn't find the daemon trigger provider (or the daemon stopped for some reason), so start it and add it to the map.
	u, err := d.startDaemonTriggerProvider(ctx, namespace, daemonTriggerProviderName)
	if err != nil {
		return nil, err
	}

	d.daemonTriggerProviderURLs[key] = u
	return u, nil
}

func (d *Dispatcher) StopProvider(providerType types.ToolReferenceType, namespace, providerName string) error {
	var (
		providerURLs map[string]*url.URL
		key          = namespace + "/" + providerName
	)
	switch providerType {
	case types.ToolReferenceTypeModelProvider:
		d.modelProviderLock.Lock()
		defer d.modelProviderLock.Unlock()
		providerURLs = d.modelProviderURLs
	case types.ToolReferenceTypeAuthProvider:
		d.authProviderLock.Lock()
		defer d.authProviderLock.Unlock()
		providerURLs = d.authProviderURLs
	case types.ToolReferenceTypeDaemonTriggerProvider:
		d.daemonTriggerProviderLock.Lock()
		defer d.daemonTriggerProviderLock.Unlock()
		providerURLs = d.daemonTriggerProviderURLs
	default:
		return types.NewErrBadRequest("unknown provider type: %s", providerType)
	}

	u := providerURLs[key]
	if u != nil && (u.Hostname() != "127.0.0.1" || engine.IsDaemonRunning(u.String())) {
		engine.StopDaemon(u.String())
	}
	delete(providerURLs, key)

	return nil
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

	model, err := d.getProviderForModel(req.Context(), namespace, modelStr)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	u, token, err := d.URLForModelProvider(req.Context(), namespace, model.Spec.Manifest.ModelProvider)
	if err != nil {
		return fmt.Errorf("failed to get model provider: %w", err)
	}

	return d.transformRequest(req, *u, body, model.Spec.Manifest.TargetModel, token)
}

func (d *Dispatcher) getProviderForModel(ctx context.Context, namespace, model string) (*v1.Model, error) {
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

func (d *Dispatcher) startModelProvider(ctx context.Context, namespace, modelProviderName string) (*url.URL, error) {
	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.ThreadPrefix + modelProviderName,
			Namespace: namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
		},
	}

	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Name}, thread); apierrors.IsNotFound(err) {
		if err = d.client.Create(ctx, thread); err != nil {
			return nil, fmt.Errorf("failed to create thread: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	var modelProvider v1.ToolReference
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: modelProviderName}, &modelProvider); err != nil || modelProvider.Spec.Type != types.ToolReferenceTypeModelProvider {
		return nil, fmt.Errorf("failed to get model provider: %w", err)
	}

	credCtx := []string{string(modelProvider.UID), system.GenericModelProviderCredentialContext}
	if modelProvider.Status.Tool == nil {
		return nil, fmt.Errorf("model provider %q has not been resolved", modelProviderName)
	}

	// Ensure that the model provider has been configured so that we don't get stuck waiting on a prompt.
	mps, err := providers.ConvertModelProviderToolRef(modelProvider, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert model provider: %w", err)
	}
	if len(mps.RequiredConfigurationParameters) > 0 {
		cred, err := d.gptscript.RevealCredential(ctx, credCtx, modelProviderName)
		if err != nil {
			return nil, fmt.Errorf("model provider is not configured: %w", err)
		}

		mps, err = providers.ConvertModelProviderToolRef(modelProvider, cred.Env)
		if err != nil {
			return nil, fmt.Errorf("failed to convert model provider: %w", err)
		}

		if len(mps.MissingConfigurationParameters) > 0 {
			return nil, fmt.Errorf("model provider is not configured: missing configuration parameters %s", strings.Join(mps.MissingConfigurationParameters, ", "))
		}

		if modelProvider.Name == "openai-model-provider" {
			d.openAICred = cred.Env["OBOT_OPENAI_MODEL_PROVIDER_API_KEY"]
		}
	}

	task, err := d.invoker.SystemTask(ctx, thread, modelProviderName, "", invoke.SystemTaskOptions{
		CredentialContextIDs: credCtx,
	})
	if err != nil {
		return nil, err
	}

	result, err := task.Result(ctx)
	if err != nil {
		return nil, err
	}

	return url.Parse(strings.TrimSpace(result.Output))
}

func (d *Dispatcher) startDaemonTriggerProvider(ctx context.Context, namespace, daemonTriggerProviderName string) (*url.URL, error) {
	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.ThreadPrefix + daemonTriggerProviderName,
			Namespace: namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
		},
	}

	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Name}, thread); apierrors.IsNotFound(err) {
		if err = d.client.Create(ctx, thread); err != nil {
			return nil, fmt.Errorf("failed to create thread: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	var daemonTriggerProvider v1.ToolReference
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: daemonTriggerProviderName}, &daemonTriggerProvider); err != nil || daemonTriggerProvider.Spec.Type != types.ToolReferenceTypeDaemonTriggerProvider {
		return nil, fmt.Errorf("failed to get daemon trigger provider: %w", err)
	}

	credCtx := []string{string(daemonTriggerProvider.UID), system.GenericDaemonTriggerProviderCredentialContext}
	if daemonTriggerProvider.Status.Tool == nil {
		return nil, fmt.Errorf("daemon trigger provider %q has not been resolved", daemonTriggerProviderName)
	}

	// Ensure that the model provider has been configured so that we don't get stuck waiting on a prompt.
	dtps, err := providers.ConvertDaemonTriggerProviderToolRef(daemonTriggerProvider, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert daemon trigger provider: %w", err)
	}
	if len(dtps.RequiredConfigurationParameters) > 0 {
		cred, err := d.gptscript.RevealCredential(ctx, credCtx, daemonTriggerProviderName)
		if err != nil {
			return nil, fmt.Errorf("daemon trigger provider is not configured: %w", err)
		}

		dtps, err = providers.ConvertDaemonTriggerProviderToolRef(daemonTriggerProvider, cred.Env)
		if err != nil {
			return nil, fmt.Errorf("failed to convert daemon trigger provider: %w", err)
		}

		if len(dtps.MissingConfigurationParameters) > 0 {
			return nil, fmt.Errorf("daemon trigger provider is not configured: missing configuration parameters %s", strings.Join(dtps.MissingConfigurationParameters, ", "))
		}
	}

	// Craft a token with the required Obot scopes (if the daemon requires Obot API access)
	var env []string
	if len(dtps.ObotScopes) > 0 {
		obotToken, err := d.tokenService.NewToken(jwt.TokenContext{
			Scope:       namespace,
			ExtraScopes: dtps.ObotScopes,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to Obot token for daemon trigger provider: %w", err)
		}
		env = append(env, fmt.Sprintf("OBOT_API_TOKEN=%s", obotToken))
	}

	task, err := d.invoker.SystemTask(ctx, thread, daemonTriggerProviderName, "", invoke.SystemTaskOptions{
		CredentialContextIDs: credCtx,
		Env:                  env,
	})
	if err != nil {
		return nil, err
	}

	result, err := task.Result(ctx)
	if err != nil {
		return nil, err
	}

	return url.Parse(strings.TrimSpace(result.Output))
}

func (d *Dispatcher) transformRequest(req *http.Request, u url.URL, body map[string]any, targetModel, token string) error {
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

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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

func (d *Dispatcher) startAuthProvider(ctx context.Context, namespace, authProviderName string) (*url.URL, error) {
	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.ThreadPrefix + authProviderName,
			Namespace: namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
		},
	}

	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Name}, thread); apierrors.IsNotFound(err) {
		if err = d.client.Create(ctx, thread); err != nil {
			return nil, fmt.Errorf("failed to create thread: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	var authProvider v1.ToolReference
	if err := d.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: authProviderName}, &authProvider); err != nil || authProvider.Spec.Type != types.ToolReferenceTypeAuthProvider {
		return nil, fmt.Errorf("failed to get auth provider: %w", err)
	}

	credCtx := []string{string(authProvider.UID), system.GenericAuthProviderCredentialContext}
	if authProvider.Status.Tool == nil {
		return nil, fmt.Errorf("auth provider %q has not been resolved", authProviderName)
	}

	// Ensure that the auth provider has been configured so that we don't get stuck waiting on a prompt.
	aps, err := providers.ConvertAuthProviderToolRef(authProvider, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert auth provider: %w", err)
	}
	if len(aps.RequiredConfigurationParameters) > 0 {
		isConfigured, missingEnvVars, err := d.isAuthProviderConfigured(ctx, credCtx, authProvider)
		if err != nil {
			return nil, fmt.Errorf("failed to check auth provider configuration: %w", err)
		} else if !isConfigured {
			if len(missingEnvVars) > 0 {
				return nil, fmt.Errorf("auth provider is not configured: missing configuration parameters %s", strings.Join(missingEnvVars, ", "))
			}
			return nil, fmt.Errorf("auth provider is not configured: %w", err)
		}
	}

	task, err := d.invoker.SystemTask(ctx, thread, authProviderName, "", invoke.SystemTaskOptions{
		CredentialContextIDs: credCtx,
	})
	if err != nil {
		return nil, err
	}

	result, err := task.Result(ctx)
	if err != nil {
		return nil, err
	}

	return url.Parse(strings.TrimSpace(result.Output))
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
		if isConfigured, _, _ := d.isAuthProviderConfigured(ctx, []string{string(authProvider.UID), system.GenericAuthProviderCredentialContext}, authProvider); isConfigured {
			result = append(result, authProvider.Name)
		}
	}

	d.configuredAuthProviders = result
}

// isAuthProviderConfigured checks an auth provider to see if all of its required environment variables are set.
// Returns: isConfigured (bool), missingEnvVars ([]string), error
func (d *Dispatcher) isAuthProviderConfigured(ctx context.Context, credCtx []string, toolRef v1.ToolReference) (bool, []string, error) {
	if toolRef.Status.Tool == nil {
		return false, nil, nil
	}

	cred, err := d.gptscript.RevealCredential(ctx, credCtx, toolRef.Name)
	if err != nil {
		return false, nil, err
	}

	aps, err := providers.ConvertAuthProviderToolRef(toolRef, cred.Env)
	if err != nil {
		return false, nil, err
	}

	return aps.Configured, aps.MissingConfigurationParameters, nil
}
