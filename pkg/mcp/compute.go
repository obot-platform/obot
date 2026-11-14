package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mebibyte = 1 << 20
)

// Compute is the memory and CPU that the MCP servers Obot runs request. It's counted from the objects those servers
// are created for, before any of them is deployed, and the K8s settings that set their default requests.
type Compute struct {
	VMCPs         []v1.VMCP
	Instances     []v1.VMCPInstance
	Filters       []v1.MCPWebhookValidation
	SystemServers []v1.SystemMCPServer
	Settings      []v1.K8sSettings
}

// ComputeAdmitter admits storage's writes of the objects that use compute, keeping compute within the entitlements'
// memory and CPU.
type ComputeAdmitter struct {
	storage      kclient.Client
	entitlements gateway.EntitlementProvider
	withLock     func(ctx context.Context, fn func() error) error
}

// ComputeCapacity shows MCP server capacity against the entitlements' memory and CPU when they limit either.
// Otherwise the cluster's capacity is what applies.
type ComputeCapacity struct {
	capacityInfoProvider
	entitlements gateway.EntitlementProvider
	storage      kclient.Reader
}

// capacityInfoProvider reads what MCP server deployments request and what the cluster allows. The session manager is
// one.
type capacityInfoProvider interface {
	GetCapacityInfo(context.Context) (types.MCPCapacityInfo, error)
	GetCapacityInfoForServers(context.Context, []string) (types.MCPCapacityInfo, error)
}

// computeServer is an MCP server Obot runs for one of the objects that use compute.
type computeServer struct {
	name      string
	runtime   types.Runtime
	resources *types.MCPResourceRequirements
}

// ListCompute lists the objects that use compute from c.
func ListCompute(ctx context.Context, c kclient.Reader) (Compute, error) {
	var (
		vmcps         v1.VMCPList
		instances     v1.VMCPInstanceList
		filters       v1.MCPWebhookValidationList
		systemServers v1.SystemMCPServerList
		settings      v1.K8sSettingsList
	)
	for _, list := range []kclient.ObjectList{&vmcps, &instances, &filters, &systemServers, &settings} {
		if err := c.List(ctx, list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return Compute{}, fmt.Errorf("failed to list the objects that use compute: %w", err)
		}
	}
	return Compute{
		VMCPs:         vmcps.Items,
		Instances:     instances.Items,
		Filters:       filters.Items,
		SystemServers: systemServers.Items,
		Settings:      settings.Items,
	}, nil
}

// replace puts obj in place of the stored object of its kind and name, or adds it. obj must be a vMCP, vMCP instance,
// filter or system MCP server.
func (c *Compute) replace(obj kclient.Object) {
	switch obj := obj.(type) {
	case *v1.VMCP:
		c.VMCPs = replaceByName(c.VMCPs, *obj)
	case *v1.VMCPInstance:
		c.Instances = replaceByName(c.Instances, *obj)
	case *v1.MCPWebhookValidation:
		c.Filters = replaceByName(c.Filters, *obj)
	case *v1.SystemMCPServer:
		c.SystemServers = replaceByName(c.SystemServers, *obj)
	}
}

// Usage sums the memory and CPU the servers request under the K8s settings, the way their deployments will. Remote
// and vMCP servers don't run in a pod, so they request nothing.
func (c Compute) Usage() (types.EntitlementsUsage, error) {
	var (
		settings = c.settings()
		usage    types.EntitlementsUsage
	)
	for _, server := range c.servers() {
		if server.runtime == types.RuntimeRemote || server.runtime == types.RuntimeVMCP {
			continue
		}
		resources, err := CoreResourceRequirements(server.resources)
		if err != nil {
			return types.EntitlementsUsage{}, fmt.Errorf("invalid resources for %s: %w", server.name, err)
		}
		container := mcpContainerResources(resources, server.runtime, settings)
		memory, cpu := requested(container, corev1.ResourceMemory), requested(container, corev1.ResourceCPU)
		usage.MemoryMiB += (memory.Value() + mebibyte - 1) / mebibyte
		usage.CPUMillicores += cpu.MilliValue()
	}
	return usage, nil
}

