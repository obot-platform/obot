package handlers

import (
	"fmt"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentbackend"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const hostedAgentPoolDefaultsName = "default"

type HostedAgentPoolHandler struct {
	utilization agentbackend.UtilizationReader
}

func NewHostedAgentPoolHandler(utilization agentbackend.UtilizationReader) *HostedAgentPoolHandler {
	return &HostedAgentPoolHandler{utilization: utilization}
}

func (*HostedAgentPoolHandler) List(req api.Context) error {
	var list v1.HostedAgentPoolList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list hosted agent pools: %w", err)
	}

	allowed := map[string]bool(nil)
	if !req.UserIsAdmin() {
		var err error
		allowed, err = userPoolIDs(req)
		if err != nil {
			return err
		}
	}

	items := make([]types.HostedAgentPool, 0, len(list.Items))
	for _, item := range list.Items {
		if !req.UserIsAdmin() && !allowed[item.Name] {
			continue
		}
		items = append(items, convertHostedAgentPool(item))
	}
	return req.Write(types.HostedAgentPoolList{Items: items})
}

func (*HostedAgentPoolHandler) Get(req api.Context) error {
	var pool v1.HostedAgentPool
	if err := req.Get(&pool, req.PathValue("hosted_agent_pool_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent pool: %w", err)
	}
	if err := requirePoolAccess(req, pool.Name); err != nil {
		return err
	}
	return req.Write(convertHostedAgentPool(pool))
}

func (*HostedAgentPoolHandler) Create(req api.Context) error {
	manifest, err := readHostedAgentPoolManifest(req)
	if err != nil {
		return err
	}
	pool := v1.HostedAgentPool{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "hp1",
			Namespace:    req.Namespace(),
		},
		Spec: v1.HostedAgentPoolSpec{Manifest: manifest},
	}
	if err := req.Create(&pool); err != nil {
		return fmt.Errorf("failed to create hosted agent pool: %w", err)
	}
	return req.WriteCreated(convertHostedAgentPool(pool))
}

func (*HostedAgentPoolHandler) Update(req api.Context) error {
	manifest, err := readHostedAgentPoolManifest(req)
	if err != nil {
		return err
	}
	var pool v1.HostedAgentPool
	if err := req.Get(&pool, req.PathValue("hosted_agent_pool_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent pool: %w", err)
	}
	pool.Spec.Manifest = manifest
	if err := req.Update(&pool); err != nil {
		return fmt.Errorf("failed to update hosted agent pool: %w", err)
	}
	return req.Write(convertHostedAgentPool(pool))
}

func (*HostedAgentPoolHandler) Delete(req api.Context) error {
	poolID := req.PathValue("hosted_agent_pool_id")
	var assignments v1.HostedAgentPoolAssignmentList
	if err := req.List(&assignments, kclient.MatchingFields{
		"spec.poolID": poolID,
	}); err != nil {
		return fmt.Errorf("failed to find hosted agent pool assignments: %w", err)
	}
	if len(assignments.Items) > 0 {
		return types.NewErrBadRequest("hosted agent pool %s is still assigned", poolID)
	}
	return req.Delete(&v1.HostedAgentPool{ObjectMeta: metav1.ObjectMeta{
		Name:      poolID,
		Namespace: req.Namespace(),
	}})
}

func (h *HostedAgentPoolHandler) Utilization(req api.Context) error {
	poolID := req.PathValue("hosted_agent_pool_id")
	var pool v1.HostedAgentPool
	if err := req.Get(&pool, poolID); err != nil {
		return fmt.Errorf("failed to get hosted agent pool: %w", err)
	}
	if err := requirePoolAccess(req, pool.Name); err != nil {
		return err
	}
	if h.utilization == nil {
		return types.NewErrNotFound("hosted agent utilization is unavailable")
	}

	snapshot, err := h.utilization.GetPoolUtilization(req.Context(), agentbackend.PoolRef{
		ID:        pool.Name,
		BackendID: pool.Status.BackendPoolID,
	})
	if err != nil {
		return fmt.Errorf("failed to get hosted agent pool utilization: %w", err)
	}

	// A pool is shared by everyone assigned to it, so the totals
	// describe other people's sandboxes as well as the caller's. The totals are
	// what a user needs in order to understand why their pool is full, so they
	// stay whole; the per-instance breakdown is narrowed to the caller, who has
	// no business seeing a co-tenant's instance IDs or consumption.
	selector := kclient.MatchingFields{"spec.poolID": poolID}
	if !req.UserIsAdmin() {
		selector["spec.userID"] = req.User.GetUID()
	}
	var instances v1.HostedAgentInstanceList
	if err := req.List(&instances, selector); err != nil {
		return fmt.Errorf("failed to map hosted agent instance utilization: %w", err)
	}

	snapshot.Instances = narrowInstanceUtilization(snapshot.Instances, instances.Items)

	return req.Write(convertPoolUtilization(snapshot))
}

