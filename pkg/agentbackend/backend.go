// Package agentbackend defines the implementation-neutral contract between
// Obot's hosted-agent controllers and an agent runtime provider.
package agentbackend

import (
	"context"
	"slices"
	"time"
)

// Backend is the complete desired-state and observation contract required by
// hosted agents. Implementations must make every operation idempotent.
type Backend interface {
	InstanceBackend
	PoolBackend
	UtilizationReader
}

type InstanceBackend interface {
	ReconcileInstance(context.Context, DesiredInstance) (InstanceObservation, error)
	ObserveInstance(context.Context, InstanceRef) (InstanceObservation, error)
	DeleteInstance(context.Context, InstanceRef) (DeleteResult, error)
}

type PoolBackend interface {
	ReconcilePool(context.Context, DesiredPool) (PoolObservation, error)
	ObservePool(context.Context, PoolRef) (PoolObservation, error)
	DeletePool(context.Context, PoolRef) (DeleteResult, error)
}

type UtilizationReader interface {
	GetPoolUtilization(context.Context, PoolRef) (UtilizationSnapshot, error)
}

// Subscriber is an optional fast path for lifecycle changes. Events are hints;
// callers must observe the referenced resource before persisting status.
type Subscriber interface {
	Subscribe(context.Context, func(context.Context, Event) error) error
}

type Event struct {
	Kind     ResourceKind
	Instance InstanceRef
	Pool     PoolRef
}

type ResourceKind string

const (
	ResourceKindInstance ResourceKind = "instance"
	ResourceKindPool     ResourceKind = "pool"
)

type InstanceRef struct {
	ID        string
	Namespace string
	UserID    string
	BackendID string
}

type PoolRef struct {
	ID        string
	BackendID string
}

type DesiredInstance struct {
	Ref      InstanceRef
	Pool     PoolRef
	Revision string

	Name        string
	Description string
	Harness     Harness
	Image       string
	Source      Source
	Model       Model
	Env         map[string]string
	Files       []File
	Secrets     []SecretRef

	// Requests is what this sandbox is guaranteed, and Limits is what it may
	// burst to. Both are derived from the pool rather than declared by the
	// agent: a pool is a shared bucket, and an agent has no inherent size.
	//
	// They are carried here, rather than computed by a backend, so that they
	// participate in the desired revision. Resizing a pool therefore changes
	// every sandbox's revision and restarts it, which is the only way a running
	// sandbox picks up new limits.
	Requests InstanceResources
	Limits   InstanceResources

	// Port is the HTTP port the agent serves on. Zero means it serves nothing,
	// and a backend should publish no address for it.
	Port int
	// Terminal asks the backend to keep the sandbox attachable for an
	// interactive session.
	Terminal bool
}

// InstanceResources is a sandbox's share of its pool, in plain units so the
// contract stays independent of any backend's resource model.
//
// These are plain numbers on purpose. Quantity-style types serialize equal
// values to different strings ("1" and "1000m"), which would make the desired
// revision hash unstable and trigger redeploys that change nothing.
type InstanceResources struct {
	CPUVCPUs    float64
	MemoryBytes int64
}

func (r InstanceResources) IsZero() bool {
	return r.CPUVCPUs == 0 && r.MemoryBytes == 0
}

type Harness struct {
	ID string
	// Interactive asks the backend to allocate a TTY and keep stdin open, the
	// equivalent of `docker run -it`. Without it an image whose entrypoint is a
	// shell exits as soon as it starts.
	Interactive bool
}

type Source struct {
	URL      string
	Revision string
	Subdir   string
}

type Model struct {
	ID       string
	Endpoint string
}

type File struct {
	Path    string
	Content []byte
	Mode    uint32
}

// SecretRef contains routing metadata only. Secret values are managed through
// the provider's secret channel and must never enter this persisted contract.
type SecretRef struct {
	ID       string
	EnvName  string
	FilePath string

	// Version changes whenever the value behind ID changes. It participates in
	// the desired revision so that a rotated secret restarts the sandbox.
	//
	// Agents read their credentials once at startup and cannot reload them, so
	// a sandbox left running across a rotation would hold a credential that no
	// longer works. Versioning the reference propagates the rotation while
	// keeping the value itself out of desired state, status and the revision.
	Version string

	// Value is the secret itself, passed transiently so a backend can write it
	// wherever that backend keeps secrets. It is excluded from the desired
	// revision by DesiredInstance.Redacted, and must never be persisted to
	// instance spec or status, or logged.
	Value string
}

// Redacted returns a copy with every secret value cleared, for hashing into the
// desired revision or for logging. Version still varies with the value, so a
// rotation changes the revision without the revision ever containing a secret.
func (d DesiredInstance) Redacted() DesiredInstance {
	d.Secrets = slices.Clone(d.Secrets)
	for i := range d.Secrets {
		d.Secrets[i].Value = ""
	}
	return d
}

type DesiredPool struct {
	Ref      PoolRef
	Revision string
	Capacity ResourceQuantity
	// MaxSandboxes bounds how many sandboxes share the pool, and so determines
	// each one's guaranteed share.
	MaxSandboxes int
	Suspended    bool
}

type ResourceQuantity struct {
	CPUVCPUs     float64
	MemoryBytes  int64
	StorageBytes int64
}

type State string

const (
	StatePending  State = "pending"
	StateReady    State = "ready"
	StateError    State = "error"
	StateDeleting State = "deleting"
)

type InstanceObservation struct {
	Ref               InstanceRef
	Exists            bool
	ObservedRevision  string
	State             State
	URL               string
	Reason            string
	Message           string
	BackendGeneration int64
}

type PoolObservation struct {
	Ref               PoolRef
	Exists            bool
	ObservedRevision  string
	State             State
	Schedulable       bool
	Capacity          ResourceQuantity
	Reason            string
	Message           string
	BackendGeneration int64
}

type DeleteResult struct {
	Complete bool
}

// UtilizationSnapshot is a live point-in-time sample, not desired state or
// historical accounting.
type UtilizationSnapshot struct {
	Timestamp time.Time
	Pool      ResourceUtilization
	Instances []InstanceUtilization
	Pressure  Pressure
	// StorageMeasured reports whether Pool.StorageBytes is a real measurement.
	// A backend that cannot attribute disk usage to this pool alone leaves it
	// false, so a caller can say so rather than draw an empty disk that is
	// really an unknown one.
	StorageMeasured bool
}

type InstanceUtilization struct {
	Ref         InstanceRef
	State       State
	Utilization ResourceUtilization
}

// ResourceUtilization is live usage. StorageBytes is only ever set on a pool:
// sandboxes share one volume separated by subPath, which the kubelet does not
// measure per directory, so a sandbox's own disk usage is not observable
// without a node agent.
type ResourceUtilization struct {
	CPUVCPUs     float64
	MemoryBytes  int64
	StorageBytes int64
}

type Pressure struct {
	CPU     bool
	Memory  bool
	Storage bool
}
