package openapiworker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type CredentialRevealer interface {
	RevealCredential(context.Context, []string, string) (gatewaytypes.Credential, error)
}

type UserHydrator interface {
	UserInfoByID(context.Context, uint) (kuser.Info, error)
}

type ObjectGetter interface {
	Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error
}

type Resolver struct {
	storage     ObjectGetter
	credentials CredentialRevealer
	users       UserHydrator
}

func NewResolver(storage ObjectGetter, credentials CredentialRevealer, users UserHydrator) *Resolver {
	return &Resolver{storage: storage, credentials: credentials, users: users}
}

type ResolvedDescription struct {
	Reference       Reference              `json:"reference"`
	BudgetSourceID  string                 `json:"budgetSourceID"`
	SchemaDigest    string                 `json:"schemaDigest"`
	ExecutionDigest string                 `json:"executionDigest"`
	Operations      []OperationDescription `json:"operations"`
	HeaderPolicies  []HeaderPolicy         `json:"headerPolicies"`
}

type resolvedExecution struct {
	reference        Reference
	description      ResolvedDescription
	manifest         CanonicalOperationManifest
	manifestBytes    []byte
	snapshot         SnapshotEnvelope
	grant            []types.VMCPToolReference
	toolOverrides    []types.ToolOverride
	credentialValues http.Header
}

func (r *Resolver) Describe(ctx context.Context, principal PrincipalContext, reference Reference) (ResolvedDescription, error) {
	resolved, err := r.resolve(ctx, principal, reference)
	if err != nil {
		return ResolvedDescription{}, err
	}
	return resolved.description, nil
}

func (r *Resolver) resolve(ctx context.Context, principal PrincipalContext, reference Reference) (resolvedExecution, error) {
	if err := validateReference(reference); err != nil {
		return resolvedExecution{}, err
	}
	principal, err := r.hydratePrincipal(ctx, principal)
	if err != nil {
		return resolvedExecution{}, err
	}
	if !r.apiKeyScopeAllows(principal, reference) {
		return resolvedExecution{}, NewError(ErrorCodePlatformForbidden, "API key is not authorized for this VMCP")
	}

	connection, selected, vmcp, instance, credentialName, err := r.resolveOwnership(ctx, principal.UserID(), reference)
	if err != nil {
		return resolvedExecution{}, err
	}
	profileAllowed := false
	for _, candidate := range vmcpaccess.EnabledComponents(principal.UserInfo(), *vmcp, []types.VMCPComponent{selected}) {
		if candidate.ID == reference.ComponentID {
			profileAllowed = true
			break
		}
	}
	if !profileAllowed {
		return resolvedExecution{}, NewError(ErrorCodeResourceHidden, "OpenAPI component is unavailable")
	}
	if selected.ID == "" || selected.CatalogEntry.Manifest.Runtime != types.RuntimeOpenAPI || selected.CatalogEntry.Manifest.OpenAPIConfig == nil {
		return resolvedExecution{}, NewError(ErrorCodeResourceHidden, "OpenAPI component is unavailable")
	}
	if selected.CatalogEntry.Manifest.OpenAPIConfig.Schema == nil {
		return resolvedExecution{}, NewError(ErrorCodeProtocolError, "OpenAPI component snapshot is missing")
	}

	schema := []byte(selected.CatalogEntry.Manifest.OpenAPIConfig.Schema.Raw)
	headerConfig := connectionConfig(connection, selected)
	manifest, manifestBytes, policyBytes, executionDigest, err := BuildManifest(schema, *selected.CatalogEntry.Manifest.OpenAPIConfig, headerConfig)
	if err != nil {
		return resolvedExecution{}, err
	}
	policies := headerPolicies(manifest, headerConfig)
	credentials, err := r.resolveCredentials(ctx, principal.UserID(), credentialName, connection, headerConfig)
	if err != nil {
		return resolvedExecution{}, err
	}

	grant := vmcpaccess.InstanceGrant(principal.UserInfo(), *vmcp, *instance)
	operations := make([]OperationDescription, 0, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		if toolEnabled(operation.LegacyToolName, selected.ToolOverrides) && vmcpaccess.ToolGranted(grant, types.VMCPToolReference{ComponentID: selected.ID, Name: operation.LegacyToolName}) {
			operations = append(operations, OperationDescription{
				OperationKey:   operation.OperationKey,
				LegacyToolName: operation.LegacyToolName,
				Method:         operation.Method,
				PathTemplate:   operation.PathTemplate,
				DisplayName:    displayName(operation, selected.ToolOverrides),
				Description:    operation.Description,
				InputSchema:    operationInputSchema(operation),
				Parameters:     append([]Parameter(nil), operation.Parameters...),
			})
		}
	}
	budgetSourceID, err := budgetSourceID(*vmcp, selected)
	if err != nil {
		return resolvedExecution{}, err
	}
	description := ResolvedDescription{
		Reference:       reference,
		BudgetSourceID:  budgetSourceID,
		SchemaDigest:    manifest.SchemaDigest,
		ExecutionDigest: executionDigest,
		Operations:      operations,
		HeaderPolicies:  policies,
	}
	return resolvedExecution{
		reference:     reference,
		description:   description,
		manifest:      manifest,
		manifestBytes: manifestBytes,
		snapshot: SnapshotEnvelope{
			ProtocolVersion: ProtocolVersion,
			SchemaBase64:    base64.StdEncoding.EncodeToString(schema),
			SchemaDigest:    manifest.SchemaDigest,
			Manifest:        json.RawMessage(manifestBytes),
			ManifestBase64:  base64.StdEncoding.EncodeToString(manifestBytes),
			PolicyBase64:    base64.StdEncoding.EncodeToString(policyBytes),
			Policy:          HeaderPolicyDocumentFromManifest(manifest, headerConfig),
			ExecutionDigest: executionDigest,
		},
		grant:            grant,
		toolOverrides:    selected.ToolOverrides,
		credentialValues: credentials,
	}, nil
}

