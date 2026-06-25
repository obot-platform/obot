package types

// ObotK8sSettings surfaces Helm-managed Obot server Kubernetes configuration.
// Values are read-only and sourced from the Helm values snapshot captured at install/upgrade time.
// Fields mirror chart/values.yaml; complex values are returned as YAML strings.
type ObotK8sSettings struct {
	// Available is false when Obot is not running on the Kubernetes backend.
	Available bool `json:"available"`

	ReplicaCount     *int32 `json:"replicaCount,omitempty"`
	UpdateStrategy   string `json:"updateStrategy,omitempty"`
	RuntimeClassName string `json:"runtimeClassName,omitempty"`

	Dev               string `json:"dev,omitempty"`
	Image             string `json:"image,omitempty"`
	ImagePullSecrets  string `json:"imagePullSecrets,omitempty"`
	AdditionalLabels  string `json:"additionalLabels,omitempty"`
	PodAnnotations    string `json:"podAnnotations,omitempty"`
	Service           string `json:"service,omitempty"`
	Ingress           string `json:"ingress,omitempty"`
	Config            string `json:"config,omitempty"`
	Resources         string `json:"resources,omitempty"`
	Persistence       string `json:"persistence,omitempty"`
	ExtraVolumes      string `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts string `json:"extraVolumeMounts,omitempty"`
	ServiceAccount    string `json:"serviceAccount,omitempty"`
	MCPNamespace      string `json:"mcpNamespace,omitempty"`
	NodeSelector      string `json:"nodeSelector,omitempty"`
	Tolerations       string `json:"tolerations,omitempty"`
	Affinity          string `json:"affinity,omitempty"`
}
