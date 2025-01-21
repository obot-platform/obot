package handlers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func SetEnv(req api.Context) error {
	id := req.PathValue("id")
	if id == "" {
		return types.NewErrBadRequest("id path variable is required")
	}

	var envs map[string]string
	if err := req.Read(&envs); err != nil {
		return err
	}

	var errs []error
	for key, val := range envs {
		if err := req.GPTClient.DeleteCredential(req.Context(), id, key); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			errs = append(errs, fmt.Errorf("failed to remove existing credential %q: %w", key, err))
			continue
		}

		if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
			Context:  id,
			ToolName: key,
			Type:     gptscript.CredentialTypeTool,
			Env:      map[string]string{key: val},
		}); err != nil {
			errs = append(errs, fmt.Errorf("failed to create credential %q: %w", key, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	obj, env, err := getObjectAndEnv(req.Context(), req.Storage, req.Namespace(), id)
	if err != nil {
		return err
	}

	for i := 0; i < len(*env); i++ {
		if _, ok := envs[(*env)[i].Name]; !ok {
			// Delete the credential for the store
			if err := req.GPTClient.DeleteCredential(req.Context(), id, (*env)[i].Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				errs = append(errs, fmt.Errorf("failed to remove existing credential %q that is not longer needed: %w", (*env)[i].Name, err))
				continue
			}
			// Remove the item from the slice
			*env = append((*env)[:i], (*env)[i+1:]...)
			i--
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	for name := range envs {
		if !slices.ContainsFunc(*env, func(envVar types.EnvVar) bool {
			return envVar.Name == name
		}) {
			*env = append(*env, types.EnvVar{Name: name})
		}
	}

	if err = req.Update(obj); err != nil {
		return fmt.Errorf("failed to update %s: %w", obj.GetObjectKind().GroupVersionKind().Kind, err)
	}

	return nil
}

func RevealEnv(req api.Context) error {
	id := req.PathValue("id")
	if id == "" {
		return types.NewErrBadRequest("id path variable is required")
	}

	_, env, err := getObjectAndEnv(req.Context(), req.Storage, req.Namespace(), id)
	if err != nil {
		return err
	}

	resp := make(map[string]string, len(*env))
	for _, e := range *env {
		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{id}, e.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return err
		}

		resp[e.Name] = cred.Env[e.Name]
	}

	return req.Write(resp)
}

func getObjectAndEnv(ctx context.Context, client kclient.Client, namespace, id string) (kclient.Object, *[]types.EnvVar, error) {
	switch {
	case system.IsAgentID(id):
		var agent v1.Agent
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: id}, &agent); err != nil {
			return nil, nil, err
		}

		return &agent, &agent.Spec.Manifest.Env, nil

	case system.IsWorkflowID(id):
		var wf v1.Workflow
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: id}, &wf); err != nil {
			return nil, nil, err
		}

		return &wf, &wf.Spec.Manifest.Env, nil

	default:
		return nil, nil, types.NewErrBadRequest("%s is not an agent nor workflow", id)
	}
}
