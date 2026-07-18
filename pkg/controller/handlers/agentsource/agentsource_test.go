package agentsource

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func discoveredHarness(sourceID, relPath string) *v1.Harness {
	return &v1.Harness{
		ObjectMeta: metav1.ObjectMeta{Name: harnessObjectName(sourceID, relPath)},
		Spec: v1.HarnessSpec{
			SourceID:     sourceID,
			RelativePath: relPath,
		},
	}
}

func discoveredAgent(sourceID, relPath, harnessRef string) *v1.HostedAgent {
	agent := &v1.HostedAgent{
		ObjectMeta: metav1.ObjectMeta{Name: agentObjectName(sourceID, relPath)},
		Spec: v1.HostedAgentSpec{
			SourceID:     sourceID,
			RelativePath: relPath,
		},
	}
	agent.Spec.Manifest.HarnessID = harnessRef
	return agent
}

func TestResolveHarnessReferences(t *testing.T) {
	t.Run("path reference resolves to the harness object name", func(t *testing.T) {
		harness := discoveredHarness("as1src", "harnesses/claude-code")
		agent := discoveredAgent("as1src", "agents/reviewer", "harnesses/claude-code")

		require.NoError(t, resolveHarnessReferences([]*v1.HostedAgent{agent}, []*v1.Harness{harness}))
		assert.Equal(t, harness.Name, agent.Spec.Manifest.HarnessID)
	})

	t.Run("full harness ID passes through untouched", func(t *testing.T) {
		ref := system.HarnessPrefix + "abcdef"
		agent := discoveredAgent("as1src", "agents/reviewer", ref)

		require.NoError(t, resolveHarnessReferences([]*v1.HostedAgent{agent}, nil))
		assert.Equal(t, ref, agent.Spec.Manifest.HarnessID)
	})

	t.Run("unknown path reference fails the sync", func(t *testing.T) {
		harness := discoveredHarness("as1src", "harnesses/claude-code")
		agent := discoveredAgent("as1src", "agents/reviewer", "harnesses/missing")

		err := resolveHarnessReferences([]*v1.HostedAgent{agent}, []*v1.Harness{harness})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "harnesses/missing")
		assert.Contains(t, err.Error(), "agents/reviewer")
	})

	t.Run("missing reference fails the sync", func(t *testing.T) {
		agent := discoveredAgent("as1src", "agents/reviewer", "")

		err := resolveHarnessReferences([]*v1.HostedAgent{agent}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not name a harness")
	})
}

func TestSourceObjectNames(t *testing.T) {
	t.Run("deterministic across syncs", func(t *testing.T) {
		assert.Equal(t,
			harnessObjectName("as1src", "harnesses/claude-code"),
			harnessObjectName("as1src", "harnesses/claude-code"),
			"the same definition must map to the same resource on every sync")
	})

	t.Run("kinds and paths do not collide", func(t *testing.T) {
		names := []string{
			harnessObjectName("as1src", "tools/my_thing"),
			// Sanitizes to the same visible fragment; the hash must separate them.
			harnessObjectName("as1src", "tools/my-thing"),
			harnessObjectName("as1other", "tools/my_thing"),
			agentObjectName("as1src", "tools/my_thing"),
		}
		seen := make(map[string]struct{}, len(names))
		for _, n := range names {
			if _, ok := seen[n]; ok {
				t.Fatalf("duplicate object name %q", n)
			}
			seen[n] = struct{}{}
		}
	})

	t.Run("names carry the kind prefix and are valid", func(t *testing.T) {
		harnessName := harnessObjectName("as1src", "härness/…weird path")
		agentName := agentObjectName("as1src", "härness/…weird path")
		assert.True(t, len(harnessName) > len(system.HarnessPrefix) && harnessName[:len(system.HarnessPrefix)] == system.HarnessPrefix)
		assert.True(t, len(agentName) > len(system.HostedAgentPrefix) && agentName[:len(system.HostedAgentPrefix)] == system.HostedAgentPrefix)
		assert.Empty(t, validation.IsDNS1123Subdomain(harnessName))
		assert.Empty(t, validation.IsDNS1123Subdomain(agentName))
	})
}
