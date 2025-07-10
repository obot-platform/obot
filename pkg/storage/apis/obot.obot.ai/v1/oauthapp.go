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
	if r.Spec.ThreadName == "" {
		// Only non-project scoped oauth can have a true global alias
		return r.Spec.Manifest.Alias
	}
	return ""
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
		case "spec.manifest.alias":
			return r.Spec.Manifest.Alias
		case "spec.threadName":
			return r.Spec.ThreadName
		case "spec.slackReceiverName":
			return r.Spec.SlackReceiverName
		case "spec.manifest.authorizationServerURL":
			return r.Spec.Manifest.AuthorizationServerURL
		}
	}

	return ""
}

func (r *OAuthApp) FieldNames() []string {
	return []string{"spec.manifest.alias", "spec.threadName", "spec.slackReceiverName", "spec.manifest.authorizationServerURL"}
}

func (r *OAuthApp) RedirectURL(baseURL string) string {
	name := r.Name
	if r.Spec.ThreadName == "" {
		name = r.Spec.Manifest.Alias
	}
	return fmt.Sprintf("%s/api/app-oauth/callback/%s", baseURL, name)
}

func (r *OAuthApp) OAuthAppGetTokenURL(baseURL string) string {
	return fmt.Sprintf("%s/api/app-oauth/get-token/%s", baseURL, r.Name)
}

func (r *OAuthApp) AuthorizeURL(baseURL string) string {
	return fmt.Sprintf("%s/api/app-oauth/authorize/%s", baseURL, r.Name)
}

func (r *OAuthApp) RefreshURL(baseURL string) string {
	return fmt.Sprintf("%s/api/app-oauth/refresh/%s", baseURL, r.Name)
}

func (r *OAuthApp) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(Thread), Name: r.Spec.ThreadName},
		{ObjType: new(SlackReceiver), Name: r.Spec.SlackReceiverName},
	}
}

type OAuthAppSpec struct {
	Manifest types.OAuthAppManifest `json:"manifest,omitempty"`
	// The project that owns this OAuth app
	ThreadName string `json:"threadName,omitempty"`
	// The Slack receiver that created and owns this OAuth app
	SlackReceiverName string `json:"slackReceiverName,omitempty"`
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
