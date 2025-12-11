package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AppPreferences struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppPreferencesSpec   `json:"spec,omitempty"`
	Status AppPreferencesStatus `json:"status,omitempty"`
}

type AppPreferencesSpec struct {
	Logos    types.LogoPreferences     `json:"logos,omitempty"`
	Theme    types.ThemePreferences    `json:"theme,omitempty"`
	Branding types.BrandingPreferences `json:"branding,omitempty"`
}

type AppPreferencesStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AppPreferencesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AppPreferences `json:"items"`
}
