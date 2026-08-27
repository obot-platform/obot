package services

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAgentsEnabled(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := []struct {
		name       string
		configured *bool
		agent      *v1.NanobotAgent
		want       bool
	}{
		{
			name:       "explicitly enabled",
			configured: &trueValue,
			want:       true,
		},
		{
			name:       "explicitly disabled with existing agent",
			configured: &falseValue,
			agent:      &v1.NanobotAgent{Name: "agent", Namespace: "default"},
			want:       false,
		},
		{
			name: "unset on new deployment",
			want: false,
		},
		{
			name:  "unset with existing agent",
			agent: &v1.NanobotAgent{Name: "agent", Namespace: "default"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(storagescheme.Scheme)
			if tt.agent != nil {
				builder = builder.WithObjects(tt.agent)
			}

			services := Services{
				StorageClient: builder.Build(),
				EnableAgents:  tt.configured,
			}
			got, err := services.AgentsEnabled(t.Context())
			if err != nil {
				t.Fatalf("AgentsEnabled() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("AgentsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentsEnabledCachesGrandfatheringDecision(t *testing.T) {
	agent := &v1.NanobotAgent{Name: "agent", Namespace: "default"}
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(agent).
		Build()
	services := Services{StorageClient: client}

	enabled, err := services.AgentsEnabled(t.Context())
	if err != nil {
		t.Fatalf("AgentsEnabled() error = %v", err)
	}
	if !enabled {
		t.Fatal("AgentsEnabled() = false, want true")
	}

	if err := client.Delete(t.Context(), agent); err != nil {
		t.Fatalf("failed to delete agent: %v", err)
	}

	enabled, err = services.AgentsEnabled(t.Context())
	if err != nil {
		t.Fatalf("AgentsEnabled() after deleting agent error = %v", err)
	}
	if !enabled {
		t.Fatal("AgentsEnabled() after deleting agent = false, want cached true")
	}
}
