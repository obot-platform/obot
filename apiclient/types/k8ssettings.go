package types

// K8sSettings represents global Kubernetes configuration for MCP server deployments
type K8sSettings struct {
	// Affinity rules (JSON/YAML blob)
	Affinity string `json:"affinity,omitempty"`

	// Tolerations (JSON/YAML blob)
	Tolerations string `json:"tolerations,omitempty"`

	// Resources configuration (JSON/YAML blob)
	Resources string `json:"resources,omitempty"`

	// RuntimeClassName specifies the RuntimeClass for MCP server pods
	// This allows running MCP servers with specific container runtimes (e.g., gVisor, Kata)
	RuntimeClassName string `json:"runtimeClassName,omitempty"`

	// StorageClassName specifies the StorageClass for nanobot workspace volumes
	StorageClassName string `json:"storageClassName,omitempty"`

	// NanobotWorkspaceSize specifies the size for nanobot workspace volumes
	NanobotWorkspaceSize string `json:"nanobotWorkspaceSize,omitempty"`

	// PodSecurityAdmission contains Pod Security Admission settings for the MCP namespace
	PodSecurityAdmission *PodSecurityAdmissionSettings `json:"podSecurityAdmission,omitempty"`

	// SetViaHelm indicates settings are from Helm (cannot be updated via API)
	SetViaHelm bool `json:"setViaHelm,omitempty"`

	Metadata Metadata `json:"metadata,omitempty"`
}

// PodSecurityAdmissionSettings contains Pod Security Admission configuration for the MCP namespace.
// These settings control how Kubernetes Pod Security Standards are enforced on MCP server pods.
// See https://kubernetes.io/docs/concepts/security/pod-security-standards/ for more details.
type PodSecurityAdmissionSettings struct {
	// Enabled indicates whether PSA labels should be applied to the MCP namespace.
	// When enabled, security contexts on MCP server pods will be configured based on the Enforce level.
	Enabled bool `json:"enabled,omitempty"`

	// Enforce is the Pod Security Standards level to enforce. Pods that violate this level will be rejected.
	// Valid values: "privileged" (no restrictions), "baseline" (minimal restrictions), "restricted" (heavily restricted).
	// This also controls the security context applied to MCP server containers.
	Enforce string `json:"enforce,omitempty"`

	// EnforceVersion is the Kubernetes version for the enforce policy (e.g., "latest", "v1.28").
	// Defaults to "latest" if not specified.
	EnforceVersion string `json:"enforceVersion,omitempty"`

	// Audit is the Pod Security Standards level to audit. Violations are recorded in the audit log but not rejected.
	// Valid values: "privileged", "baseline", "restricted".
	Audit string `json:"audit,omitempty"`

	// AuditVersion is the Kubernetes version for the audit policy (e.g., "latest", "v1.28").
	// Defaults to "latest" if not specified.
	AuditVersion string `json:"auditVersion,omitempty"`

	// Warn is the Pod Security Standards level to warn about. Violations trigger a user-facing warning but are not rejected.
	// Valid values: "privileged", "baseline", "restricted".
	Warn string `json:"warn,omitempty"`

	// WarnVersion is the Kubernetes version for the warn policy (e.g., "latest", "v1.28").
	// Defaults to "latest" if not specified.
	WarnVersion string `json:"warnVersion,omitempty"`
}