// servers returns the servers the controllers can create for the objects, whether or not they're running yet:
//   - one for each multi-user component of a vMCP
//   - one for each single-user component of an instance
//   - one for each enabled filter, other than guardrail filters, which run in Obot
//   - one for each enabled system MCP server that isn't a filter's
//
// A single-user component counts for every instance, even when no profile enables it for the instance's user:
// profiles follow the user's groups, which change without a write that could be refused. Objects being deleted have
// no servers.
func (c Compute) servers() []computeServer {
	var servers []computeServer
	vmcps := make(map[string]*v1.VMCP, len(c.VMCPs))
	for i := range c.VMCPs {
		vmcp := &c.VMCPs[i]
		if !vmcp.DeletionTimestamp.IsZero() {
			continue
		}
		vmcps[vmcp.Name] = vmcp
		for _, component := range vmcp.Spec.Manifest.Components {
			if vmcpconfig.IsMultiUser(component) {
				servers = append(servers, componentServer(component))
			}
		}
	}

	for _, instance := range c.Instances {
		vmcp, ok := vmcps[instance.Spec.Manifest.VMCPID]
		if !ok || !instance.DeletionTimestamp.IsZero() {
			// The controllers don't run an instance of a missing vMCP.
			continue
		}
		// Only a migrated connection's components differ from its vMCP's.
		components := vmcp.Spec.Manifest.Components
		if len(instance.Spec.LegacyComponents) > 0 || len(instance.Spec.LegacyDisabledComponents) > 0 {
			components = vmcpconfig.ComponentsForInstance(*vmcp, instance)
		}
		for _, component := range components {
			if !vmcpconfig.IsMultiUser(component) {
				servers = append(servers, componentServer(component))
			}
		}
	}

	// The filter controller creates a server for each filter, so creating or enabling a filter is what's refused.
	for i := range c.Filters {
		if server, ok := filterServer(&c.Filters[i]); ok {
			servers = append(servers, server)
		}
	}
	for i := range c.SystemServers {
		if server, ok := systemServer(&c.SystemServers[i]); ok {
			servers = append(servers, server)
		}
	}
	return servers
}

// componentServer returns the server a vMCP component runs.
func componentServer(component types.VMCPComponent) computeServer {
	manifest := component.CatalogEntry.Manifest
	return computeServer{
		name:      fmt.Sprintf("component %q", component.Name),
		runtime:   manifest.Runtime,
		resources: manifest.Resources,
	}
}

// filterServer returns the server a filter runs, if it runs one. A guardrail runs none, and the image doesn't change
// what it requests.
func filterServer(filter *v1.MCPWebhookValidation) (computeServer, bool) {
	if !filter.DeletionTimestamp.IsZero() || filter.Spec.Manifest.Guardrail != nil {
		return computeServer{}, false
	}
	return systemManifestServer(FilterServerManifest(filter, ""))
}

// systemServer returns the server a system MCP server runs, if it runs one. A filter's own server was counted with its
// filter, so writing it never changes usage.
func systemServer(server *v1.SystemMCPServer) (computeServer, bool) {
	if !server.DeletionTimestamp.IsZero() || server.Spec.WebhookValidationName != "" {
		return computeServer{}, false
	}
	return systemManifestServer(server.Spec.Manifest)
}

// systemManifestServer returns the server a system server manifest runs, unless it's disabled. The system server
// controller only deploys a server that isn't disabled.
func systemManifestServer(manifest types.SystemMCPServerManifest) (computeServer, bool) {
	if manifest.Enabled != nil && !*manifest.Enabled {
		return computeServer{}, false
	}
	return computeServer{
		name:      fmt.Sprintf("system MCP server %q", manifest.Name),
		runtime:   manifest.Runtime,
		resources: manifest.Resources,
	}, true
}

// sameRequests reports whether two servers request the same: the same runtime and resources. Limits count too, since
// Kubernetes requests the limit when there's no request.
func sameRequests(a, b computeServer) bool {
	// A server that sets no resources gets the defaults, the same as one that sets them all empty.
	var none types.MCPResourceRequirements
	return a.runtime == b.runtime && *cmp.Or(a.resources, &none) == *cmp.Or(b.resources, &none)
}

