package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func backendWithPodSecurity(t *testing.T, level string) *Backend {
	t.Helper()
	backend, err := New(nil, nil, Options{
		Namespace:        "obot-agents",
		ClusterDomain:    "cluster.local",
		PodSecurityLevel: PodSecurityLevel(level),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return backend
}

func TestParsePodSecurityLevelDefaultsToRestricted(t *testing.T) {
	for _, level := range []string{"", "unknown", "Restricted"} {
		if got := ParsePodSecurityLevel(level); got != PodSecurityRestricted {
			t.Errorf("ParsePodSecurityLevel(%q) = %q, want restricted", level, got)
		}
	}
	if got := ParsePodSecurityLevel("baseline"); got != PodSecurityBaseline {
		t.Errorf("ParsePodSecurityLevel(baseline) = %q", got)
	}
	if got := ParsePodSecurityLevel("privileged"); got != PodSecurityPrivileged {
		t.Errorf("ParsePodSecurityLevel(privileged) = %q", got)
	}
}

// A sandbox that does not satisfy the namespace's Pod Security level is refused
// at admission: the Deployment is created and no pod ever appears. The chart
// labels that namespace restricted by default, so this is what a default
// install requires of every field.
func TestSandboxSatisfiesRestrictedPodSecurity(t *testing.T) {
	backend := backendWithPodSecurity(t, "")
	objects, err := backend.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	spec := deploymentFrom(t, objects).Spec.Template.Spec

	podSecurity := spec.SecurityContext
	if podSecurity == nil {
		t.Fatal("pod has no security context")
	}
	if podSecurity.RunAsNonRoot == nil || !*podSecurity.RunAsNonRoot {
		t.Error("pod securityContext.runAsNonRoot is not true")
	}
	if podSecurity.SeccompProfile == nil || podSecurity.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("pod securityContext.seccompProfile is not RuntimeDefault")
	}
	// The pool volume is shared, and each sandbox owns a subPath within it.
	// Dropping fsGroup to satisfy admission would cost the sandbox write access
	// to its own directory.
	if podSecurity.FSGroup == nil || *podSecurity.FSGroup != defaultFSGroup {
		t.Errorf("pod securityContext.fsGroup = %v, want %d", podSecurity.FSGroup, defaultFSGroup)
	}
	if podSecurity.FSGroupChangePolicy == nil || *podSecurity.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch {
		t.Error("pod securityContext.fsGroupChangePolicy is not OnRootMismatch")
	}

	container := spec.Containers[0].SecurityContext
	if container == nil {
		t.Fatal("container has no security context")
	}
	if container.AllowPrivilegeEscalation == nil || *container.AllowPrivilegeEscalation {
		t.Error("container securityContext.allowPrivilegeEscalation is not false")
	}
	if container.Capabilities == nil || len(container.Capabilities.Drop) != 1 || container.Capabilities.Drop[0] != "ALL" {
		t.Error("container securityContext does not drop ALL capabilities")
	}
	if container.RunAsNonRoot == nil || !*container.RunAsNonRoot {
		t.Error("container securityContext.runAsNonRoot is not true")
	}
	if container.SeccompProfile == nil || container.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("container securityContext.seccompProfile is not RuntimeDefault")
	}
}

// The cleanup job runs in the same namespace and is admitted by the same rules,
// so a sandbox that deletes cleanly under restricted needs this too.
func TestCleanupJobSatisfiesRestrictedPodSecurity(t *testing.T) {
	backend := backendWithPodSecurity(t, "restricted")
	job, err := backend.cleanupJob("inst-1", "alloc-1")
	if err != nil {
		t.Fatalf("cleanupJob: %v", err)
	}
	spec := job.Spec.Template.Spec

	if spec.SecurityContext == nil || spec.SecurityContext.RunAsNonRoot == nil || !*spec.SecurityContext.RunAsNonRoot {
		t.Error("cleanup pod securityContext.runAsNonRoot is not true")
	}
	container := spec.Containers[0].SecurityContext
	if container == nil || container.AllowPrivilegeEscalation == nil || *container.AllowPrivilegeEscalation {
		t.Error("cleanup container securityContext.allowPrivilegeEscalation is not false")
	}
}

// An operator who lowers the namespace's level is telling us a harness needs
// root; forcing runAsNonRoot anyway would break exactly the images the lower
// level was chosen for.
func TestLoweredPodSecurityLevelsRelaxTheSandbox(t *testing.T) {
	baseline := backendWithPodSecurity(t, "baseline")
	objects, err := baseline.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	spec := deploymentFrom(t, objects).Spec.Template.Spec
	if spec.SecurityContext.RunAsNonRoot != nil {
		t.Error("baseline pod should not force runAsNonRoot")
	}
	if spec.SecurityContext.SeccompProfile == nil {
		t.Error("baseline pod should still set a seccomp profile")
	}
	if container := spec.Containers[0].SecurityContext; container == nil || *container.AllowPrivilegeEscalation {
		t.Error("baseline container should disallow privilege escalation")
	}

	privileged := backendWithPodSecurity(t, "privileged")
	objects, err = privileged.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	spec = deploymentFrom(t, objects).Spec.Template.Spec
	if spec.Containers[0].SecurityContext != nil {
		t.Error("privileged container should have no security context")
	}
	// fsGroup is not an admission requirement; it is how the sandbox reaches
	// its own directory on the shared volume, so it survives every level.
	if spec.SecurityContext == nil || spec.SecurityContext.FSGroup == nil {
		t.Error("privileged pod should still set fsGroup for the pool volume")
	}
}
