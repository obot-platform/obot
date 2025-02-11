package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type DaemonTriggerProviderHandler struct {
	gptscript  *gptscript.GPTScript
	dispatcher *dispatcher.Dispatcher
	invoker    *invoke.Invoker
}

func NewDaemonTriggerProviderHandler(gClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher, invoker *invoke.Invoker) *DaemonTriggerProviderHandler {
	return &DaemonTriggerProviderHandler{
		gptscript:  gClient,
		dispatcher: dispatcher,
		invoker:    invoker,
	}
}

func (h *DaemonTriggerProviderHandler) ByID(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeDaemonTriggerProvider {
		return types.NewErrNotFound(
			"trigger provider %q not found",
			ref.Name,
		)
	}

	var credEnvVars map[string]string
	if ref.Status.Tool != nil {
		aps, err := providers.ConvertDaemonTriggerProviderToolRef(ref, nil)
		if err != nil {
			return err
		}
		if len(aps.RequiredConfigurationParameters) > 0 {
			cred, err := h.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericDaemonTriggerProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for trigger provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	daemonTriggerProvider, err := convertToolReferenceToDaemonTriggerProvider(ref, credEnvVars)
	if err != nil {
		return err
	}

	return req.Write(daemonTriggerProvider)
}

func (h *DaemonTriggerProviderHandler) List(req api.Context) error {
	var refList v1.ToolReferenceList
	if err := req.List(&refList, &kclient.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeDaemonTriggerProvider),
		}),
	}); err != nil {
		return err
	}

	credCtxs := make([]string, 0, len(refList.Items)+1)
	for _, ref := range refList.Items {
		credCtxs = append(credCtxs, string(ref.UID))
	}
	credCtxs = append(credCtxs, system.GenericDaemonTriggerProviderCredentialContext)

	creds, err := h.gptscript.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list trigger provider credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		credMap[cred.Context+cred.ToolName] = cred.Env
	}

	resp := make([]types.DaemonTriggerProvider, 0, len(refList.Items))
	for _, ref := range refList.Items {
		env, ok := credMap[string(ref.UID)+ref.Name]
		if !ok {
			env = credMap[system.GenericDaemonTriggerProviderCredentialContext+ref.Name]
		}
		daemonTriggerProvider, err := convertToolReferenceToDaemonTriggerProvider(ref, env)
		if err != nil {
			log.Warnf("failed to convert trigger provider %q: %v", ref.Name, err)
			continue
		}
		resp = append(resp, daemonTriggerProvider)
	}

	return req.Write(types.DaemonTriggerProviderList{Items: resp})
}

func (h *DaemonTriggerProviderHandler) Configure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeDaemonTriggerProvider {
		return types.NewErrBadRequest("%q is not an trigger provider", ref.Name)
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := h.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericDaemonTriggerProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = h.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := h.gptscript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  string(ref.UID),
		ToolName: ref.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential for trigger provider %q: %w", ref.Name, err)
	}

	if err := h.dispatcher.StopProvider(types.ToolReferenceTypeDaemonTriggerProvider, ref.Namespace, ref.Name); err != nil {
		return fmt.Errorf("failed to stop trigger provider: %w", err)
	}

	if ref.Annotations[v1.DaemonTriggerProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.DaemonTriggerProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.DaemonTriggerProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

type DaemonTriggerOptions struct {
	// TODO(njhale): Use a real JSON schema type for this.
	Schema openapi3.Schema `json:"schema,omitempty"`
	Err    string          `json:"error,omitempty"`
}

func (e *DaemonTriggerOptions) Error() string {
	return fmt.Sprintf("failed to get daemon trigger options: {\"error\": \"%s\"}", e.Err)
}

func (h *DaemonTriggerProviderHandler) Options(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeDaemonTriggerProvider {
		return types.NewErrBadRequest("%q is not a daemon trigger provider", ref.Name)
	}

	log.Debugf("Getting options for daemon trigger provider %q", ref.Name)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	envs := make([]string, 0, len(envVars))
	for key, val := range envVars {
		envs = append(envs, key+"="+val)
	}

	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix + "-" + ref.Name + "-options-",
			Namespace:    ref.Namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
		},
	}

	if err := req.Create(thread); err != nil {
		return fmt.Errorf("failed to create thread: %w", err)
	}

	defer func() { _ = req.Delete(thread) }()

	task, err := h.invoker.SystemTask(req.Context(), thread, "options from "+ref.Spec.Reference, "", invoke.SystemTaskOptions{Env: envs})
	if err != nil {
		return err
	}
	defer task.Close()

	res, err := task.Result(req.Context())
	if err != nil {
		if strings.Contains(err.Error(), "tool not found: options from "+ref.Spec.Reference) { // there's no simple way to do errors.As/.Is at this point unfortunately
			log.Errorf("Daemon trigger provider %q does not provide an options tool. Looking for 'validate from %s'", ref.Name, ref.Spec.Reference)
			return types.NewErrNotFound(
				fmt.Sprintf("`options from %s` tool not found", ref.Spec.Reference),
				ref.Name,
			)
		}
		return types.NewErrHttp(http.StatusUnprocessableEntity, strings.Trim(err.Error(), "\"'"))
	}

	var daemonTriggerOptions DaemonTriggerOptions
	if json.Unmarshal([]byte(res.Output), &daemonTriggerOptions) == nil && daemonTriggerOptions.Err != "" {
		return types.NewErrHttp(http.StatusUnprocessableEntity, daemonTriggerOptions.Error())
	}

	return req.Write(daemonTriggerOptions)
}

func (h *DaemonTriggerProviderHandler) Deconfigure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeDaemonTriggerProvider {
		return types.NewErrBadRequest("%q is not a trigger provider", ref.Name)
	}

	cred, err := h.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericDaemonTriggerProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = h.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	// Stop the trigger provider so that the credential is completely removed from the system.
	if err := h.dispatcher.StopProvider(types.ToolReferenceTypeDaemonTriggerProvider, ref.Namespace, ref.Name); err != nil {
		return fmt.Errorf("failed to stop model provider: %w", err)
	}

	if ref.Annotations[v1.DaemonTriggerProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.DaemonTriggerProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.DaemonTriggerProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

func (h *DaemonTriggerProviderHandler) Reveal(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeDaemonTriggerProvider {
		return types.NewErrBadRequest("%q is not a daemon trigger provider", ref.Name)
	}

	cred, err := h.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericDaemonTriggerProviderCredentialContext}, ref.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal credential for daemon trigger provider %q: %w", ref.Name, err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", ref.Name)
}

func convertToolReferenceToDaemonTriggerProvider(ref v1.ToolReference, credEnvVars map[string]string) (types.DaemonTriggerProvider, error) {
	name := ref.Name
	if ref.Status.Tool != nil {
		name = ref.Status.Tool.Name
	}

	tps, err := providers.ConvertDaemonTriggerProviderToolRef(ref, credEnvVars)
	if err != nil {
		return types.DaemonTriggerProvider{}, err
	}
	tp := types.DaemonTriggerProvider{
		Metadata: MetadataFrom(&ref),
		DaemonTriggerProviderManifest: types.DaemonTriggerProviderManifest{
			Name:          name,
			Namespace:     ref.Namespace,
			ToolReference: ref.Spec.Reference,
		},
		DaemonTriggerProviderStatus: *tps,
	}

	tp.Type = "daemontriggerprovider"

	return tp, nil
}
