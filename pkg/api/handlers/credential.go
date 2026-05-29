package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const tool = `
Name: Obot Credential Store Helper
Share Tools: store, get, list, erase

---
name: store

#!http://localhost:%d/api/credentials/store

---
name: get

#!http://localhost:%[1]d/api/credentials/get

---
name: list

#!http://localhost:%[1]d/api/credentials/list

---
name: erase

#!http://localhost:%[1]d/api/credentials/erase
`

type CredentialHandler struct {
	tool  string
	token string
}

func NewCredentialHandler(port int, token string) *CredentialHandler {
	return &CredentialHandler{
		tool:  fmt.Sprintf(tool, port),
		token: token,
	}
}

func (h *CredentialHandler) Tool(req api.Context) error {
	return req.Write(h.tool)
}

func (h *CredentialHandler) Store(req api.Context) error {
	if err := h.auth(req); err != nil {
		return err
	}

	var body struct {
		ServerURL string
		Username  string
		Secret    string
	}

	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	if len(body.ServerURL) == 0 {
		return types.NewErrBadRequest("serverURL is required")
	}

	name, context, ok := strings.Cut(body.ServerURL, "///")
	if !ok {
		return types.NewErrBadRequest("invalid server URL: %s", body.ServerURL)
	}

	var credEnv struct {
		Env map[string]string
	}
	if err := json.Unmarshal([]byte(body.Secret), &credEnv); err != nil {
		return types.NewErrBadRequest("invalid secret format: %v", err)
	}

	cred := gtypes.Credential{
		Context: context,
		Name:    name,
		Secrets: credEnv.Env,
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), cred); err != nil {
		return fmt.Errorf("failed to store credential: %w", err)
	}

	req.WriteHeader(http.StatusOK)

	return nil
}

func (h *CredentialHandler) Get(req api.Context) error {
	if err := h.auth(req); err != nil {
		return err
	}

	serverURL, err := req.Body()
	if err != nil || len(serverURL) == 0 {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	name, context, ok := strings.Cut(string(serverURL), "///")
	if !ok {
		return types.NewErrBadRequest("invalid server URL: %s", string(serverURL))
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{context}, name)
	if err != nil {
		if errors.As(err, &gclient.CredentialNotFoundError{}) {
			// Return empty credential when not found.
			return req.Write(credentialResponse{})
		}
		return err
	}

	secret, _ := json.Marshal(map[string]any{"env": cred.Secrets})
	return req.Write(credentialResponse{
		ServerURL: string(serverURL),
		Username:  "tool",
		Secret:    string(secret),
	})
}

func (h *CredentialHandler) List(req api.Context) error {
	if err := h.auth(req); err != nil {
		return err
	}

	var contexts []string
	if err := req.Read(&contexts); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	creds, err := req.GatewayClient.ListCredentials(req.Context(), gclient.ListCredentialsOptions{
		CredentialContexts: contexts,
		AllContexts:        len(contexts) == 0,
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	var result []credentialResponse
	for _, cred := range creds {
		secret, _ := json.Marshal(map[string]any{"env": cred.Secrets})
		result = append(result, credentialResponse{
			ServerURL: fmt.Sprintf("%s///%s", cred.Name, cred.Context),
			Username:  "tool",
			Secret:    string(secret),
		})
	}

	return req.Write(result)
}

func (h *CredentialHandler) Erase(req api.Context) error {
	if err := h.auth(req); err != nil {
		return err
	}

	serverURL, err := req.Body()
	if err != nil || len(serverURL) == 0 {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	name, context, ok := strings.Cut(string(serverURL), "///")
	if !ok {
		return types.NewErrBadRequest("invalid server URL: %s", string(serverURL))
	}

	if _, err := req.GatewayClient.DeleteCredential(req.Context(), context, name); err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	return nil
}

func (h *CredentialHandler) auth(req api.Context) error {
	if envHeader := req.Request.Header.Get("X-GPTScript-Env"); envHeader != "" {
		for envVar := range strings.SplitSeq(envHeader, ",") {
			token, ok := strings.CutPrefix(envVar, "OBOT_CREDSTORE_API_TOKEN=")
			if ok && token == h.token {
				return nil
			}
		}
	}

	return types.NewErrHTTP(http.StatusUnauthorized, "missing or invalid token")
}

type credentialResponse struct {
	ServerURL string `json:"serverURL"`
	Username  string `json:"username"`
	Secret    string `json:"secret"`
}
