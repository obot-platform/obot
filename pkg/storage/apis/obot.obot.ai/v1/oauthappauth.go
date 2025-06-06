package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppAuth struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthAppAuthSpec   `json:"spec"`
	Status            OAuthAppAuthStatus `json:"status"`
}

type OAuthAppAuthSpec struct {
	AuthRequestName   string `json:"authRequestName"`
	OAuthAppNamespace string `json:"oauthAppNamespace"`
	OAuthAppName      string `json:"oauthAppName"`
}

type OAuthAppAuthStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppAuthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthAppAuth `json:"items"`
}
