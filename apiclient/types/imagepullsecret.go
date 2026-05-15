package types

type ImagePullSecretType string

const (
	ImagePullSecretTypeBasic ImagePullSecretType = "basic"
	ImagePullSecretTypeECR   ImagePullSecretType = "ecr"
)

// ImagePullSecret represents an admin-managed Kubernetes image pull secret.
type ImagePullSecret struct {
	Metadata
	Manifest ImagePullSecretManifest `json:"manifest"`
	Status   ImagePullSecretStatus   `json:"status"`
}

type ImagePullSecretList List[ImagePullSecret]

type ImagePullSecretManifest struct {
	Enabled     bool                        `json:"enabled"`
	Type        ImagePullSecretType         `json:"type,omitempty"`
	DisplayName string                      `json:"displayName,omitempty"`
	Basic       *BasicImagePullSecretConfig `json:"basic,omitempty"`
	ECR         *ECRImagePullSecretConfig   `json:"ecr,omitempty"`
}

type BasicImagePullSecretConfig struct {
	Server   string `json:"server,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ECRImagePullSecretConfig struct {
	RoleARN         string `json:"roleARN,omitempty"`
	Region          string `json:"region,omitempty"`
	IssuerURL       string `json:"issuerURL,omitempty"`
	Audience        string `json:"audience,omitempty"`
	RefreshSchedule string `json:"refreshSchedule,omitempty"`
}

type ImagePullSecretStatus struct {
	PasswordConfigured bool     `json:"passwordConfigured,omitempty"`
	Subject            string   `json:"subject,omitempty"`
	TrustPolicyJSON    string   `json:"trustPolicyJSON,omitempty"`
	ECRPolicyJSON      string   `json:"ecrPolicyJSON,omitempty"`
	LastReconciledTime *Time    `json:"lastReconciledTime,omitempty"`
	LastSuccessTime    *Time    `json:"lastSuccessTime,omitempty"`
	LastError          string   `json:"lastError,omitempty"`
	TokenExpiresAt     *Time    `json:"tokenExpiresAt,omitempty"`
	RegistryEndpoints  []string `json:"registryEndpoints,omitempty"`
}

type ImagePullSecretCapability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	IssuerURL string `json:"issuerURL,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Audience  string `json:"audience,omitempty"`
}

type ImagePullSecretTestRequest struct {
	Image string `json:"image,omitempty"`
}

type ImagePullSecretTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type ImagePullSecretRefreshResponse struct {
	Message string `json:"message,omitempty"`
}