// settings returns the spec of the K8s settings that servers are deployed with.
func (c Compute) settings() v1.K8sSettingsSpec {
	for _, settings := range c.Settings {
		if settings.Name == system.K8sSettingsName {
			return settings.Spec
		}
	}
	return v1.K8sSettingsSpec{}
}

// NewComputeAdmitter returns the admitter. storage must read without a cache, so limits are checked against what's
// stored. withLock serializes the writes that change compute across replicas: the gateway client's WithComputeLock.
func NewComputeAdmitter(storage kclient.Client, entitlements gateway.EntitlementProvider, withLock func(ctx context.Context, fn func() error) error) *ComputeAdmitter {
	return &ComputeAdmitter{
		storage:      storage,
		entitlements: entitlements,
		withLock:     withLock,
	}
}

// Admit runs write, storage's write of obj, if it keeps compute within the entitlements' memory and CPU. Without a
// memory or CPU limit, or for a write that can't raise compute, write runs right away. Otherwise compute is counted
// from what's stored and write runs under one lock, so replicas take turns and nothing that changes compute lands in
// between. Only a write that raises compute past a limit is refused, so writes that lower it always run, even while
// over.
func (a *ComputeAdmitter) Admit(ctx context.Context, obj kclient.Object, write func() error) error {
	switch obj.(type) {
	case *v1.VMCP, *v1.VMCPInstance, *v1.MCPWebhookValidation, *v1.SystemMCPServer:
		if obj.GetDeletionTimestamp() != nil {
			// An object being deleted has no servers, so writing it can't raise compute.
			return write()
		}
	default:
		// Nothing limits other kinds. That includes the K8s settings, even though they set the default requests:
		// startup writes them, so refusing one would keep every replica from starting. If they take usage past the
		// limits, the over-limit clock starts instead.
		return write()
	}

	// Read the limits before taking the lock. They come from the cached subscription, which is written without the
	// lock, so holding it never kept them from changing.
	granted, err := a.entitlements.Entitlements(ctx)
	if err != nil {
		return err
	}
	if granted.MemoryMiB == nil && granted.CPUMillicores == nil {
		// Nothing limits compute, no need to count.
		return write()
	}

	cannotRaise, err := a.cannotRaise(ctx, obj)
	if err != nil {
		return err
	}
	if cannotRaise {
		// The write can't raise compute, whatever else is stored, so it doesn't need the lock.
		return write()
	}

	return a.withLock(ctx, func() error {
		// Count what's stored now, then again as if obj were written.
		compute, err := ListCompute(ctx, a.storage)
		if err != nil {
			return err
		}
		before, err := compute.Usage()
		if err != nil {
			return err
		}
		compute.replace(obj)
		after, err := compute.Usage()
		if err != nil {
			return err
		}

		// Each limit is checked on its own, so an unlimited one never refuses.
		if raised(granted.MemoryMiB, before.MemoryMiB, after.MemoryMiB) {
			return newComputeLimitError("memory")
		}
		if raised(granted.CPUMillicores, before.CPUMillicores, after.CPUMillicores) {
			return newComputeLimitError("CPU")
		}
		return write()
	})
}

// cannotRaise reports whether writing obj can't raise compute: an update that keeps the servers counted for it, an
// update of a stale copy, or a filter or system MCP server that runs no server once written. For an update, it reads
// the stored object without a cache.
func (a *ComputeAdmitter) cannotRaise(ctx context.Context, obj kclient.Object) (bool, error) {
	switch obj := obj.(type) {
	case *v1.VMCP:
		return sameAsStored(ctx, a.storage, obj, sameVMCPServers)
	case *v1.VMCPInstance:
		return sameAsStored(ctx, a.storage, obj, sameInstanceServers)
	case *v1.MCPWebhookValidation:
		return keepsServer(ctx, a.storage, obj, filterServer)
	case *v1.SystemMCPServer:
		return keepsServer(ctx, a.storage, obj, systemServer)
	}
	return false, nil
}