// narrowInstanceUtilization keeps only the usage entries belonging to instances
// the caller may see, and rewrites their identity from the backend's stable UID
// to the API resource ID so clients can join usage against instances they
// already hold without learning backend identifiers.
//
// Anything the caller cannot see is dropped rather than anonymised: on a shared
// pool the count and sizes of a co-tenant's sandboxes are themselves
// worth withholding. The pool totals are reported separately and stay whole.
func narrowInstanceUtilization(usages []agentbackend.InstanceUtilization, visible []v1.HostedAgentInstance) []agentbackend.InstanceUtilization {
	ids := make(map[string]string, len(visible))
	for _, instance := range visible {
		ids[string(instance.UID)] = instance.Name
	}

	result := make([]agentbackend.InstanceUtilization, 0, len(usages))
	for _, usage := range usages {
		id, ok := ids[usage.Ref.ID]
		if !ok {
			continue
		}
		usage.Ref.ID = id
		result = append(result, usage)
	}
	return result
}

func readHostedAgentPoolManifest(req api.Context) (types.HostedAgentPoolManifest, error) {
	var manifest types.HostedAgentPoolManifest
	if err := req.Read(&manifest); err != nil {
		return manifest, types.NewErrBadRequest("failed to read hosted agent pool: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return manifest, types.NewErrBadRequest("invalid hosted agent pool: %v", err)
	}
	return manifest, nil
}

func convertHostedAgentPool(pool v1.HostedAgentPool) types.HostedAgentPool {
	return types.HostedAgentPool{
		Metadata:                MetadataFrom(&pool),
		HostedAgentPoolManifest: pool.Spec.Manifest,
		Status:                  pool.Status.HostedAgentPoolStatus,
	}
}

type HostedAgentPoolDefaultsHandler struct{}

func NewHostedAgentPoolDefaultsHandler() *HostedAgentPoolDefaultsHandler {
	return &HostedAgentPoolDefaultsHandler{}
}

func (*HostedAgentPoolDefaultsHandler) Get(req api.Context) error {
	var defaults v1.HostedAgentPoolDefaults
	if err := req.Get(&defaults, hostedAgentPoolDefaultsName); err != nil {
		return fmt.Errorf("failed to get hosted agent pool defaults: %w", err)
	}
	return req.Write(convertHostedAgentPoolDefaults(defaults))
}

func (*HostedAgentPoolDefaultsHandler) Create(req api.Context) error {
	manifest, err := readHostedAgentPoolDefaultsManifest(req)
	if err != nil {
		return err
	}
	defaults := v1.HostedAgentPoolDefaults{
		ObjectMeta: metav1.ObjectMeta{Name: hostedAgentPoolDefaultsName, Namespace: req.Namespace()},
		Spec:       v1.HostedAgentPoolDefaultsSpec{Manifest: manifest},
	}
	if err := req.Create(&defaults); err != nil {
		return fmt.Errorf("failed to create hosted agent pool defaults: %w", err)
	}
	return req.WriteCreated(convertHostedAgentPoolDefaults(defaults))
}

func (*HostedAgentPoolDefaultsHandler) Update(req api.Context) error {
	manifest, err := readHostedAgentPoolDefaultsManifest(req)
	if err != nil {
		return err
	}
	var defaults v1.HostedAgentPoolDefaults
	if err := req.Get(&defaults, hostedAgentPoolDefaultsName); err != nil {
		return fmt.Errorf("failed to get hosted agent pool defaults: %w", err)
	}
	defaults.Spec.Manifest = manifest
	if err := req.Update(&defaults); err != nil {
		return fmt.Errorf("failed to update hosted agent pool defaults: %w", err)
	}
	return req.Write(convertHostedAgentPoolDefaults(defaults))
}

func (*HostedAgentPoolDefaultsHandler) Delete(req api.Context) error {
	return req.Delete(&v1.HostedAgentPoolDefaults{ObjectMeta: metav1.ObjectMeta{
		Name: hostedAgentPoolDefaultsName, Namespace: req.Namespace(),
	}})
}

func readHostedAgentPoolDefaultsManifest(req api.Context) (types.HostedAgentPoolDefaultsManifest, error) {
	var manifest types.HostedAgentPoolDefaultsManifest
	if err := req.Read(&manifest); err != nil {
		return manifest, types.NewErrBadRequest("failed to read hosted agent pool defaults: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return manifest, types.NewErrBadRequest("invalid hosted agent pool defaults: %v", err)
	}
	return manifest, nil
}

func convertHostedAgentPoolDefaults(defaults v1.HostedAgentPoolDefaults) types.HostedAgentPoolDefaults {
	return types.HostedAgentPoolDefaults{
		Metadata:                        MetadataFrom(&defaults),
		HostedAgentPoolDefaultsManifest: defaults.Spec.Manifest,
	}
}

type HostedAgentPoolAssignmentHandler struct{}

func NewHostedAgentPoolAssignmentHandler() *HostedAgentPoolAssignmentHandler {
	return &HostedAgentPoolAssignmentHandler{}
}

func (*HostedAgentPoolAssignmentHandler) List(req api.Context) error {
	options := []kclient.ListOption{}
	if !req.UserIsAdmin() {
		options = append(options, kclient.MatchingFields{"spec.userID": req.User.GetUID()})
	}
	var list v1.HostedAgentPoolAssignmentList
	if err := req.List(&list, options...); err != nil {
		return fmt.Errorf("failed to list hosted agent pool assignments: %w", err)
	}
	items := make([]types.HostedAgentPoolAssignment, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertHostedAgentPoolAssignment(item))
	}
	return req.Write(types.HostedAgentPoolAssignmentList{Items: items})
}

func (*HostedAgentPoolAssignmentHandler) Get(req api.Context) error {
	var assignment v1.HostedAgentPoolAssignment
	if err := req.Get(&assignment, req.PathValue("hosted_agent_pool_assignment_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent pool assignment: %w", err)
	}
	if !req.UserIsAdmin() && assignment.Spec.Manifest.UserID != req.User.GetUID() {
		return types.NewErrNotFound("hosted agent pool assignment not found")
	}
	return req.Write(convertHostedAgentPoolAssignment(assignment))
}

func (*HostedAgentPoolAssignmentHandler) Create(req api.Context) error {
	manifest, err := readHostedAgentPoolAssignmentManifest(req)
	if err != nil {
		return err
	}
	if err := validatePoolAssignment(req, manifest, ""); err != nil {
		return err
	}
	assignment := v1.HostedAgentPoolAssignment{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "hpa1", Namespace: req.Namespace()},
		Spec:       v1.HostedAgentPoolAssignmentSpec{Manifest: manifest},
	}
	if err := req.Create(&assignment); err != nil {
		return fmt.Errorf("failed to create hosted agent pool assignment: %w", err)
	}
	return req.WriteCreated(convertHostedAgentPoolAssignment(assignment))
}

