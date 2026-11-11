package storage

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// testAdmitter refuses catalogs whose display name is "refused", and records the catalogs it's asked about.
type testAdmitter struct {
	lock    sync.Mutex
	limited []string
}

func (l *testAdmitter) Admit(_ context.Context, obj kclient.Object, write func() error) error {
	catalog, ok := obj.(*v1.MCPCatalog)
	if !ok {
		return write()
	}

	l.lock.Lock()
	l.limited = append(l.limited, catalog.Spec.DisplayName)
	l.lock.Unlock()
	if catalog.Spec.DisplayName == "refused" {
		return &apierrors.StatusError{ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusPaymentRequired,
			Message: "over the plan",
		}}
	}
	return write()
}

func (l *testAdmitter) seen() []string {
	l.lock.Lock()
	defer l.lock.Unlock()
	return slices.Clone(l.limited)
}

func TestStorageAdmitsWrites(t *testing.T) {
	c, _, _, services, err := Start(t.Context(), sservices.Config{
		DSN: "sqlite://file:" + filepath.Join(t.TempDir(), "obot.db") + "?_journal=WAL&cache=shared&_busy_timeout=30000",
	})
	if err != nil {
		t.Fatal(err)
	}
	admitter := new(testAdmitter)
	services.Admission.Set(admitter)

	wantRefused := func(t *testing.T, err error) {
		t.Helper()
		if status, ok := errors.AsType[*apierrors.StatusError](err); !ok ||
			status.ErrStatus.Code != http.StatusPaymentRequired || status.ErrStatus.Message != "over the plan" {
			t.Fatalf("error = %v, want the admitter's refusal", err)
		}
	}
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "catalog"}
	stored := func(t *testing.T) *v1.MCPCatalog {
		t.Helper()
		var catalog v1.MCPCatalog
		if err := c.Get(t.Context(), key, &catalog); err != nil {
			t.Fatal(err)
		}
		return &catalog
	}

	// Creates are admitted.
	wantRefused(t, c.Create(t.Context(), &v1.MCPCatalog{
		Namespace: key.Namespace,
		Name:      key.Name,
		Spec: v1.MCPCatalogSpec{
			DisplayName: "refused",
		},
	}))
	if err := c.Get(t.Context(), key, new(v1.MCPCatalog)); !apierrors.IsNotFound(err) {
		t.Fatalf("Get() after a refused create = %v, want not found", err)
	}
	catalog := &v1.MCPCatalog{
		Namespace: key.Namespace,
		Name:      key.Name,
		Spec: v1.MCPCatalogSpec{
			DisplayName: "created",
		},
	}
	if err := c.Create(t.Context(), catalog); err != nil {
		t.Fatal(err)
	}

	// Updates are admitted.
	catalog.Spec.DisplayName = "refused"
	wantRefused(t, c.Update(t.Context(), catalog))
	catalog = stored(t)
	catalog.Spec.DisplayName = "updated"
	if err := c.Update(t.Context(), catalog); err != nil {
		t.Fatal(err)
	}

	// Patches become updates, so they're admitted too.
	wantRefused(t, c.Patch(t.Context(), stored(t), kclient.RawPatch(kclient.Merge.Type(), []byte(`{"spec":{"displayName":"refused"}}`))))
	if err := c.Patch(t.Context(), stored(t), kclient.RawPatch(kclient.Merge.Type(), []byte(`{"spec":{"displayName":"patched"}}`))); err != nil {
		t.Fatal(err)
	}
	if got := stored(t).Spec.DisplayName; got != "patched" {
		t.Fatalf("display name = %q, want patched", got)
	}

	// Status writes skip admission.
	catalog = stored(t)
	catalog.Status.IsSyncing = true
	if err := c.Status().Update(t.Context(), catalog); err != nil {
		t.Fatal(err)
	}

	want := []string{"refused", "created", "refused", "updated", "refused", "patched"}
	if got := admitter.seen(); !slices.Equal(got, want) {
		t.Errorf("admitted writes = %v, want %v", got, want)
	}
}
