package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/api/handlers/providers"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type FileScannerProviderHandler struct {
	gptscript  *gptscript.GPTScript
	dispatcher *dispatcher.Dispatcher
	invoker    *invoke.Invoker
}

func NewFileScannerProviderHandler(gClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher, invoker *invoke.Invoker) *FileScannerProviderHandler {
	return &FileScannerProviderHandler{
		gptscript:  gClient,
		dispatcher: dispatcher,
		invoker:    invoker,
	}
}

func (f *FileScannerProviderHandler) ByID(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeFileScannerProvider {
		return types.NewErrNotFound(
			"file scanner provider %q not found",
			ref.Name,
		)
	}

	mps, err := providers.ConvertFileScannerProviderToolRef(ref, nil)
	if err != nil {
		return err
	}

	var credEnvVars map[string]string
	if ref.Status.Tool != nil {
		if len(mps.RequiredConfigurationParameters) > 0 {
			cred, err := f.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericFileScannerProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for file scanner provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	fileScannerProvider, err := convertToolReferenceToFileScannerProvider(ref, credEnvVars)
	if err != nil {
		return err
	}

	return req.Write(fileScannerProvider)
}

func (f *FileScannerProviderHandler) List(req api.Context) error {
	var refList v1.ToolReferenceList
	if err := req.List(&refList, &kclient.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeFileScannerProvider),
		}),
	}); err != nil {
		return err
	}

	credCtxs := make([]string, 0, len(refList.Items)+1)
	for _, ref := range refList.Items {
		credCtxs = append(credCtxs, string(ref.UID))
	}
	credCtxs = append(credCtxs, system.GenericFileScannerProviderCredentialContext)

	creds, err := f.gptscript.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list file scanner provider credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		credMap[cred.Context+cred.ToolName] = cred.Env
	}

	resp := make([]types.FileScannerProvider, 0, len(refList.Items))
	for _, ref := range refList.Items {
		env, ok := credMap[string(ref.UID)+ref.Name]
		if !ok {
			env = credMap[system.GenericFileScannerProviderCredentialContext+ref.Name]
		}
		fileScannerProvider, err := convertToolReferenceToFileScannerProvider(ref, env)
		if err != nil {
			log.Errorf("failed to convert model provider %q: %v", ref.Name, err)
			continue
		}
		resp = append(resp, fileScannerProvider)
	}

	return req.Write(types.FileScannerProviderList{Items: resp})
}

type fileScannerProviderValidationError struct {
	Err string `json:"error"`
}

func (ve *fileScannerProviderValidationError) Error() string {
	return fmt.Sprintf("file scanner provider credentials validation failed: {\"error\": \"%s\"}", ve.Err)
}

func (f *FileScannerProviderHandler) Validate(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeFileScannerProvider {
		return types.NewErrBadRequest("%q is not a file scanner provider", ref.Name)
	}

	log.Debugf("Validating file scanner provider %q", ref.Name)

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
			GenerateName: system.ThreadPrefix + "-" + ref.Name + "-validate-",
			Namespace:    ref.Namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
			Ephemeral:  true,
		},
	}

	if err := req.Create(thread); err != nil {
		return fmt.Errorf("failed to create thread: %w", err)
	}

	task, err := f.invoker.SystemTask(req.Context(), thread, "validate from "+ref.Spec.Reference, "", invoke.SystemTaskOptions{Env: envs})
	if err != nil {
		return err
	}
	defer task.Close()

	res, err := task.Result(req.Context())
	if err != nil {
		if strings.Contains(err.Error(), "tool not found: validate from "+ref.Spec.Reference) { // there's no simple way to do errors.As/.Is at this point unfortunately
			log.Errorf("Model provider %q does not provide a validate tool. Looking for 'validate from %s'", ref.Name, ref.Spec.Reference)
			return types.NewErrNotFound(
				fmt.Sprintf("`validate from %s` tool not found", ref.Spec.Reference),
				ref.Name,
			)
		}
		return types.NewErrHTTP(http.StatusUnprocessableEntity, strings.Trim(err.Error(), "\"'"))
	}

	var validationError fileScannerProviderValidationError
	if json.Unmarshal([]byte(res.Output), &validationError) == nil && validationError.Err != "" {
		return types.NewErrHTTP(http.StatusUnprocessableEntity, validationError.Error())
	}

	return nil
}

func (f *FileScannerProviderHandler) Configure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeFileScannerProvider {
		return types.NewErrBadRequest("%q is not a file scanner provider", ref.Name)
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := f.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericFileScannerProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = f.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err = f.gptscript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  string(ref.UID),
		ToolName: ref.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	f.dispatcher.StopFileScannerProvider(ref.Namespace, ref.Name)

	if ref.Annotations[v1.FileScannerProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.FileScannerProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.FileScannerProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

func (f *FileScannerProviderHandler) Deconfigure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeFileScannerProvider {
		return types.NewErrBadRequest("%q is not a file scanner provider", ref.Name)
	}

	cred, err := f.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericFileScannerProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = f.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	// Stop the file scanner provider so that the credential is completely removed from the system.
	f.dispatcher.StopFileScannerProvider(ref.Namespace, ref.Name)

	if ref.Annotations[v1.FileScannerProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.FileScannerProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.FileScannerProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

func (f *FileScannerProviderHandler) Reveal(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeFileScannerProvider {
		return types.NewErrBadRequest("%q is not a file scanner provider", ref.Name)
	}

	cred, err := f.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericFileScannerProviderCredentialContext}, ref.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", ref.Name)
}

func convertToolReferenceToFileScannerProvider(ref v1.ToolReference, credEnvVars map[string]string) (types.FileScannerProvider, error) {
	name := ref.Name
	if ref.Status.Tool != nil {
		name = ref.Status.Tool.Name
	}

	mps, err := providers.ConvertFileScannerProviderToolRef(ref, credEnvVars)
	if err != nil {
		return types.FileScannerProvider{}, err
	}
	mp := types.FileScannerProvider{
		Metadata: MetadataFrom(&ref),
		FileScannerProviderManifest: types.FileScannerProviderManifest{
			Name:          name,
			ToolReference: ref.Spec.Reference,
		},
		FileScannerProviderStatus: *mps,
	}

	mp.Type = "filescannerprovider"

	return mp, nil
}
