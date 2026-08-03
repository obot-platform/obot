package kubernetes

import (
	"strings"
	"testing"
)

// The sandbox subdirectory is the only thing separating one agent's workspace
// from every other agent's on a shared pool volume, and it is the argument to
// an rm -rf. A name that reduces to nothing resolves to the pool root, so these
// inputs must be refused rather than sanitized into something plausible.
func TestSandboxSubdirRejectsAnythingThatResolvesToThePoolRoot(t *testing.T) {
	for _, instanceID := range []string{
		"",
		" ",
		".",
		"..",
		"../..",
		"/",
		"//",
		"-",
		"---",
		"!!!",
		"...",
		"./",
		"\x00",
	} {
		t.Run(strings.ReplaceAll(instanceID, "\x00", "NUL"), func(t *testing.T) {
			subdir, err := sandboxSubdir(instanceID)
			if err == nil {
				t.Fatalf("sandboxSubdir(%q) = %q, want an error", instanceID, subdir)
			}
			if subdir != "" {
				t.Errorf("a rejected ID must not yield a name, got %q", subdir)
			}
		})
	}
}

func TestSandboxSubdirAcceptsRealInstanceIDs(t *testing.T) {
	for instanceID, want := range map[string]string{
		"inst-1":             "inst-1",
		"hai1abc123":         "hai1abc123",
		"HAI1ABC123":         "hai1abc123",
		"hai1-abc-123":       "hai1-abc-123",
		"a":                  "a",
		"agent.instance/one": "agent-instance-one",
		// Traversal and separators are destroyed by sanitize before the
		// pattern ever sees them, leaving an ordinary label.
		"/etc/passwd":                 "etc-passwd",
		"../../escape":                "escape",
		strings.Repeat("a", 63):       strings.Repeat("a", 63),
		"-leading-and-trailing-dash-": "leading-and-trailing-dash",
	} {
		got, err := sandboxSubdir(instanceID)
		if err != nil {
			t.Errorf("sandboxSubdir(%q): %v", instanceID, err)
			continue
		}
		if got != want {
			t.Errorf("sandboxSubdir(%q) = %q, want %q", instanceID, got, want)
		}
	}

	// Longer than a DNS label. Kubernetes would reject the mount anyway, so
	// failing here keeps the error next to its cause.
	if _, err := sandboxSubdir(strings.Repeat("a", 64)); err == nil {
		t.Error("a 64-character name should be rejected")
	}
}

// Deleting a sandbox whose ID cannot be validated must fail rather than fall
// back to a job that would remove the whole pool.
func TestCleanupJobRefusesUnusableInstanceIDs(t *testing.T) {
	backend := testBackend(t)
	for _, instanceID := range []string{"", "..", "---"} {
		if job, err := backend.cleanupJob(instanceID, "alloc-1"); err == nil {
			t.Errorf("cleanupJob(%q) built a job targeting %v, want an error",
				instanceID, job.Spec.Template.Spec.Containers[0].Args)
		}
	}
}

func TestInstanceObjectsRefuseUnusableInstanceIDs(t *testing.T) {
	backend := testBackend(t)
	for _, instanceID := range []string{"", "..", "---"} {
		desired := desiredInstance()
		desired.Ref.ID = instanceID
		if _, err := backend.instanceObjects(desired); err == nil {
			t.Errorf("instanceObjects(%q) should refuse to mount the pool root", instanceID)
		}
	}
}

// The subdirectory reaches the shell as an argument, never interpolated into
// the script, and the script re-checks it before running rm.
func TestCleanupJobPassesSubdirAsAnArgument(t *testing.T) {
	backend := testBackend(t)
	job, err := backend.cleanupJob("inst-1", "alloc-1")
	if err != nil {
		t.Fatalf("cleanupJob: %v", err)
	}

	args := job.Spec.Template.Spec.Containers[0].Args
	if len(args) != 3 {
		t.Fatalf("expected script, $0 and the subdirectory, got %v", args)
	}
	if args[0] != cleanupScript {
		t.Error("the first argument must be the guarded script")
	}
	if args[2] != "inst-1" {
		t.Errorf("subdirectory passed as %q, want inst-1", args[2])
	}
	if strings.Contains(args[0], "inst-1") {
		t.Error("the subdirectory must not be interpolated into the script")
	}
	if !strings.Contains(args[0], `rm -rf "/pool/$dir"`) {
		t.Error("the script must quote the target")
	}
	// Without the guard the script would happily run against the pool root.
	for _, refusal := range []string{`''`, `*/*`, `*..*`} {
		if !strings.Contains(args[0], refusal) {
			t.Errorf("the script must refuse %s", refusal)
		}
	}
}
