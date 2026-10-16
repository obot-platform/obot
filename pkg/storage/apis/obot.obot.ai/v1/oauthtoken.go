package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ DeleteRefs = (*OAuthToken)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              OAuthTokenSpec   `json:"spec"`
	Status            OAuthTokenStatus `json:"status"`
}

type OAuthTokenSpec struct {
	Scope                 string `json:"scope"`
	Resource              string `json:"resource"`
	ClientID              string `json:"clientID"`
	UserID                uint   `json:"userID"`
	MCPID                 string `json:"mcpID"`
	Audience              string `json:"audience"`
	AuthProviderUserID    string `json:"authProviderUserID"`
	AuthProviderName      string `json:"authProviderName"`
	AuthProviderNamespace string `json:"authProviderNamespace"`

	// The record survives rotation so a lost response can be recovered with the previous token.
	CurrentTokenHash  string `json:"currentTokenHash,omitempty"`
	PreviousTokenHash string `json:"previousTokenHash,omitempty"`
	// +optional
	PreviousValidUntil    metav1.Time `json:"previousValidUntil,omitzero"`
	EncryptedCurrentToken string      `json:"encryptedCurrentToken,omitempty"`
}

type OAuthTokenStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []OAuthToken `json:"items"`
}

func (in *OAuthToken) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(OAuthClient), Name: in.Spec.ClientID},
	}
}
