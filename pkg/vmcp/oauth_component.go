package vmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ComponentOAuthAnnotationPrefix = "obot.ai/component-oauth-"
)

// ComponentOAuthCredentialPrefix identifies credentials owned by one vMCP.
func ComponentOAuthCredentialPrefix(vmcpID string) string {
	return "vmcp-oauth-" + utils.Digest(vmcpID)[:32] + "-"
}

// InvalidateCatalogOAuthTokens leaves independently configured components authorized.
func InvalidateCatalogOAuthTokens(ctx context.Context, reader kclient.Reader, creds interface {
	RevealCredential(context.Context, []string, string) (gatewaytypes.Credential, error)
	DeleteMCPOAuthTokenForAllUsers(context.Context, string) error
}, namespace, reference string) error {
	var servers v1.MCPServerList
	if err := reader.List(ctx, &servers, kclient.InNamespace(namespace)); err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, server := range servers.Items {
		if server.Spec.Manifest.Runtime != types.RuntimeRemote {
			continue
		}
		contexts, err := ServerOAuthCredentialContexts(ctx, reader, server)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		found := false
		for _, candidate := range contexts {
			found = found || candidate == reference
		}
		if !found {
			continue
		}
		cred, err := creds.RevealCredential(ctx, contexts, system.StaticOAuthCredentialName)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return err
		}
		if err == nil && cred.Context != reference {
			continue
		}
		ids[server.Name] = true
	}
	var connections v1.MCPServerInstanceList
	if err := reader.List(ctx, &connections, kclient.InNamespace(namespace)); err != nil {
		return err
	}
	for _, connection := range connections.Items {
		if ids[connection.Spec.MCPServerName] {
			if err := creds.DeleteMCPOAuthTokenForAllUsers(ctx, connection.Name); err != nil {
				return err
			}
		}
	}
	for id := range ids {
		if err := creds.DeleteMCPOAuthTokenForAllUsers(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func ComponentOAuthCredentialContext(vmcpID, componentID string) string {
	return ComponentOAuthCredentialPrefix(vmcpID) + utils.Digest(componentID)[:32]
}

func ComponentOAuthAnnotation(componentID string) string {
	return ComponentOAuthAnnotationPrefix + utils.Digest(componentID)[:32]
}

func ComponentOAuthPending(vmcp v1.VMCP, componentID string) bool {
	return strings.HasPrefix(vmcp.Annotations[ComponentOAuthAnnotation(componentID)], "pending:")
}

// ComponentOAuthCredentialContexts keeps overrides separate from catalog defaults.
func ComponentOAuthCredentialContexts(vmcpID string, component types.VMCPComponent) []string {
	if component.CatalogEntry.Manifest.Runtime != types.RuntimeRemote {
		if ref := ComponentOAuthCredentialReference(component); ref != "" {
			return []string{ref}
		}
		return nil
	}
	contexts := []string{ComponentOAuthCredentialContext(vmcpID, component.ID)}
	if ref := ComponentOAuthCredentialReference(component); ref != "" {
		contexts = append(contexts, ref)
	}
	return contexts
}

func ServerVMCP(ctx context.Context, reader kclient.Reader, server v1.MCPServer) (*v1.VMCP, error) {
	vmcpID := server.Spec.VMCPID
	if server.Spec.VMCPInstanceID != "" {
		var instance v1.VMCPInstance
		if err := reader.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: server.Spec.VMCPInstanceID}, &instance); err != nil {
			return nil, err
		}
		vmcpID = instance.Spec.Manifest.VMCPID
	}
	if vmcpID == "" {
		return nil, nil
	}
	var vmcp v1.VMCP
	if err := reader.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: vmcpID}, &vmcp); err != nil {
		return nil, err
	}
	return &vmcp, nil
}

func ServerOAuthCredentialContexts(ctx context.Context, reader kclient.Reader, server v1.MCPServer) ([]string, error) {
	vmcp, err := ServerVMCP(ctx, reader, server)
	if err != nil {
		return nil, err
	}
	if vmcp != nil {
		for _, component := range vmcp.Spec.Manifest.Components {
			if component.ID == server.Spec.VMCPComponentID {
				return ComponentOAuthCredentialContexts(vmcp.Name, component), nil
			}
		}
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "vmcpcomponents"}, server.Spec.VMCPComponentID)
	}
	ref, _, err := ServerOAuthCredentialReference(ctx, reader, server)
	if err != nil || ref == "" {
		return nil, err
	}
	return []string{ref}, nil
}

func ComponentOAuthCredentialCheckHash(req router.Request, vmcp v1.VMCP, component types.VMCPComponent) (string, error) {
	hash, err := OAuthCredentialCheckHash(req, vmcp.Namespace, ComponentOAuthCredentialReference(component), component.MCPServerCatalogEntryID)
	return utils.Digest([]string{hash, vmcp.Name, component.ID, vmcp.Annotations[ComponentOAuthAnnotation(component.ID)]}), err
}

func ServerOAuthCredentialCheckHash(req router.Request, server v1.MCPServer) (string, error) {
	vmcp, err := ServerVMCP(req.Ctx, req.Client, server)
	if err != nil {
		return "", err
	}
	if vmcp != nil {
		for _, component := range vmcp.Spec.Manifest.Components {
			if component.ID == server.Spec.VMCPComponentID {
				return ComponentOAuthCredentialCheckHash(req, *vmcp, component)
			}
		}
		return "", fmt.Errorf("vMCP component not found")
	}
	ref, sourceID, err := ServerOAuthCredentialReference(req.Ctx, req.Client, server)
	if err != nil {
		return "", err
	}
	return OAuthCredentialCheckHash(req, server.Namespace, ref, sourceID)
}

// ComponentOAuthTokenIDs includes dedicated runtimes and shared connections.
func ComponentOAuthTokenIDs(ctx context.Context, reader kclient.Reader, vmcp v1.VMCP, componentID string) ([]string, error) {
	var servers v1.MCPServerList
	if err := reader.List(ctx, &servers, kclient.InNamespace(vmcp.Namespace)); err != nil {
		return nil, err
	}
	var instances v1.VMCPInstanceList
	if err := reader.List(ctx, &instances, kclient.InNamespace(vmcp.Namespace)); err != nil {
		return nil, err
	}
	ownedInstances := map[string]bool{}
	for _, instance := range instances.Items {
		ownedInstances[instance.Name] = instance.Spec.Manifest.VMCPID == vmcp.Name
	}
	ids := map[string]bool{}
	for _, server := range servers.Items {
		if server.Spec.VMCPComponentID == componentID && (server.Spec.VMCPID == vmcp.Name || ownedInstances[server.Spec.VMCPInstanceID]) {
			ids[server.Name] = true
		}
	}
	var connections v1.MCPServerInstanceList
	if err := reader.List(ctx, &connections, kclient.InNamespace(vmcp.Namespace)); err != nil {
		return nil, err
	}
	for _, connection := range connections.Items {
		if ids[connection.Spec.MCPServerName] {
			ids[connection.Name] = true
		}
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	return result, nil
}

// InvalidateComponentOAuthTokens removes every user's authorization for a component.
func InvalidateComponentOAuthTokens(ctx context.Context, reader kclient.Reader, tokens interface {
	DeleteMCPOAuthTokenForAllUsers(context.Context, string) error
}, vmcp v1.VMCP, componentID string) error {
	ids, err := ComponentOAuthTokenIDs(ctx, reader, vmcp, componentID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := tokens.DeleteMCPOAuthTokenForAllUsers(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
