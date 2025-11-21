package v1

import (
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
	// Logo preferences
	Logos LogoPreferences `json:"logos,omitempty"`

	// Theme preferences
	Theme ThemePreferences `json:"theme,omitempty"`
}

type LogoPreferences struct {
	LogoIcon           string `json:"logoIcon,omitempty"`
	LogoIconError      string `json:"logoIconError,omitempty"`
	LogoIconWarning    string `json:"logoIconWarning,omitempty"`
	LogoDefault        string `json:"logoDefault,omitempty"`
	LogoEnterprise     string `json:"logoEnterprise,omitempty"`
	LogoChat           string `json:"logoChat,omitempty"`
	DarkLogoDefault    string `json:"darkLogoDefault,omitempty"`
	DarkLogoChat       string `json:"darkLogoChat,omitempty"`
	DarkLogoEnterprise string `json:"darkLogoEnterprise,omitempty"`
}

type ThemePreferences struct {
	// Light theme colors
	BackgroundColor string `json:"backgroundColor,omitempty"`
	OnSurfaceColor  string `json:"onSurfaceColor,omitempty"`
	Surface1Color   string `json:"surface1Color,omitempty"`
	Surface2Color   string `json:"surface2Color,omitempty"`
	Surface3Color   string `json:"surface3Color,omitempty"`
	PrimaryColor    string `json:"primaryColor,omitempty"`

	// Dark theme colors
	DarkBackgroundColor string `json:"darkBackgroundColor,omitempty"`
	DarkOnSurfaceColor  string `json:"darkOnSurfaceColor,omitempty"`
	DarkSurface1Color   string `json:"darkSurface1Color,omitempty"`
	DarkSurface2Color   string `json:"darkSurface2Color,omitempty"`
	DarkSurface3Color   string `json:"darkSurface3Color,omitempty"`
	DarkPrimaryColor    string `json:"darkPrimaryColor,omitempty"`
}

type AppPreferencesStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AppPreferencesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AppPreferences `json:"items"`
}
