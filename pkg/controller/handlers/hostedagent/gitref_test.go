package hostedagent

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildSource(t *testing.T, agent types.HostedAgentManifest, instance types.HostedAgentInstanceManifest) (string, string) {
	t.Helper()

	desired, err := defaultDesiredBuilder{}.Build(context.Background(), BuildInput{
		Instance: &v1.HostedAgentInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "hai1", Namespace: "n", UID: "uid-1"},
			Spec:       v1.HostedAgentInstanceSpec{Manifest: instance, UserID: "u1"},
		},
		Agent:   &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: agent}},
		Harness: &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return desired.Source.URL, desired.Source.Revision
}

func TestAgentGitRefReachesTheSandbox(t *testing.T) {
	url, ref := buildSource(t,
		types.HostedAgentManifest{GitRepo: "https://example.com/a.git", GitRef: "v1.2.3"},
		types.HostedAgentInstanceManifest{Name: "i"})

	if url != "https://example.com/a.git" || ref != "v1.2.3" {
		t.Fatalf("source = %q @ %q", url, ref)
	}
}

// A ref names a revision of one specific repository. Carrying the agent's ref
// onto a repository the user supplied would check out someone else's revision
// -- or more likely fail to resolve, with an error pointing at the wrong repo.
func TestUserRepoDoesNotInheritTheAgentRef(t *testing.T) {
	url, ref := buildSource(t,
		types.HostedAgentManifest{GitRepo: "https://example.com/a.git", GitRef: "v1.2.3", AllowUserGitRepo: true},
		types.HostedAgentInstanceManifest{Name: "i", GitRepo: "https://example.com/mine.git"})

	if url != "https://example.com/mine.git" {
		t.Fatalf("url = %q, want the user's repository", url)
	}
	if ref != "" {
		t.Fatalf("ref = %q, want the user's repository to use its default branch", ref)
	}
}

func TestUserRefIsUsedWithTheUserRepo(t *testing.T) {
	url, ref := buildSource(t,
		types.HostedAgentManifest{GitRepo: "https://example.com/a.git", GitRef: "v1.2.3", AllowUserGitRepo: true},
		types.HostedAgentInstanceManifest{Name: "i", GitRepo: "https://example.com/mine.git", GitRef: "my-branch"})

	if url != "https://example.com/mine.git" || ref != "my-branch" {
		t.Fatalf("source = %q @ %q", url, ref)
	}
}

// The ref participates in desired state, so changing it has to restart the
// sandbox; otherwise a repinned agent keeps running the old revision.
func TestChangingTheRefChangesTheRevision(t *testing.T) {
	base := types.HostedAgentManifest{GitRepo: "https://example.com/a.git", GitRef: "v1"}
	instance := types.HostedAgentInstanceManifest{Name: "i"}

	first, err := defaultDesiredBuilder{}.Build(context.Background(), BuildInput{
		Instance: &v1.HostedAgentInstance{ObjectMeta: metav1.ObjectMeta{Name: "hai1", UID: "uid-1"}, Spec: v1.HostedAgentInstanceSpec{Manifest: instance}},
		Agent:    &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: base}},
		Harness:  &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	base.GitRef = "v2"
	second, err := defaultDesiredBuilder{}.Build(context.Background(), BuildInput{
		Instance: &v1.HostedAgentInstance{ObjectMeta: metav1.ObjectMeta{Name: "hai1", UID: "uid-1"}, Spec: v1.HostedAgentInstanceSpec{Manifest: instance}},
		Agent:    &v1.HostedAgent{Spec: v1.HostedAgentSpec{Manifest: base}},
		Harness:  &v1.Harness{Spec: v1.HarnessSpec{Manifest: types.HarnessManifest{Image: "img"}}}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if first.Revision == second.Revision {
		t.Fatal("repinning the ref left the revision unchanged, so the sandbox would not restart")
	}
}