func (*HostedAgentPoolAssignmentHandler) Update(req api.Context) error {
	manifest, err := readHostedAgentPoolAssignmentManifest(req)
	if err != nil {
		return err
	}
	var assignment v1.HostedAgentPoolAssignment
	if err := req.Get(&assignment, req.PathValue("hosted_agent_pool_assignment_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent pool assignment: %w", err)
	}
	if err := validatePoolAssignment(req, manifest, assignment.Name); err != nil {
		return err
	}
	assignment.Spec.Manifest = manifest
	if err := req.Update(&assignment); err != nil {
		return fmt.Errorf("failed to update hosted agent pool assignment: %w", err)
	}
	return req.Write(convertHostedAgentPoolAssignment(assignment))
}

func (*HostedAgentPoolAssignmentHandler) Delete(req api.Context) error {
	return req.Delete(&v1.HostedAgentPoolAssignment{ObjectMeta: metav1.ObjectMeta{
		Name:      req.PathValue("hosted_agent_pool_assignment_id"),
		Namespace: req.Namespace(),
	}})
}

func readHostedAgentPoolAssignmentManifest(req api.Context) (types.HostedAgentPoolAssignmentManifest, error) {
	var manifest types.HostedAgentPoolAssignmentManifest
	if err := req.Read(&manifest); err != nil {
		return manifest, types.NewErrBadRequest("failed to read hosted agent pool assignment: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return manifest, types.NewErrBadRequest("invalid hosted agent pool assignment: %v", err)
	}
	return manifest, nil
}

func validatePoolAssignment(req api.Context, manifest types.HostedAgentPoolAssignmentManifest, currentID string) error {
	if err := req.Get(&v1.HostedAgentPool{}, manifest.PoolID); apierrors.IsNotFound(err) {
		return types.NewErrBadRequest("hosted agent pool %s not found", manifest.PoolID)
	} else if err != nil {
		return fmt.Errorf("failed to get hosted agent pool %s: %w", manifest.PoolID, err)
	}
	if !manifest.Default {
		return nil
	}

	var assignments v1.HostedAgentPoolAssignmentList
	if err := req.List(&assignments, kclient.MatchingFields{
		"spec.userID":  manifest.UserID,
		"spec.default": "true",
	}); err != nil {
		return fmt.Errorf("failed to find default hosted agent pool assignment: %w", err)
	}
	for _, assignment := range assignments.Items {
		if assignment.Name != currentID {
			return types.NewErrBadRequest("user %s already has a default hosted agent pool", manifest.UserID)
		}
	}
	return nil
}

func convertHostedAgentPoolAssignment(assignment v1.HostedAgentPoolAssignment) types.HostedAgentPoolAssignment {
	return types.HostedAgentPoolAssignment{
		Metadata:                          MetadataFrom(&assignment),
		HostedAgentPoolAssignmentManifest: assignment.Spec.Manifest,
	}
}

func userPoolIDs(req api.Context) (map[string]bool, error) {
	var assignments v1.HostedAgentPoolAssignmentList
	if err := req.List(&assignments, kclient.MatchingFields{"spec.userID": req.User.GetUID()}); err != nil {
		return nil, fmt.Errorf("failed to list hosted agent pool assignments: %w", err)
	}
	result := make(map[string]bool, len(assignments.Items))
	for _, assignment := range assignments.Items {
		result[assignment.Spec.Manifest.PoolID] = true
	}
	return result, nil
}

func requirePoolAccess(req api.Context, poolID string) error {
	var pool v1.HostedAgentPool
	if err := req.Get(&pool, poolID); apierrors.IsNotFound(err) {
		return types.NewErrNotFound("hosted agent pool %s not found", poolID)
	} else if err != nil {
		return fmt.Errorf("failed to get hosted agent pool %s: %w", poolID, err)
	}
	if req.UserIsAdmin() {
		return nil
	}
	allowed, err := userPoolIDs(req)
	if err != nil {
		return err
	}
	if !allowed[poolID] {
		return types.NewErrNotFound("hosted agent pool %s not found", poolID)
	}
	return nil
}

type hostedAgentPoolUtilization struct {
	Timestamp time.Time                        `json:"timestamp"`
	Pool      hostedAgentResourceUtilization   `json:"pool"`
	Instances []hostedAgentInstanceUtilization `json:"instances"`
	Pressure  hostedAgentPoolResourcePressure  `json:"pressure"`
	// StorageMeasured distinguishes a pool using no disk from one whose disk
	// cannot be measured. Without it a client cannot tell the two apart, and
	// showing an empty disk bar for an unknown figure is worse than saying so.
	StorageMeasured bool `json:"storageMeasured"`
}

type hostedAgentInstanceUtilization struct {
	InstanceID string                         `json:"instanceID"`
	State      agentbackend.State             `json:"state"`
	Usage      hostedAgentResourceUtilization `json:"usage"`
}

// hostedAgentResourceUtilization is live usage. storageBytes is meaningful only
// on the pool: sandboxes share one volume, so a sandbox's own disk usage is not
// observable and is always reported as zero.
type hostedAgentResourceUtilization struct {
	CPUVCPUs     float64 `json:"cpuVcpus"`
	MemoryBytes  int64   `json:"memoryBytes"`
	StorageBytes int64   `json:"storageBytes"`
}

type hostedAgentPoolResourcePressure struct {
	CPU     bool `json:"cpu"`
	Memory  bool `json:"memory"`
	Storage bool `json:"storage"`
}

func convertPoolUtilization(snapshot agentbackend.UtilizationSnapshot) hostedAgentPoolUtilization {
	result := hostedAgentPoolUtilization{
		Timestamp: snapshot.Timestamp,
		Pool:      convertResourceUtilization(snapshot.Pool),
		Instances: make([]hostedAgentInstanceUtilization, 0, len(snapshot.Instances)),
		Pressure: hostedAgentPoolResourcePressure{
			CPU: snapshot.Pressure.CPU, Memory: snapshot.Pressure.Memory, Storage: snapshot.Pressure.Storage,
		},
		StorageMeasured: snapshot.StorageMeasured,
	}
	for _, instance := range snapshot.Instances {
		result.Instances = append(result.Instances, hostedAgentInstanceUtilization{
			InstanceID: instance.Ref.ID,
			State:      instance.State,
			Usage:      convertResourceUtilization(instance.Utilization),
		})
	}
	return result
}

func convertResourceUtilization(usage agentbackend.ResourceUtilization) hostedAgentResourceUtilization {
	return hostedAgentResourceUtilization{
		CPUVCPUs: usage.CPUVCPUs, MemoryBytes: usage.MemoryBytes, StorageBytes: usage.StorageBytes,
	}
}
