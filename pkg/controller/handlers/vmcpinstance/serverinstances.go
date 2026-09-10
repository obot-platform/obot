package vmcpinstance

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (*Handler) EnsureMCPServerInstances(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.VMCPInstance)
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Namespace, instance.Spec.Manifest.VMCPID); err != nil {
		return kclient.IgnoreNotFound(err)
	}
	desired := map[string]struct{}{}
	for _, component := range vmcpconfig.ComponentsForInstance(vmcp, *instance) {
		if !vmcpconfig.IsMultiUser(component) {
			continue
		}
		desired[component.ID] = struct{}{}
		serverName := name.SafeConcatName(system.MCPServerPrefix+vmcp.Name, component.ID)
		var server v1.MCPServer
		if err := req.Get(&server, instance.Namespace, serverName); apierrors.IsNotFound(err) {
			// The vMCP controller creates the shared server; req.Get watches it.
			continue
		} else if err != nil {
			return err
		}
		if server.Spec.VMCPID != vmcp.Name || server.Spec.VMCPComponentID != component.ID || server.Spec.VMCPInstanceID != "" {
			return fmt.Errorf("MCPServer %q has different vMCP ownership", serverName)
		}
		if !server.DeletionTimestamp.IsZero() {
			continue
		}
		connection := v1.MCPServerInstance{
			Name:       name.SafeConcatName(system.MCPServerInstancePrefix+instance.Name, component.ID),
			Namespace:  instance.Namespace,
			Finalizers: []string{v1.MCPServerInstanceFinalizer},
			Spec: v1.MCPServerInstanceSpec{
				UserID:          instance.Spec.UserID,
				MCPServerName:   serverName,
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: component.ID,
			},
		}
		var existing v1.MCPServerInstance
		if err := req.Get(&existing, connection.Namespace, connection.Name); err == nil {
			if existing.Spec.UserID != connection.Spec.UserID || existing.Spec.MCPServerName != serverName || existing.Spec.VMCPInstanceID != instance.Name || existing.Spec.VMCPComponentID != component.ID {
				return fmt.Errorf("MCPServerInstance %q has different vMCP ownership", connection.Name)
			}
		} else if !apierrors.IsNotFound(err) {
			return err
		} else if err := req.Client.Create(req.Ctx, &connection); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	var existing v1.MCPServerInstanceList
	if err := req.List(&existing, &kclient.ListOptions{Namespace: instance.Namespace, FieldSelector: fields.OneTermEqualSelector("spec.vmcpInstanceID", instance.Name)}); err != nil {
		return err
	}
	for i := range existing.Items {
		if _, ok := desired[existing.Items[i].Spec.VMCPComponentID]; !ok {
			if err := req.Client.Delete(req.Ctx, &existing.Items[i]); kclient.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}
	return nil
}
