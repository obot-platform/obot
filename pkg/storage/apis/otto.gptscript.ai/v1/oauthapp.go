package v1

import (
	"fmt"

	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/acorn-io/baaah/pkg/fields"
	"github.com/gptscript-ai/otto/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*OAuthApp)(nil)
	_ conditions.Conditions = (*OAuthAppReference)(nil)
	_ fields.Fields         = (*OAuthAppReference)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthAppSpec   `json:"spec,omitempty"`
	Status            OAuthAppStatus `json:"status,omitempty"`
}

func (r *OAuthApp) RedirectURL(baseURL string) string {
	name := r.Name
	if r.Status.RefNameAssigned {
		name = r.Spec.RefName
	}
	return fmt.Sprintf("%s/oauth-apps/%s/callback", baseURL, name)
}

func (r *OAuthApp) AuthorizeURL(baseURL string) string {
	name := r.Name
	if r.Status.RefNameAssigned {
		name = r.Spec.RefName
	}
	return fmt.Sprintf("%s/oauth-apps/%s/authorize", baseURL, name)
}

func (r *OAuthApp) RefreshURL(baseURL string) string {
	name := r.Name
	if r.Status.RefNameAssigned {
		name = r.Spec.RefName
	}
	return fmt.Sprintf("%s/oauth-apps/%s/refresh", baseURL, name)
}

func (r *OAuthApp) GetConditions() *[]metav1.Condition {
	return &r.Status.Conditions
}

func (r *OAuthApp) DeleteRefs() []Ref {
	return nil
}

type OAuthAppSpec struct {
	types.OAuthAppManifest `json:",inline"`
}

type OAuthAppStatus struct {
	Conditions                   []metav1.Condition `json:"conditions,omitempty"`
	types.OAuthAppExternalStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthApp `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthAppReferenceSpec   `json:"spec,omitempty"`
	Status            OAuthAppReferenceStatus `json:"status,omitempty"`
}

func (*OAuthAppReference) NamespaceScoped() bool {
	return false
}

func (r *OAuthAppReference) Has(field string) bool {
	return r.Get(field) != ""
}

func (r *OAuthAppReference) Get(field string) string {
	if r != nil {
		switch field {
		case "spec.appName":
			return r.Spec.AppName
		case "spec.appNamespace":
			return r.Spec.AppNamespace
		}
	}
	return ""
}

func (r *OAuthAppReference) FieldNames() []string {
	return []string{"spec.appName", "spec.appNamespace"}
}

func (r *OAuthAppReference) GetConditions() *[]metav1.Condition {
	return &r.Status.Conditions
}

type OAuthAppReferenceSpec struct {
	Custom       bool   `json:"custom,omitempty"`
	AppName      string `json:"appName,omitempty"`
	AppNamespace string `json:"appNamespace,omitempty"`
}

type OAuthAppReferenceStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthAppReferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthAppReference `json:"items"`
}
