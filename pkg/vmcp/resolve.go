package vmcp

import (
	"cmp"
	"context"
	"slices"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveConnectID resolves both current IDs and migrated composite aliases.
// A nil VMCP means the ID still belongs to the legacy resource model.
// Instance aliases always identify one connection, never another user's connection.
func ResolveConnectID(ctx context.Context, client kclient.Client, id, userID string) (*v1.VMCP, *v1.VMCPInstance, error) {
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}
	var (
		vmcp     v1.VMCP
		instance *v1.VMCPInstance
	)
	switch {
	case system.IsVMCPID(id):
		if err := client.Get(ctx, key, &vmcp); err != nil {
			return nil, nil, err
		}
	case system.IsVMCPInstanceID(id):
		instance = &v1.VMCPInstance{}
		if err := client.Get(ctx, key, instance); err != nil {
			return nil, nil, err
		}
	default:
		// Existing legacy resources remain authoritative until migration finishes.
		var old kclient.Object = new(v1.MCPServerCatalogEntry)
		if system.IsMCPServerID(id) {
			old = new(v1.MCPServer)
		}
		if system.IsMCPServerInstanceID(id) || system.IsSystemMCPServerID(id) {
			return nil, nil, nil
		}
		if err := client.Get(ctx, key, old); err == nil {
			return nil, nil, nil
		} else if !apierrors.IsNotFound(err) {
			return nil, nil, err
		}
		if system.IsMCPServerID(id) {
			var instances v1.VMCPInstanceList
			if err := client.List(ctx, &instances, kclient.InNamespace(key.Namespace), kclient.MatchingFields{"spec.legacySlug": id}); err != nil {
				return nil, nil, err
			}
			if len(instances.Items) == 0 {
				return nil, nil, nil
			}
			instance = &instances.Items[0]
		} else {
			var vmcps v1.VMCPList
			if err := client.List(ctx, &vmcps, kclient.InNamespace(key.Namespace), kclient.MatchingFields{"spec.legacySlug": id}); err != nil {
				return nil, nil, err
			}
			if len(vmcps.Items) == 0 {
				return nil, nil, nil
			}
			vmcp = vmcps.Items[0]
		}
	}
	if instance != nil {
		if instance.Spec.UserID != userID {
			return nil, nil, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "vmcpinstance"}, id)
		}
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: instance.Namespace, Name: instance.Spec.Manifest.VMCPID}, &vmcp); err != nil {
			return nil, nil, err
		}
	}
	return &vmcp, instance, nil
}

// FindInstance chooses the oldest connection for a canonical vMCP URL. Migrated
// users may have several connections; their legacy URLs still select each one.
func FindInstance(ctx context.Context, client kclient.Client, namespace, vmcpID, userID string) (*v1.VMCPInstance, error) {
	var instances v1.VMCPInstanceList
	if err := client.List(ctx, &instances, kclient.InNamespace(namespace), kclient.MatchingFields{"spec.manifest.vmcpID": vmcpID, "spec.userID": userID}); err != nil {
		return nil, err
	}
	if len(instances.Items) == 0 {
		return nil, nil
	}
	slices.SortFunc(instances.Items, func(a, b v1.VMCPInstance) int {
		aTime, bTime := a.CreationTimestamp, b.CreationTimestamp
		if a.Spec.LegacyCreatedAt != nil {
			aTime = *a.Spec.LegacyCreatedAt
		}
		if b.Spec.LegacyCreatedAt != nil {
			bTime = *b.Spec.LegacyCreatedAt
		}
		if result := aTime.Compare(bTime.Time); result != 0 {
			return result
		}
		if result := cmp.Compare(a.Spec.LegacySlug, b.Spec.LegacySlug); result != 0 {
			return result
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return &instances.Items[0], nil
}