func (r *Resolver) resolveOwnership(ctx context.Context, userID string, reference Reference) (connection v1.MCPServerInstance, component types.VMCPComponent, vmcp *v1.VMCP, instance *v1.VMCPInstance, credentialName string, err error) {
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: reference.InstanceName}
	if getErr := r.storage.Get(ctx, key, &connection); getErr == nil {
		instance = &v1.VMCPInstance{}
		if getErr := r.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: connection.Spec.VMCPInstanceID}, instance); getErr != nil {
			return connection, component, nil, nil, "", hidden(getErr)
		}
		vmcp = &v1.VMCP{}
		if getErr := r.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: instance.Spec.Manifest.VMCPID}, vmcp); getErr != nil {
			return connection, component, nil, instance, "", hidden(getErr)
		}
		if connection.Spec.UserID != userID || instance.Spec.UserID != userID || instance.Spec.Manifest.VMCPID != reference.VMCPID ||
			connection.Spec.VMCPComponentID != reference.ComponentID || !connection.DeletionTimestamp.IsZero() || !instance.DeletionTimestamp.IsZero() {
			return connection, component, nil, instance, "", NewError(ErrorCodeResourceHidden, "OpenAPI component connection is unavailable")
		}
		var server v1.MCPServer
		if getErr := r.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: connection.Spec.MCPServerName}, &server); getErr != nil {
			return connection, component, nil, instance, "", hidden(getErr)
		}
		if server.Spec.VMCPID != reference.VMCPID || server.Spec.VMCPComponentID != reference.ComponentID || server.Spec.VMCPInstanceID != "" ||
			server.Spec.Manifest.Runtime != types.RuntimeOpenAPI || !server.DeletionTimestamp.IsZero() {
			return connection, component, nil, instance, "", NewError(ErrorCodeResourceHidden, "OpenAPI component connection is unavailable")
		}
		for _, candidate := range vmcpaccess.ComponentsForInstance(*vmcp, *instance) {
			if candidate.ID != reference.ComponentID {
				continue
			}
			if vmcpaccess.IsMultiUser(candidate) {
				component = candidate
			}
			break
		}
		if component.ID == "" {
			return connection, component, nil, instance, "", NewError(ErrorCodeResourceHidden, "OpenAPI component connection is unavailable")
		}
		return connection, component, vmcp, instance, connection.Name, nil
	} else if !apierrors.IsNotFound(getErr) {
		return connection, component, nil, nil, "", hidden(getErr)
	}

	var server v1.MCPServer
	if getErr := r.storage.Get(ctx, key, &server); getErr != nil {
		return connection, component, nil, nil, "", hidden(getErr)
	}
	if server.Spec.UserID != userID || server.Spec.VMCPID != reference.VMCPID || server.Spec.VMCPComponentID != reference.ComponentID ||
		server.Spec.VMCPInstanceID == "" || server.Spec.Manifest.Runtime != types.RuntimeOpenAPI || !server.DeletionTimestamp.IsZero() {
		return connection, component, nil, nil, "", NewError(ErrorCodeResourceHidden, "OpenAPI component is unavailable")
	}
	instance = &v1.VMCPInstance{}
	if getErr := r.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: server.Spec.VMCPInstanceID}, instance); getErr != nil {
		return connection, component, nil, nil, "", hidden(getErr)
	}
	vmcp = &v1.VMCP{}
	if getErr := r.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: instance.Spec.Manifest.VMCPID}, vmcp); getErr != nil {
		return connection, component, nil, instance, "", hidden(getErr)
	}
	for _, candidate := range vmcpaccess.ComponentsForInstance(*vmcp, *instance) {
		if candidate.ID == reference.ComponentID {
			component = candidate
			return connection, component, vmcp, instance, server.Name, nil
		}
	}
	return connection, component, nil, instance, "", NewError(ErrorCodeResourceHidden, "OpenAPI component is unavailable")
}

