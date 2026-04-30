package types

// AppPreferences represents global application appearance preferences
type AppPreferences struct {
	Logos    LogoPreferences  `json:"logos,omitempty"`
	Theme    ThemePreferences `json:"theme,omitempty"`
	Metadata Metadata         `json:"metadata,omitempty"`
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
	BackgroundColor   string `json:"backgroundColor,omitempty"`
	OnBackgroundColor string `json:"onBackgroundColor,omitempty"`
	Surface1Color     string `json:"surface1Color,omitempty"`
	Surface2Color     string `json:"surface2Color,omitempty"`
	Surface3Color     string `json:"surface3Color,omitempty"`
	SecondaryColor    string `json:"secondaryColor,omitempty"`
	SuccessColor      string `json:"successColor,omitempty"`
	WarningColor      string `json:"warningColor,omitempty"`
	ErrorColor        string `json:"errorColor,omitempty"`
	PrimaryColor      string `json:"primaryColor,omitempty"`
	OnPrimaryColor    string `json:"onPrimaryColor,omitempty"`
	OnSuccessColor    string `json:"onSuccessColor,omitempty"`
	OnWarningColor    string `json:"onWarningColor,omitempty"`
	OnErrorColor      string `json:"onErrorColor,omitempty"`
	FontFamily        string `json:"fontFamily,omitempty"`

	DarkBackgroundColor   string `json:"darkBackgroundColor,omitempty"`
	DarkOnBackgroundColor string `json:"darkOnBackgroundColor,omitempty"`
	DarkSurface1Color     string `json:"darkSurface1Color,omitempty"`
	DarkSurface2Color     string `json:"darkSurface2Color,omitempty"`
	DarkSurface3Color     string `json:"darkSurface3Color,omitempty"`
	DarkSecondaryColor    string `json:"darkSecondaryColor,omitempty"`
	DarkSuccessColor      string `json:"darkSuccessColor,omitempty"`
	DarkWarningColor      string `json:"darkWarningColor,omitempty"`
	DarkErrorColor        string `json:"darkErrorColor,omitempty"`
	DarkPrimaryColor      string `json:"darkPrimaryColor,omitempty"`
	DarkOnPrimaryColor    string `json:"darkOnPrimaryColor,omitempty"`
	DarkOnSuccessColor    string `json:"darkOnSuccessColor,omitempty"`
	DarkOnWarningColor    string `json:"darkOnWarningColor,omitempty"`
	DarkOnErrorColor      string `json:"darkOnErrorColor,omitempty"`
}
