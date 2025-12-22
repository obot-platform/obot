package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

var _ DeleteRefs = (*OAuthToken)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthTokenSpec   `json:"spec"`
	Status            OAuthTokenStatus `json:"status"`
}

func (in *OAuthToken) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(OAuthClient), Name: in.Spec.ClientID},
	}
}

type OAuthTokenSpec struct {
	Scope                 string `json:"scope"`
	Resource              string `json:"resource"`
	ClientID              string `json:"clientID"`
	UserID                uint   `json:"userID"`
	MCPID                 string `json:"mcpID"`
	AuthProviderUserID    string `json:"authProviderUserID"`
	AuthProviderName      string `json:"authProviderName"`
	AuthProviderNamespace string `json:"authProviderNamespace"`
}

type OAuthTokenStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthToken `json:"items"`
}
