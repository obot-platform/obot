package v1

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*OAuthApp)(nil)
	_ fields.Fields = (*OAuthAppLogin)(nil)
	_ Aliasable     = (*OAuthApp)(nil)
	_ Generationed  = (*OAuthApp)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthAppSpec `json:"spec,omitempty"`
	Status            EmptyStatus  `json:"status,omitempty"`
}

func (r *OAuthApp) GetAliasName() string {
	return r.Spec.Manifest.Integration
}

func (r *OAuthApp) SetAssigned(bool) {}

func (r *OAuthApp) IsAssigned() bool {
	return true
}

func (r *OAuthApp) GetObservedGeneration() int64 {
	return r.Generation
}

func (r *OAuthApp) SetObservedGeneration(int64) {}

func (r *OAuthApp) Has(field string) bool {
	return r.Get(field) != ""
}

func (r *OAuthApp) Get(field string) string {
	if r != nil {
		switch field {
		case "spec.manifest.integration":
			return r.Spec.Manifest.Integration
		}
	}

	return ""
}

func (r *OAuthApp) FieldNames() []string {
	return []string{"spec.manifest.integration"}
}

func (r *OAuthApp) RedirectURL(baseURL string) string {
	return fmt.Sprintf("%s/api/app-oauth/callback/%s", baseURL, r.Spec.Manifest.Integration)
}

func OAuthAppGetTokenURL(baseURL string) string {
	return fmt.Sprintf("%s/api/app-oauth/get-token", baseURL)
}

func (r *OAuthApp) AuthorizeURL(baseURL string) string {
	return fmt.Sprintf("%s/api/app-oauth/authorize/%s", baseURL, r.Spec.Manifest.Integration)
}

func (r *OAuthApp) RefreshURL(baseURL string) string {
	return fmt.Sprintf("%s/api/app-oauth/refresh/%s", baseURL, r.Spec.Manifest.Integration)
}

func (r *OAuthApp) DeleteRefs() []Ref {
	return nil
}

type OAuthAppSpec struct {
	Manifest types.OAuthAppManifest `json:"manifest,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthApp `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppLogin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthAppLoginSpec   `json:"spec,omitempty"`
	Status            OAuthAppLoginStatus `json:"status,omitempty"`
}

func (o *OAuthAppLogin) Has(field string) bool {
	return o.Get(field) != ""
}

func (o *OAuthAppLogin) Get(field string) string {
	if o != nil {
		switch field {
		case "spec.credentialContext":
			return o.Spec.CredentialContext
		}
	}
	return ""
}

func (o *OAuthAppLogin) FieldNames() []string {
	return []string{"spec.credentialContext"}
}

func (o *OAuthAppLogin) DeleteRefs() []Ref {
	if system.IsAgentID(o.Spec.CredentialContext) {
		return []Ref{{ObjType: new(Agent), Name: o.Spec.CredentialContext}}
	} else if system.IsWorkflowID(o.Spec.CredentialContext) {
		return []Ref{{ObjType: new(Workflow), Name: o.Spec.CredentialContext}}
	}
	return nil
}

type OAuthAppLoginSpec struct {
	CredentialContext string   `json:"credentialContext,omitempty"`
	ToolReference     string   `json:"toolReference,omitempty"`
	OAuthApps         []string `json:"oauthApps,omitempty"`
}

type OAuthAppLoginStatus struct {
	External types.OAuthAppLoginAuthStatus `json:"external,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppLoginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthAppLogin `json:"items"`
}
