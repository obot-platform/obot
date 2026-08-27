package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type authProviderTriggerCall struct {
	gvk   schema.GroupVersionKind
	key   string
	delay time.Duration
}

type recordingAuthProviderTrigger struct {
	calls       []authProviderTriggerCall
	errorsByKey map[string]error
}

func (r *recordingAuthProviderTrigger) Trigger(_ context.Context, gvk schema.GroupVersionKind, key string, delay time.Duration) error {
	r.calls = append(r.calls, authProviderTriggerCall{
		gvk:   gvk,
		key:   key,
		delay: delay,
	})
	return r.errorsByKey[key]
}

func TestReconcileAuthProvidersTriggersEveryAuthProvider(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.AuthProvider{
				Name:      "first",
				Namespace: "default",
			},
			&v1.AuthProvider{
				Name:      "second",
				Namespace: "other",
			},
		).
		Build()
	trigger := &recordingAuthProviderTrigger{}

	if err := reconcileAuthProviders(t.Context(), client, trigger); err != nil {
		t.Fatalf("reconcileAuthProviders() error = %v", err)
	}

	if len(trigger.calls) != 2 {
		t.Fatalf("trigger call count = %d, want 2", len(trigger.calls))
	}
	wantGVK := v1.SchemeGroupVersion.WithKind("AuthProvider")
	wantKeys := map[string]bool{
		"default/first": false,
		"other/second":  false,
	}
	for _, call := range trigger.calls {
		if call.gvk != wantGVK {
			t.Errorf("trigger GVK = %v, want %v", call.gvk, wantGVK)
		}
		if call.delay != 0 {
			t.Errorf("trigger delay = %v, want 0", call.delay)
		}
		if _, ok := wantKeys[call.key]; !ok {
			t.Errorf("unexpected trigger key %q", call.key)
		} else {
			wantKeys[call.key] = true
		}
	}
	for key, called := range wantKeys {
		if !called {
			t.Errorf("auth provider %q was not triggered", key)
		}
	}
}

func TestReconcileAuthProvidersContinuesAfterTriggerError(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.AuthProvider{
				Name:      "first",
				Namespace: "default",
			},
			&v1.AuthProvider{
				Name:      "second",
				Namespace: "default",
			},
		).
		Build()
	wantErr := errors.New("trigger failed")
	trigger := &recordingAuthProviderTrigger{
		errorsByKey: map[string]error{
			"default/first": wantErr,
		},
	}

	err := reconcileAuthProviders(t.Context(), client, trigger)
	if !errors.Is(err, wantErr) {
		t.Fatalf("reconcileAuthProviders() error = %v, want an error wrapping %v", err, wantErr)
	}
	if len(trigger.calls) != 2 {
		t.Fatalf("trigger call count = %d, want 2", len(trigger.calls))
	}
}