// keepsServer reports whether writing obj, which runs at most one server of its own, can't raise compute: once
// written it runs none, or it's an update that keeps the one it runs.
func keepsServer[T any, PT interface {
	*T
	kclient.Object
}](ctx context.Context, storage kclient.Reader, obj PT, server func(PT) (computeServer, bool)) (bool, error) {
	after, runs := server(obj)
	if !runs {
		// Nothing runs once it's written, so the write can only lower compute.
		return true, nil
	}
	return sameAsStored(ctx, storage, obj, func(stored, _ PT) bool {
		before, ran := server(stored)
		return ran && sameRequests(before, after)
	})
}

// sameAsStored reads the object obj updates and reports whether same says the two run the same servers. An update of
// a stale copy changes nothing either.
func sameAsStored[T any, PT interface {
	*T
	kclient.Object
}](ctx context.Context, storage kclient.Reader, obj PT, same func(stored, obj PT) bool) (bool, error) {
	if obj.GetResourceVersion() == "" {
		// Storage fills in the resource version of every update, so this is a create.
		return false, nil
	}

	// Read into an empty object. Get decodes over what it's given, so a copy of obj would keep obj's values wherever
	// the stored object leaves a field out.
	stored := PT(new(T))
	if err := storage.Get(ctx, kclient.ObjectKeyFromObject(obj), stored); err != nil {
		// Nothing's stored to compare with, so count it.
		return false, kclient.IgnoreNotFound(err)
	}

	if stored.GetResourceVersion() != obj.GetResourceVersion() {
		// Storage refuses the write with a conflict unless the stored object is still this version, so there's
		// nothing to count.
		return true, nil
	}
	return same(stored, obj), nil
}

// sameVMCPServers reports whether two versions of a vMCP run the same servers: the same components, in order, each
// with the same fixed configuration. Its instances run its single-user components, so those count too.
//
// A component counts as changed even when its server isn't. Changing it, or its fixed configuration, releases the
// snapshot a migrated instance retains for it (see vmcp.LegacyComponent), so that instance runs a different server
// without a write to it.
func sameVMCPServers(stored, obj *v1.VMCP) bool {
	return slices.EqualFunc(stored.Spec.Manifest.Components, obj.Spec.Manifest.Components, func(a, b types.VMCPComponent) bool {
		// Compare what a retained snapshot is bound to.
		return utils.Digest([]any{a, stored.Spec.ComponentStaticConfigurationHashes[a.ID]}) ==
			utils.Digest([]any{b, obj.Spec.ComponentStaticConfigurationHashes[b.ID]})
	})
}

// sameInstanceServers reports whether two versions of an instance pick the same servers from their vMCP. Its vMCP could
// change while this write skips the lock, so they're compared by what picks them: the same vMCP, and the same
// components a migration retains and disables (see vmcp.ComponentsForInstance).
func sameInstanceServers(stored, obj *v1.VMCPInstance) bool {
	return stored.Spec.Manifest.VMCPID == obj.Spec.Manifest.VMCPID &&
		slices.Equal(stored.Spec.LegacyDisabledComponents, obj.Spec.LegacyDisabledComponents) &&
		slices.EqualFunc(stored.Spec.LegacyComponents, obj.Spec.LegacyComponents, func(a, b types.VMCPComponent) bool {
			// A retained snapshot replaces the vMCP's component while its source digest matches it.
			return a.SourceDigest == b.SourceDigest && sameComponentServer(a, b)
		})
}

// sameComponentServer reports whether two versions of a snapshot an instance retains run the same server: the same
// ID, which a migrated instance retains and disables components by, shared or not, and the same requests.
func sameComponentServer(a, b types.VMCPComponent) bool {
	return a.ID == b.ID &&
		vmcpconfig.IsMultiUser(a) == vmcpconfig.IsMultiUser(b) &&
		sameRequests(componentServer(a), componentServer(b))
}

// ComputeLimitRefusal returns why storage refused a write for taking compute past a limit, if that's what err is.
func ComputeLimitRefusal(err error) (string, bool) {
	if status, ok := errors.AsType[*apierrors.StatusError](err); ok && status.ErrStatus.Code == http.StatusPaymentRequired {
		return status.ErrStatus.Message, true
	}
	return "", false
}

