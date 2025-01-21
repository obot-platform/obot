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
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Dispatcher struct {
	invoker    *invoke.Invoker
	gptscript  *gptscript.GPTScript
	client     kclient.Client
	modelLock  *sync.RWMutex
	modelUrls  map[string]*url.URL
	authLock   *sync.RWMutex
	authUrls   map[string]*url.URL
	openAICred string
}

func New(invoker *invoke.Invoker, c kclient.Client, gClient *gptscript.GPTScript) *Dispatcher {
	return &Dispatcher{
		invoker:   invoker,
		gptscript: gClient,
		client:    c,
		modelLock: new(sync.RWMutex),
		modelUrls: make(map[string]*url.URL),
		authLock:  new(sync.RWMutex),
		authUrls:  make(map[string]*url.URL),
	}
}

func (d *Dispatcher) URLForAuthProvider(ctx context.Context, namespace, authProviderName string) (*url.URL, error) {
	key := namespace + "/" + authProviderName
	// Check the map with the read lock.
	d.authLock.RLock()
	u, ok := d.authUrls[key]
	d.authLock.RUnlock()
	if ok && engine.IsDaemonRunning(u.String()) {
		return u, nil
	}

	d.authLock.Lock()
	defer d.authLock.Unlock()

	// If we didn't find anything with the read lock, check with the write lock.
	// It could be that another thread beat us to the write lock and added the auth provider we desire.
	u, ok = d.authUrls[key]
	if ok && engine.IsDaemonRunning(u.String()) {
		return u, nil
	}

	// We didn't find the auth provider (or the daemon stopped for some reason), so start it and add it to the map.
	u, err := d.startAuthProvider(ctx, namespace, authProviderName)
	if err != nil {
		return nil, err
	}

	d.authUrls[key] = u
	return u, nil
}

func (d *Dispatcher) URLForModelProvider(ctx context.Context, namespace, modelProviderName string) (*url.URL, string, error) {
	key := namespace + "/" + modelProviderName
	// Check the map with the read lock.
	d.modelLock.RLock()
	u, ok := d.modelUrls[key]
	d.modelLock.RUnlock()
	if ok && (u.Hostname() != "127.0.0.1" || engine.IsDaemonRunning(u.String())) {
		if u.Host == "api.openai.com" {
			return u, d.openAICred, nil
		}
		return u, "", nil
	}

	d.modelLock.Lock()
	defer d.modelLock.Unlock()

	// If we didn't find anything with the read lock, check with the write lock.
	// It could be that another thread beat us to the write lock and added the model provider we desire.
	u, ok = d.modelUrls[key]
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

	d.modelUrls[key] = u
	if u.Host == "api.openai.com" {
		return u, d.openAICred, nil
	}

	return u, "", nil
}

func (d *Dispatcher) StopModelProvider(namespace, modelProviderName string) {
	key := namespace + "/" + modelProviderName
	d.modelLock.Lock()
	defer d.modelLock.Unlock()

	u := d.modelUrls[key]
	if u != nil && u.Hostname() == "127.0.0.1" && engine.IsDaemonRunning(u.String()) {
		engine.StopDaemon(u.String())
	}

	delete(d.modelUrls, key)
}

func (d *Dispatcher) StopAuthProvider(namespace, authProviderName string) {
	key := namespace + "/" + authProviderName
	d.authLock.Lock()
	defer d.authLock.Unlock()

	u := d.authUrls[key]
	if u != nil && u.Hostname() == "127.0.0.1" && engine.IsDaemonRunning(u.String()) {
		engine.StopDaemon(u.String())
	}

	delete(d.authUrls, key)
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

	u, token, err := d.URLForModelProvider(req.Context(), namespace, model.Spec.Manifest.ModelProvider)
	if err != nil {
		return fmt.Errorf("failed to get model provider: %w", err)
	}

	return d.transformRequest(req, *u, body, model.Spec.Manifest.TargetModel, token)
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
	if modelProvider.Status.Tool.Metadata["envVars"] != "" {
		cred, err := d.gptscript.RevealCredential(ctx, credCtx, modelProviderName)
		if err != nil {
			return nil, fmt.Errorf("model provider is not configured: %w", err)
		}

		var missingEnvVars []string
		for _, envVar := range strings.Split(modelProvider.Status.Tool.Metadata["envVars"], ",") {
			if cred.Env[envVar] == "" {
				missingEnvVars = append(missingEnvVars, envVar)
			}
		}

		if len(missingEnvVars) > 0 {
			return nil, fmt.Errorf("model provider is not configured: missing configuration parameters %s", strings.Join(missingEnvVars, ", "))
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

	if authProvider.Status.Tool.Metadata["envVars"] != "" {
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

func (d *Dispatcher) ListConfiguredAuthProviders(ctx context.Context, namespace string) ([]string, error) {
	var authProviders v1.ToolReferenceList
	if err := d.client.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeAuthProvider),
		}),
	}); err != nil {
		return nil, err
	}

	var result []string
	for _, authProvider := range authProviders.Items {
		if isConfigured, _, _ := d.isAuthProviderConfigured(ctx, []string{string(authProvider.UID), system.GenericAuthProviderCredentialContext}, authProvider); isConfigured {
			result = append(result, authProvider.Name)
		}
	}

	return result, nil
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

	var requiredEnvVars []string
	if toolRef.Status.Tool.Metadata["envVars"] != "" {
		requiredEnvVars = strings.Split(toolRef.Status.Tool.Metadata["envVars"], ",")
	}

	var missingEnvVars []string
	for _, envVar := range requiredEnvVars {
		if cred.Env[envVar] == "" {
			missingEnvVars = append(missingEnvVars, envVar)
		}
	}

	if len(missingEnvVars) > 0 {
		return false, missingEnvVars, nil
	}

	return true, nil, nil
}
