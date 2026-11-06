package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/entitlements"
	"github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// meters are what the plan limits on storage writes. Each kind belongs to one meter, so a write takes one lock.
	meters = []*meter{
		{
			lock:   db.HostedResourcesLock,
			kinds:  []kclient.Object{&v1.VMCP{}, &v1.VMCPInstance{}, &v1.K8sSettings{}},
			limits: []entitlements.Limit{entitlements.Memory, entitlements.CPU},
			usage:  mcp.HostedUsage,
		},
	}
	// meterOf finds the meter that watches a kind.
	meterOf = metersByKind(meters)
)

// Locker takes turns across replicas. The gateway client is one.
type Locker interface {
	WithLock(ctx context.Context, id db.LockID, fn func(tx *gorm.DB) error) error
}

// meter measures usage the plan limits, from every object of its kinds.
type meter struct {
	// lock is held while the meter counts and a write of its kinds runs.
	lock db.LockID
	// kinds are the objects whose writes can change the meter's usage.
	kinds []kclient.Object
	// limits are the plan's limits the meter's usage is checked against.
	limits []entitlements.Limit
	// usage measures what objs use. It only reads objs, so a write can be measured before it's made.
	usage func(objs []kclient.Object) (types.EntitlementsUsage, error)
}

// Limits refuses storage writes that raise a meter's usage past the plan. It's storage's limiter while billing is on.
type Limits struct {
	client kclient.Client
	locker Locker
}

// metersByKind indexes meters by the kinds they watch. A kind with two meters would need two locks, so it panics.
func metersByKind(meters []*meter) map[reflect.Type]*meter {
	byKind := make(map[reflect.Type]*meter)
	for _, m := range meters {
		for _, kind := range m.kinds {
			t := reflect.TypeOf(kind)
			if _, ok := byKind[t]; ok {
				panic(fmt.Sprintf("%s has two meters", t))
			}
			byKind[t] = m
		}
	}
	return byKind
}

func NewLimits(c kclient.Client, locker Locker) *Limits {
	return &Limits{
		client: c,
		locker: locker,
	}
}

// Within runs write, storage's write of obj, if it keeps its meter's usage within the plan. The plan is read, the
// meter counts and write runs under the meter's lock, so replicas take turns and nothing changes in between. Only a
// write that raises a limited usage past its limit is refused, so writes that lower usage always run, even while over.
func (l *Limits) Within(ctx context.Context, obj kclient.Object, write func() error) error {
	m, ok := meterOf[reflect.TypeOf(obj)]
	if !ok {
		// Nothing limits this kind.
		return write()
	}

	return l.locker.WithLock(ctx, m.lock, func(*gorm.DB) error {
		subscription, err := read(ctx, l.client)
		if err != nil {
			return err
		}
		objs, err := listAll(ctx, l.client, m.kinds)
		if err != nil {
			return err
		}
		before, err := m.usage(objs)
		if err != nil {
			return err
		}
		// Measure again as if obj were written.
		after, err := m.usage(replace(objs, obj))
		if err != nil {
			return err
		}
		if err := entitlements.Exceeded(m.limits, EntitlementsOf(subscription.Status), before, after); err != nil {
			return err
		}
		return write()
	})
}

// Refusal returns storage's refusal of a write for going past the plan, if err is one.
func Refusal(err error) (*types.ErrHTTP, bool) {
	if status, ok := errors.AsType[*apierrors.StatusError](err); ok && status.ErrStatus.Code == http.StatusPaymentRequired {
		return types.NewErrHTTP(http.StatusPaymentRequired, status.ErrStatus.Message), true
	}
	return nil, false
}

// Usage measures every meter from c's objects. With a controller's client, listing them registers their kinds, so a
// change to any of them triggers the controller to measure again.
func Usage(ctx context.Context, c kclient.Client) (types.EntitlementsUsage, error) {
	var total types.EntitlementsUsage
	for _, m := range meters {
		objs, err := listAll(ctx, c, m.kinds)
		if err != nil {
			return types.EntitlementsUsage{}, err
		}
		usage, err := m.usage(objs)
		if err != nil {
			return types.EntitlementsUsage{}, err
		}
		// Meters fill different fields, so summing them merges them.
		total = types.EntitlementsUsage{
			Users:         total.Users + usage.Users,
			Devices:       total.Devices + usage.Devices,
			MemoryMiB:     total.MemoryMiB + usage.MemoryMiB,
			CPUMillicores: total.CPUMillicores + usage.CPUMillicores,
		}
	}
	return total, nil
}

// listAll lists every object of kinds in the default namespace, finding each kind's list type in c's scheme.
func listAll(ctx context.Context, c kclient.Client, kinds []kclient.Object) ([]kclient.Object, error) {
	var objs []kclient.Object
	for _, kind := range kinds {
		gvk, err := c.GroupVersionKindFor(kind)
		if err != nil {
			return nil, err
		}
		list, err := c.Scheme().New(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
		if err != nil {
			return nil, err
		}
		items, ok := list.(kclient.ObjectList)
		if !ok {
			return nil, fmt.Errorf("%s isn't a list", gvk.Kind+"List")
		}
		// The count only reads the objects, so a cache needn't copy them.
		if err := c.List(ctx, items, kclient.InNamespace(system.DefaultNamespace), kclient.UnsafeDisableDeepCopy); err != nil {
			return nil, fmt.Errorf("failed to list %s: %w", gvk.Kind, err)
		}
		if err := meta.EachListItem(items, func(item runtime.Object) error {
			objs = append(objs, item.(kclient.Object))
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return objs, nil
}

// replace returns objs with obj in place of the object of the same kind and name, or added if there's none. An
// object without a name is always added, since it's being created with a generated one.
func replace(objs []kclient.Object, obj kclient.Object) []kclient.Object {
	replaced := make([]kclient.Object, 0, len(objs)+1)
	for _, o := range objs {
		if obj.GetName() != "" && reflect.TypeOf(o) == reflect.TypeOf(obj) && o.GetNamespace() == obj.GetNamespace() && o.GetName() == obj.GetName() {
			continue
		}
		replaced = append(replaced, o)
	}
	return append(replaced, obj)
}