// newComputeLimitError is storage's refusal of a write that takes compute past a limit. Storage hands the client its
// status as is. The message names the limit, not the numbers, since members see it too.
func newComputeLimitError(resource string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusPaymentRequired,
		Message: fmt.Sprintf("There isn't room in the plan's hosted MCP server %s for this. Free some up, or ask an owner to change the plan.", resource),
	}}
}

// raised reports whether going from before to after raises usage past limit. nil is unlimited.
func raised(limit *int64, before, after int64) bool {
	return limit != nil && after > before && after > *limit
}

// requested returns what a container requests of name. Kubernetes requests the limit when a container sets a limit
// without a request, so that counts too.
func requested(resources corev1.ResourceRequirements, name corev1.ResourceName) resource.Quantity {
	if request, ok := resources.Requests[name]; ok {
		return request
	}
	return resources.Limits[name]
}

// replaceByName returns objs with obj in place of the object of the same name, or added if there's none.
func replaceByName[T any, PT interface {
	*T
	kclient.Object
}](objs []T, obj T) []T {
	name := PT(&obj).GetName()
	replaced := make([]T, 0, len(objs)+1)
	for i := range objs {
		if PT(&objs[i]).GetName() != name {
			replaced = append(replaced, objs[i])
		}
	}
	return append(replaced, obj)
}

// NewComputeCapacity returns capacity's view of the capacity the session manager reports.
func NewComputeCapacity(capacity capacityInfoProvider, entitlements gateway.EntitlementProvider, storage kclient.Reader) *ComputeCapacity {
	return &ComputeCapacity{
		capacityInfoProvider: capacity,
		entitlements:         entitlements,
		storage:              storage,
	}
}

// GetCapacityInfo returns the capacity of every MCP server. When the entitlements limit it, what's requested is what
// the entitlements count: what the servers Obot runs request, whether or not they're deployed yet.
func (c *ComputeCapacity) GetCapacityInfo(ctx context.Context) (types.MCPCapacityInfo, error) {
	info, err := c.capacityInfoProvider.GetCapacityInfo(ctx)
	if err != nil {
		return info, err
	}
	info, err = c.withLimits(ctx, info)
	if err != nil || info.Source != types.CapacitySourceEntitlements {
		return info, err
	}

	compute, err := ListCompute(ctx, c.storage)
	if err != nil {
		return info, err
	}
	used, err := compute.Usage()
	if err != nil {
		return info, err
	}
	info.CPURequested = formatCPU(*resource.NewMilliQuantity(used.CPUMillicores, resource.DecimalSI))
	info.MemoryRequested = formatMemory(*resource.NewQuantity(used.MemoryMiB*mebibyte, resource.BinarySI))
	return info, nil
}

// GetCapacityInfoForServers returns the capacity the named servers use. What's requested stays what their deployments
// request, shown against the entitlements' limits.
func (c *ComputeCapacity) GetCapacityInfoForServers(ctx context.Context, names []string) (types.MCPCapacityInfo, error) {
	info, err := c.capacityInfoProvider.GetCapacityInfoForServers(ctx, names)
	if err != nil {
		return info, err
	}
	return c.withLimits(ctx, info)
}

// withLimits replaces the cluster's limits in info with the entitlements', if they limit memory or CPU. An unlimited
// one shows no limit, and a limit of 0 still shows, since nothing fits.
func (c *ComputeCapacity) withLimits(ctx context.Context, info types.MCPCapacityInfo) (types.MCPCapacityInfo, error) {
	granted, err := c.entitlements.Entitlements(ctx)
	if err != nil {
		return info, err
	}
	if granted.MemoryMiB == nil && granted.CPUMillicores == nil {
		return info, nil
	}

	info.Source = types.CapacitySourceEntitlements
	info.CPULimit, info.MemoryLimit = "", ""
	if limit := granted.CPUMillicores; limit != nil {
		info.CPULimit = cmp.Or(formatCPU(*resource.NewMilliQuantity(*limit, resource.DecimalSI)), "0")
	}
	if limit := granted.MemoryMiB; limit != nil {
		info.MemoryLimit = cmp.Or(formatMemory(*resource.NewQuantity(*limit*mebibyte, resource.BinarySI)), "0")
	}
	return info, nil
}