func connectionConfig(connection v1.MCPServerInstance, component types.VMCPComponent) []types.MCPConfig {
	if len(connection.Spec.Config) > 0 {
		return connection.Spec.Config
	}
	return vmcpaccess.ComponentConfig(component)
}

func headerPolicies(manifest CanonicalOperationManifest, config []types.MCPConfig) []HeaderPolicy {
	document := HeaderPolicyDocumentFromManifest(manifest, config)
	policies := make([]HeaderPolicy, 0, len(document.Forwardable)+len(document.Credentials))
	for _, definition := range document.Forwardable {
		policies = append(policies, HeaderPolicy{
			Name: definition.Name, In: "header", Required: definition.Required, Forwardable: true,
		})
	}
	for _, definition := range document.Credentials {
		policies = append(policies, HeaderPolicy{
			Name: definition.Name, In: "header", Required: definition.Required,
			Prefix: definition.Prefix, Sensitive: true, Credential: true,
		})
	}
	return policies
}

func (r *Resolver) resolveCredentials(ctx context.Context, userID, credentialName string, connection v1.MCPServerInstance, config []types.MCPConfig) (http.Header, error) {
	result := http.Header{}
	var required []string
	for _, field := range config {
		if field.Usage != types.Header || !field.UserAllowed {
			continue
		}
		credential, err := r.credentials.RevealCredential(ctx, []string{CredentialContext(userID, credentialName)}, credentialName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if field.Required {
					required = append(required, field.Name)
				}
				continue
			}
			return nil, NewError(ErrorCodeWorkerUnavailable, "cannot resolve business credentials")
		}
		value := credential.Secrets[field.Key]
		if value == "" && field.Required {
			required = append(required, field.Name)
			continue
		}
		if value != "" {
			if field.Prefix != "" && !strings.HasPrefix(value, field.Prefix) {
				value = field.Prefix + value
			}
			result.Set(field.Name, value)
		}
	}
	if len(required) > 0 {
		return nil, NewError(ErrorCodeBusinessAuthorizationRequired, "business authorization is required")
	}
	return result, nil
}

