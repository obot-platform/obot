package kubernetes

import (
	corev1 "k8s.io/api/core/v1"
)

// PodSecurityLevel is the Pod Security Admission level the sandbox namespace
// enforces. Sandboxes share that namespace with MCP servers, so a sandbox that
// does not satisfy the namespace's level is rejected at admission rather than
// failing later -- the Deployment is created and no pod ever appears.
//
// This mirrors the levels the MCP Kubernetes backend applies, and defaults to
// restricted for the same reason: it is what the Helm chart labels the
// namespace with unless an operator lowers it.
type PodSecurityLevel string

const (
	PodSecurityPrivileged PodSecurityLevel = "privileged"
	PodSecurityBaseline   PodSecurityLevel = "baseline"
	PodSecurityRestricted PodSecurityLevel = "restricted"

	// sandboxRunAsUser owns the sandbox's directory on the pool volume. It
	// matches defaultFSGroup so that a restricted sandbox can write to the
	// subPath its pod security context makes it the group owner of.
	sandboxRunAsUser = 1000
)

// ParsePodSecurityLevel maps configuration to a level, defaulting to
// restricted. An unrecognized value is treated as restricted rather than
// rejected: the strict reading is the safe one, and it is also the only one
// that keeps working when the namespace really is restricted.
func ParsePodSecurityLevel(level string) PodSecurityLevel {
	switch PodSecurityLevel(level) {
	case PodSecurityPrivileged:
		return PodSecurityPrivileged
	case PodSecurityBaseline:
		return PodSecurityBaseline
	default:
		return PodSecurityRestricted
	}
}

// podSecurityContext returns the pod-level context for the configured level.
//
// FSGroup and FSGroupChangePolicy are set at every level, including
// privileged: they are not admission requirements but are what gives a sandbox
// write access to its own subdirectory of the shared pool volume without
// recursively chowning the whole volume on each start.
func (b *Backend) podSecurityContext() *corev1.PodSecurityContext {
	securityContext := &corev1.PodSecurityContext{
		FSGroup:             new(b.opts.FSGroup),
		FSGroupChangePolicy: new(corev1.FSGroupChangeOnRootMismatch),
	}

	switch b.opts.PodSecurityLevel {
	case PodSecurityPrivileged:
		return securityContext
	case PodSecurityBaseline:
		securityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
		return securityContext
	default:
		securityContext.RunAsNonRoot = new(true)
		securityContext.RunAsUser = new(int64(sandboxRunAsUser))
		securityContext.RunAsGroup = new(int64(sandboxRunAsUser))
		securityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
		return securityContext
	}
}

// containerSecurityContext returns the container-level context for the
// configured level. Restricted refuses a container that can escalate
// privileges or holds any capability, so both have to be stated explicitly --
// leaving them unset is what a default sandbox would do, and it is rejected.
func (b *Backend) containerSecurityContext() *corev1.SecurityContext {
	switch b.opts.PodSecurityLevel {
	case PodSecurityPrivileged:
		return nil
	case PodSecurityBaseline:
		return &corev1.SecurityContext{
			AllowPrivilegeEscalation: new(false),
		}
	default:
		return &corev1.SecurityContext{
			AllowPrivilegeEscalation: new(false),
			RunAsNonRoot:             new(true),
			RunAsUser:                new(int64(sandboxRunAsUser)),
			RunAsGroup:               new(int64(sandboxRunAsUser)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
	}
}
