package mcp

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mebibyte = 1 << 20
)

// HostedUsage measures what the hosted MCP servers the vMCP and vMCP instance controllers create from objs request:
// one server for each hosted multi-user component of a vMCP, and one for each hosted single-user component of an
// instance. The K8s settings in objs set the default requests. Objects being deleted don't count.
func HostedUsage(objs []kclient.Object) (types.EntitlementsUsage, error) {
	var (
		vmcps     = make(map[string]*v1.VMCP)
		instances []*v1.VMCPInstance
		settings  v1.K8sSettingsSpec
	)
	for _, obj := range objs {
		if !obj.GetDeletionTimestamp().IsZero() {
			continue
		}
		switch obj := obj.(type) {
		case *v1.VMCP:
			vmcps[obj.Name] = obj
		case *v1.VMCPInstance:
			instances = append(instances, obj)
		case *v1.K8sSettings:
			settings = obj.Spec
		}
	}

	var usage types.EntitlementsUsage
	add := func(component types.VMCPComponent) error {
		manifest := component.CatalogEntry.Manifest
		if manifest.Runtime == types.RuntimeRemote || manifest.Runtime == types.RuntimeVMCP {
			// Remote and vMCP servers don't run in a pod.
			return nil
		}
		resources, err := CoreResourceRequirements(manifest.Resources)
		if err != nil {
			return fmt.Errorf("invalid resources for component %q: %w", component.Name, err)
		}
		container := mcpContainerResources(resources, manifest.Runtime, settings)
		memory, cpu := requested(container, corev1.ResourceMemory), requested(container, corev1.ResourceCPU)
		usage.MemoryMiB += (memory.Value() + mebibyte - 1) / mebibyte
		usage.CPUMillicores += cpu.MilliValue()
		return nil
	}

	for _, vmcp := range vmcps {
		for _, component := range vmcp.Spec.Manifest.Components {
			if !vmcpconfig.IsMultiUser(component) {
				continue
			}
			if err := add(component); err != nil {
				return types.EntitlementsUsage{}, err
			}
		}
	}
	for _, instance := range instances {
		vmcp, ok := vmcps[instance.Spec.Manifest.VMCPID]
		if !ok {
			// The controllers don't run an instance of a missing vMCP.
			continue
		}
		for _, component := range vmcpconfig.ComponentsForInstance(*vmcp, *instance) {
			if vmcpconfig.IsMultiUser(component) {
				continue
			}
			if err := add(component); err != nil {
				return types.EntitlementsUsage{}, err
			}
		}
	}
	return usage, nil
}

// requested returns what a container requests of name. Kubernetes requests the limit when a container sets a limit
// without a request, so that counts too.
func requested(resources corev1.ResourceRequirements, name corev1.ResourceName) resource.Quantity {
	if request, ok := resources.Requests[name]; ok {
		return request
	}
	return resources.Limits[name]
}
