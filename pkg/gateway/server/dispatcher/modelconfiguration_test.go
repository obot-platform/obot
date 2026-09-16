package dispatcher

import (
	"context"
	"errors"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHasModelProvider(t *testing.T) {
	tests := []struct {
		name             string
		deletingOnly     bool
		deletingName     string
		absent           bool
		credentialFree   bool
		env              map[string]string
		storedConfigured bool
		statusError      string
		stale            bool
		pending          bool
		applied          bool
		readErr          bool
		want             bool
		wantErr          bool
	}{
		{
			name:         "last provider pending deletion",
			deletingOnly: true,
			wantErr:      true,
		},
		{
			name:         "deleting provider before configured provider",
			deletingName: "a-deleting",
			env:          map[string]string{"KEY": "secret", "URL": "https://example.com"},
			want:         true,
		},
		{
			name:         "deleting provider after configured provider",
			deletingName: "z-deleting",
			env:          map[string]string{"KEY": "secret", "URL": "https://example.com"},
			want:         true,
		},
		{
			name:         "deleting provider with unconfigured provider",
			deletingName: "a-deleting",
			wantErr:      true,
		},
		{
			name:   "no providers",
			absent: true,
		},
		{
			name: "unconfigured provider",
		},
		{
			name:           "credential free provider",
			credentialFree: true,
			want:           true,
		},
		{
			name: "configured before status catches up",
			env:  map[string]string{"KEY": "secret", "URL": "https://example.com"},
			want: true,
		},
		{
			name: "empty required values match provider semantics",
			env:  map[string]string{"KEY": "", "URL": ""},
			want: true,
		},
		{
			name:        "upstream error is still configured",
			env:         map[string]string{"KEY": "secret", "URL": "https://example.com"},
			statusError: "upstream unavailable",
			want:        true,
		},
		{
			name:    "partial credentials",
			env:     map[string]string{"KEY": "secret"},
			wantErr: true,
		},
		{
			name:        "unconfigured provider with error",
			statusError: "upstream unavailable",
			wantErr:     true,
		},
		{
			name:             "stale configured status",
			storedConfigured: true,
			wantErr:          true,
		},
		{
			name:    "unobserved manifest",
			stale:   true,
			wantErr: true,
		},
		{
			name:    "unreadable credentials",
			readErr: true,
			wantErr: true,
		},
		{
			name:    "pending configuration",
			pending: true,
			wantErr: true,
		},
		{
			name:    "applied change permits absence",
			pending: true,
			applied: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &v1.ModelProvider{
				Name:       "test",
				Namespace:  system.DefaultNamespace,
				Generation: 1,
				Spec:       v1.ModelProviderSpec{RequiredConfigurationParameters: []types.ProviderConfigurationParameter{{Name: "KEY"}, {Name: "URL"}}},
				Status:     v1.ModelProviderStatus{Configured: tt.storedConfigured, ObservedGeneration: 1, MissingConfigurationParameters: []string{"KEY", "URL"}, Error: tt.statusError},
			}

			if tt.credentialFree {
				provider.Spec.RequiredConfigurationParameters = nil
			}

			if tt.stale {
				provider.Generation++
			}

			if tt.deletingOnly {
				provider.DeletionTimestamp = new(metav1.Now())
				provider.Finalizers = []string{"test-cleanup"}
			}

			var objects []kclient.Object
			if tt.deletingName != "" {
				deleting := provider.DeepCopy()
				deleting.Name = tt.deletingName
				deleting.DeletionTimestamp = new(metav1.Now())
				deleting.Finalizers = []string{"test-cleanup"}
				objects = append(objects, deleting)
			}
			if !tt.absent {
				objects = append(objects, provider)
			}

			if tt.pending {
				objects = append(objects, &v1.ProviderConfigurationChange{
					Name:      "change",
					Namespace: system.DefaultNamespace,
					Spec:      v1.ProviderConfigurationChangeSpec{ProviderType: v1.ProviderTypeModel, DesiredState: v1.ProviderDesiredStateDeconfigured},
					Status:    v1.ProviderConfigurationChangeStatus{Applied: tt.applied},
				})
			}

			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
			got, err := hasModelProvider(t.Context(), storage, func(context.Context, v1.ModelProvider) (map[string]string, error) {
				if tt.readErr {
					return nil, errors.New("database down")
				}

				return tt.env, nil
			})
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("got (%v, %v), want (%v, error=%v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestHasModelProviderConfiguredProviderWins(t *testing.T) {
	tests := []struct {
		name             string
		env              map[string]string
		storedConfigured bool
		statusError      string
		stale            bool
		readErr          bool
		pending          bool
	}{
		{
			name: "unconfigured provider",
		},
		{
			name: "partial credentials",
			env:  map[string]string{"KEY": "secret"},
		},
		{
			name:             "stale configured status",
			storedConfigured: true,
		},
		{
			name:        "provider error",
			statusError: "upstream unavailable",
		},
		{
			name:  "unobserved manifest",
			stale: true,
		},
		{
			name:    "unreadable credentials",
			readErr: true,
		},
		{
			name:    "pending configuration",
			pending: true,
		},
	}

	for _, tt := range tests {
		// The fake client lists providers by name, so exercise both orderings.
		for _, configuredName := range []string{"a-configured", "z-configured"} {
			for _, credentialFree := range []bool{false, true} {
				name := tt.name + "/" + configuredName
				if credentialFree {
					name += "/credential-free"
				}

				t.Run(name, func(t *testing.T) {
					provider := &v1.ModelProvider{
						Name:       "test",
						Namespace:  system.DefaultNamespace,
						Generation: 1,
						Spec: v1.ModelProviderSpec{
							RequiredConfigurationParameters: []types.ProviderConfigurationParameter{
								{Name: "KEY"},
								{Name: "URL"},
							},
						},
						Status: v1.ModelProviderStatus{
							Configured:                     tt.storedConfigured,
							ObservedGeneration:             1,
							MissingConfigurationParameters: []string{"KEY", "URL"},
							Error:                          tt.statusError,
						},
					}

					if tt.stale {
						provider.Generation++
					}

					configured := &v1.ModelProvider{
						Name:      configuredName,
						Namespace: system.DefaultNamespace,
						Spec:      provider.Spec,
					}
					if credentialFree {
						configured.Spec.RequiredConfigurationParameters = nil
					}

					objects := []kclient.Object{provider, configured}
					if tt.pending {
						objects = append(objects, &v1.ProviderConfigurationChange{
							Name:      "change",
							Namespace: system.DefaultNamespace,
							Spec: v1.ProviderConfigurationChangeSpec{
								ProviderType: v1.ProviderTypeModel,
								DesiredState: v1.ProviderDesiredStateDeconfigured,
							},
						})
					}

					storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
					got, err := hasModelProvider(t.Context(), storage, func(_ context.Context, provider v1.ModelProvider) (map[string]string, error) {
						if provider.Name == configuredName {
							return map[string]string{"KEY": "secret", "URL": "https://example.com"}, nil
						}
						if tt.readErr {
							return nil, errors.New("database down")
						}
						return tt.env, nil
					})
					if !got || err != nil {
						t.Fatalf("got (%v, %v), want (true, nil)", got, err)
					}
				})
			}
		}
	}
}
