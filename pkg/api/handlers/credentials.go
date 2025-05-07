package handlers

import (
	"errors"
	"maps"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

func ListCredentials(req api.Context) error {
	context := req.PathValue("context")
	if context == "" {
		context = req.Namespace()
	}
	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: []string{context},
	})
	if err != nil {
		return err
	}

	var result types.CredentialList
	for _, cred := range creds {
		result.Items = append(result.Items, convertCredential(cred))
	}

	return req.Write(result)
}

func DeleteCredential(req api.Context) error {
	id := req.PathValue("id")
	context := req.PathValue("context")
	if context == "" {
		context = req.Namespace()
	}
	err := req.GPTClient.DeleteCredential(req.Context(), context, id)
	if notFound := (*gptscript.ErrNotFound)(nil); errors.As(err, &notFound) {
		return nil
	}
	return err
}

func convertCredential(cred gptscript.Credential) types.Credential {
	return types.Credential{
		ContextID: cred.Context,
		Name:      cred.ToolName,
		EnvVars:   slices.Sorted(maps.Keys(cred.Env)),
		ExpiresAt: types.NewTimeFromPointer(cred.ExpiresAt),
	}
}

func RecreateAllCredentials(req api.Context) error {
	return req.GPTClient.RecreateAllCredentials(req.Context())
}
