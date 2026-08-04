package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func resolverFor(agent types.HostedAgentManifest, harness types.HarnessManifest) *iconResolver {
	return &iconResolver{
		agents:    map[string]types.HostedAgentManifest{"ha1": agent},
		harnesses: map[string]types.HarnessManifest{"hrn1": harness},
	}
}

// The chain exists so a client renders a row from one payload instead of
// fetching the agent and its harness per instance.
func TestIconResolutionOrder(t *testing.T) {
	agent := types.HostedAgentManifest{HarnessID: "hrn1", Icon: "agent.svg", IconDark: "agent-dark.svg"}
	harness := types.HarnessManifest{Icon: "harness.svg", IconDark: "harness-dark.svg"}

	// The user's own choice wins.
	got := resolverFor(agent, harness).apply(
		types.HostedAgentInstance{HostedAgentInstanceManifest: types.HostedAgentInstanceManifest{Icon: "mine.svg"}}, "ha1")
	if got.ResolvedIcon != "mine.svg" {
		t.Errorf("instance icon = %q, want the instance's own", got.ResolvedIcon)
	}

	// Otherwise the agent's.
	got = resolverFor(agent, harness).apply(types.HostedAgentInstance{}, "ha1")
	if got.ResolvedIcon != "agent.svg" || got.ResolvedIconDark != "agent-dark.svg" {
		t.Errorf("resolved = %q/%q, want the agent's", got.ResolvedIcon, got.ResolvedIconDark)
	}

	// Otherwise the harness's, which is where a config source's icon lives when
	// only the harness declares one.
	got = resolverFor(types.HostedAgentManifest{HarnessID: "hrn1"}, harness).apply(types.HostedAgentInstance{}, "ha1")
	if got.ResolvedIcon != "harness.svg" || got.ResolvedIconDark != "harness-dark.svg" {
		t.Errorf("resolved = %q/%q, want the harness's", got.ResolvedIcon, got.ResolvedIconDark)
	}
}

// A dark variant is optional at every level, so a client should never have to
// decide what to do when only the light one is set.
func TestDarkIconFallsBackToLight(t *testing.T) {
	got := resolverFor(types.HostedAgentManifest{HarnessID: "hrn1", Icon: "agent.svg"}, types.HarnessManifest{}).
		apply(types.HostedAgentInstance{}, "ha1")
	if got.ResolvedIconDark != "agent.svg" {
		t.Errorf("dark icon = %q, want it to fall back to the light one", got.ResolvedIconDark)
	}
}

// Nothing anywhere resolves to nothing, rather than to a broken image.
func TestNoIconAnywhereResolvesEmpty(t *testing.T) {
	got := resolverFor(types.HostedAgentManifest{HarnessID: "hrn1"}, types.HarnessManifest{}).
		apply(types.HostedAgentInstance{}, "ha1")
	if got.ResolvedIcon != "" || got.ResolvedIconDark != "" {
		t.Errorf("resolved = %q/%q, want empty", got.ResolvedIcon, got.ResolvedIconDark)
	}
}

// An instance whose agent has been deleted still renders; it just has no icon.
func TestUnknownAgentDoesNotPanic(t *testing.T) {
	got := resolverFor(types.HostedAgentManifest{}, types.HarnessManifest{}).
		apply(types.HostedAgentInstance{HostedAgentInstanceManifest: types.HostedAgentInstanceManifest{Icon: "mine.svg"}}, "gone")
	if got.ResolvedIcon != "mine.svg" {
		t.Errorf("resolved = %q, want the instance's own icon", got.ResolvedIcon)
	}
}
