package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ DeleteRefs = (*OAuthAuthRequest)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAuthRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthAuthRequestSpec   `json:"spec"`
	Status            OAuthAuthRequestStatus `json:"status"`
}

func (o *OAuthAuthRequest) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(OAuthClient), Name: o.Spec.ClientID},
	}
}

type OAuthAuthRequestSpec struct {
	RedirectURI          string      `json:"redirectURI"`
	ClientID             string      `json:"clientID"`
	Code                 string      `json:"code"`
	CodeChallenge        string      `json:"codeChallenge"`
	CodeChallengeMethod  string      `json:"codeChallengeMethod"`
	GrantType            string      `json:"grantType"`
	Scope                string      `json:"scope"`
	ExpiresAt            metav1.Time `json:"expiresAt"`
	ProviderAccessToken  string      `json:"providerAccessToken"`
	ProviderRefreshToken string      `json:"providerRefreshToken"`
}

type OAuthAuthRequestStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAuthRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthAuthRequest `json:"items"`
}
