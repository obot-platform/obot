package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ DeleteRefs    = (*OAuthAuthRequest)(nil)
	_ fields.Fields = (*OAuthAuthRequest)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAuthRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthAuthRequestSpec   `json:"spec"`
	Status            OAuthAuthRequestStatus `json:"status"`
}

func (in *OAuthAuthRequest) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *OAuthAuthRequest) Get(field string) string {
	if in != nil {
		switch field {
		case "hashedAuthCode":
			return in.Status.HashedAuthCode
		}
	}

	return ""
}

func (in *OAuthAuthRequest) FieldNames() []string {
	return []string{"hashedAuthCode"}
}

func (o *OAuthAuthRequest) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(OAuthClient), Name: o.Spec.ClientID},
	}
}

type OAuthAuthRequestSpec struct {
	RedirectURI         string `json:"redirectURI"`
	State               string `json:"state"`
	ClientID            string `json:"clientID"`
	CodeChallenge       string `json:"codeChallenge"`
	CodeChallengeMethod string `json:"codeChallengeMethod"`
	GrantType           string `json:"grantType"`
	Scope               string `json:"scope"`
}

type OAuthAuthRequestStatus struct {
	HashedAuthCode         string            `json:"hashedAuthCode"`
	ProviderTokenType      string            `json:"providerTokenType"`
	ProviderTokenCreatedAt metav1.Time       `json:"providerTokenCreatedAt"`
	ProviderAccessToken    string            `json:"providerAccessToken"`
	ProviderRefreshToken   string            `json:"providerRefreshToken"`
	ExpiresAt              metav1.Time       `json:"expiresAt"`
	Ok                     bool              `json:"ok"`
	Error                  string            `json:"error"`
	Scope                  string            `json:"scope"`
	Data                   map[string]string `json:"data"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAuthRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthAuthRequest `json:"items"`
}