func (r *Resolver) hydratePrincipal(ctx context.Context, principal PrincipalContext) (PrincipalContext, error) {
	if !principal.hydrateNeeded() || r.users == nil {
		return principal, nil
	}
	userID, err := strconv.ParseUint(principal.UserID(), 10, 64)
	if err != nil {
		return principal, NewError(ErrorCodePlatformUnauthenticated, "authenticated user cannot be resolved")
	}
	userInfo, err := r.users.UserInfoByID(ctx, uint(userID))
	if err != nil {
		return principal, NewError(ErrorCodePlatformUnauthenticated, "authenticated user is unavailable")
	}
	return principal.withUserInfo(userInfo), nil
}

func (p PrincipalContext) hydrateNeeded() bool {
	return p.user.GetExtra()["obot_groups"] == nil || p.user.GetExtra()["auth_provider_groups"] == nil
}

func (r *Resolver) apiKeyScopeAllows(principal PrincipalContext, reference Reference) bool {
	if !principal.apiKey {
		return true
	}
	for _, scope := range principal.authorizedVMCPs {
		if scope == "*" || scope == reference.VMCPID || scope == reference.InstanceName {
			return true
		}
	}
	return false
}

func validateReference(reference Reference) error {
	if reference.VMCPID == "" || reference.ComponentID == "" || reference.InstanceName == "" {
		return NewError(ErrorCodeInvalidRequest, "VMCP, component, and instance identifiers are required")
	}
	return nil
}

func toolEnabled(name string, overrides []types.ToolOverride) bool {
	if len(overrides) == 0 {
		return true
	}
	for _, override := range overrides {
		if override.Name == name {
			return override.Enabled
		}
	}
	return false
}

func displayName(operation Operation, overrides []types.ToolOverride) string {
	for _, override := range overrides {
		if override.Name == operation.LegacyToolName && override.OverrideName != "" {
			return override.OverrideName
		}
	}
	return operation.DisplayName
}

func operationInputSchema(operation Operation) json.RawMessage {
	type property struct {
		Type string `json:"type"`
	}
	schema := struct {
		Type       string              `json:"type"`
		Properties map[string]property `json:"properties,omitempty"`
		Required   []string            `json:"required,omitempty"`
	}{
		Type:       "object",
		Properties: map[string]property{},
	}
	for _, parameter := range operation.Parameters {
		parameterType := "string"
		if parameter.Schema != nil {
			var decoded struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(parameter.Schema, &decoded) == nil && decoded.Type != "" {
				parameterType = decoded.Type
			}
		}
		schema.Properties[parameter.Name] = property{Type: parameterType}
		if parameter.Required {
			schema.Required = append(schema.Required, parameter.Name)
		}
	}
	encoded, _ := json.Marshal(schema)
	return encoded
}

func budgetSourceID(vmcp v1.VMCP, component types.VMCPComponent) (string, error) {
	reference := vmcp.Spec.ComponentCatalogReferences[component.ID]
	if reference == "" {
		if vmcp.Spec.AdoptionSourceURL != "" && vmcp.Spec.AdoptionEntryKey != "" {
			reference = vmcp.Spec.AdoptionSourceURL + "::" + vmcp.Spec.AdoptionEntryKey
		} else {
			return "", NewError(ErrorCodeBudgetIdentityUnavailable, "budget source identity is unavailable")
		}
	}
	sum := sha256.Sum256([]byte("obot-openapi-budget-source-v1\x00" + reference))
	return hex.EncodeToString(sum[:]), nil
}

func hidden(err error) error {
	if apierrors.IsNotFound(err) {
		return NewError(ErrorCodeResourceHidden, "OpenAPI resource is unavailable")
	}
	return NewError(ErrorCodeWorkerUnavailable, "cannot resolve OpenAPI resource")
}
