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
	ClientID             string      `json:"clientID"`
	Scope                string      `json:"scope"`
	ExpiresAt            metav1.Time `json:"expiresAt"`
	ProviderRefreshToken string      `json:"providerRefreshToken"`
	ProviderAccessToken  string      `json:"providerAccessToken"`
}

type OAuthTokenStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthToken `json:"items"`
}
