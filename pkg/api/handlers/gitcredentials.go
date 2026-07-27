package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gitcredential"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GitCredentialHandler struct{}

func NewGitCredentialHandler() *GitCredentialHandler {
	return nil
}

func (*GitCredentialHandler) List(req api.Context) error {
	var list v1.GitCredentialList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list Git credentials: %w", err)
	}

	items := make([]types.GitCredential, 0, len(list.Items))
	for _, credential := range list.Items {
		configured, err := gitcredential.Configured(req.Context(), req.GatewayClient, credential.Name)
		if err != nil {
			return fmt.Errorf("failed to check Git credential %q: %w", credential.Name, err)
		}
		items = append(items, convertGitCredential(credential, configured))
	}
	return req.Write(types.GitCredentialList{Items: items})
}

func (*GitCredentialHandler) Get(req api.Context) error {
	var credential v1.GitCredential
	if err := req.Get(&credential, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get Git credential: %w", err)
	}
	configured, err := gitcredential.Configured(req.Context(), req.GatewayClient, credential.Name)
	if err != nil {
		return fmt.Errorf("failed to check Git credential %q: %w", credential.Name, err)
	}
	return req.Write(convertGitCredential(credential, configured))
}

func (*GitCredentialHandler) Create(req api.Context) error {
	manifest, host, err := readGitCredentialManifest(req)
	if err != nil {
		return err
	}
	if manifest.Token == "" {
		return types.NewErrBadRequest("token is required")
	}

	credential := v1.GitCredential{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.GitCredentialPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.GitCredentialFinalizer},
		},
		Spec: v1.GitCredentialSpec{
			DisplayName: manifest.DisplayName,
			Host:        host,
		},
	}
	if err := req.Create(&credential); err != nil {
		return fmt.Errorf("failed to create Git credential: %w", err)
	}
	if err := gitcredential.Store(req.Context(), req.GatewayClient, credential.Name, manifest.Token); err != nil {
		_ = req.Delete(&credential)
		return fmt.Errorf("failed to store Git credential token: %w", err)
	}
	return req.WriteCreated(convertGitCredential(credential, true))
}

func (*GitCredentialHandler) Update(req api.Context) error {
	manifest, host, err := readGitCredentialManifest(req)
	if err != nil {
		return err
	}

	var credential v1.GitCredential
	if err := req.Get(&credential, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get Git credential: %w", err)
	}
	if host != credential.Spec.Host {
		return types.NewErrBadRequest("host is immutable")
	}
	configured, err := gitcredential.Configured(req.Context(), req.GatewayClient, credential.Name)
	if err != nil {
		return fmt.Errorf("failed to check Git credential %q: %w", credential.Name, err)
	}
	if manifest.Token == "" && !configured {
		return types.NewErrBadRequest("token is required")
	}

	credential.Spec.DisplayName = manifest.DisplayName
	if err := req.Update(&credential); err != nil {
		return fmt.Errorf("failed to update Git credential: %w", err)
	}
	if manifest.Token != "" {
		if err := gitcredential.Store(req.Context(), req.GatewayClient, credential.Name, manifest.Token); err != nil {
			return fmt.Errorf("failed to store Git credential token: %w", err)
		}
		configured = true
	}
	return req.Write(convertGitCredential(credential, configured))
}

func (*GitCredentialHandler) Delete(req api.Context) error {
	var credential v1.GitCredential
	if err := req.Get(&credential, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get Git credential: %w", err)
	}
	references, err := gitCredentialReferences(req, credential.Name)
	if err != nil {
		return err
	}
	if references.Len() > 0 {
		return types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("Git credential is still used by %d resources", references.Len()))
	}
	if err := req.Delete(&credential); err != nil {
		return fmt.Errorf("failed to delete Git credential: %w", err)
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

func readGitCredentialManifest(req api.Context) (types.GitCredentialManifest, string, error) {
	var manifest types.GitCredentialManifest
	if err := req.Read(&manifest); err != nil {
		return manifest, "", types.NewErrBadRequest("failed to read Git credential manifest: %v", err)
	}
	manifest.DisplayName = strings.TrimSpace(manifest.DisplayName)
	manifest.Token = strings.TrimSpace(manifest.Token)
	if manifest.DisplayName == "" {
		return manifest, "", types.NewErrBadRequest("displayName is required")
	}
	host, err := gitcredential.NormalizeHost(manifest.Host)
	if err != nil {
		return manifest, "", types.NewErrBadRequest("invalid host: %v", err)
	}
	return manifest, host, nil
}

func gitCredentialReferences(req api.Context, credentialID string) (v1.GitCredentialReferences, error) {
	return gitcredential.References(req.Context(), req.Storage, req.Namespace(), credentialID)
}

func convertGitCredential(credential v1.GitCredential, configured bool) types.GitCredential {
	convertUses := func(references []v1.GitCredentialReference) []types.GitCredentialUse {
		uses := make([]types.GitCredentialUse, 0, len(references))
		for _, reference := range references {
			uses = append(uses, types.GitCredentialUse{
				ID:          reference.ID,
				DisplayName: reference.DisplayName,
			})
		}
		return uses
	}
	return types.GitCredential{
		Metadata:        MetadataFrom(&credential),
		DisplayName:     credential.Spec.DisplayName,
		Host:            credential.Spec.Host,
		TokenConfigured: configured,
		Uses: types.GitCredentialUses{
			SkillRepositories: convertUses(credential.Status.References.SkillRepositories),
			MCPCatalogs:       convertUses(credential.Status.References.MCPCatalogs),
			SystemMCPCatalogs: convertUses(credential.Status.References.SystemMCPCatalogs),
		},
	}
}

func validateSharedGitCredential(req api.Context, credentialID, sourceURL string) error {
	if credentialID == "" {
		return nil
	}
	if _, err := gitcredential.Resolve(req.Context(), req.Storage, req.GatewayClient, req.Namespace(), credentialID, sourceURL); err != nil {
		return types.NewErrBadRequest("invalid Git credential reference: %v", err)
	}
	return nil
}

func validateCatalogGitCredentials(req api.Context, sourceURLs []string, references map[string]string) error {
	active := make(map[string]struct{}, len(sourceURLs))
	for _, sourceURL := range sourceURLs {
		active[sourceURL] = struct{}{}
	}
	for sourceURL, credentialID := range references {
		credentialID = strings.TrimSpace(credentialID)
		if credentialID == "" {
			delete(references, sourceURL)
			continue
		}
		if _, ok := active[sourceURL]; !ok {
			return types.NewErrBadRequest("Git credential reference specified for unknown source URL %q", sourceURL)
		}
		references[sourceURL] = credentialID
		if err := validateSharedGitCredential(req, credentialID, sourceURL); err != nil {
			return err
		}
	}
	return nil
}

func remapCatalogSourceValues[T any](originalURLs, normalizedURLs []string, values map[string]T) {
	for i, originalURL := range originalURLs {
		if i >= len(normalizedURLs) || originalURL == normalizedURLs[i] {
			continue
		}
		if value, ok := values[originalURL]; ok {
			delete(values, originalURL)
			values[normalizedURLs[i]] = value
		}
	}
}

func removeSharedCredentialTokens(tokens map[string]string, references map[string]string) {
	for sourceURL, credentialID := range references {
		if credentialID != "" {
			delete(tokens, sourceURL)
		}
	}
}
