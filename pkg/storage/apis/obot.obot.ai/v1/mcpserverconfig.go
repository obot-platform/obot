package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MCPServerConfigSpec `json:"spec"`
	Status            MCPServerConfigStatus
}

type MCPServerConfigSpec struct {
	AccessTokens map[string]AccessToken `json:"accessTokens,omitempty"`
	EnvVars      map[string]string      `json:"envVars,omitempty"`
}

type AccessToken struct {
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	ExpiresAt    metav1.Time `json:"expiresAt"`
}

type MCPServerConfigStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServerConfig `json:"items"`
}
