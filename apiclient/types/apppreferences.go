package types

// AppPreferences represents global application appearance preferences
type AppPreferences struct {
	Logos    LogoPreferences     `json:"logos,omitempty"`
	Theme    ThemePreferences    `json:"theme,omitempty"`
	Branding BrandingPreferences `json:"branding,omitempty"`
	Metadata Metadata            `json:"metadata,omitempty"`
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
	BackgroundColor     string `json:"backgroundColor,omitempty"`
	OnBackgroundColor   string `json:"onBackgroundColor,omitempty"`
	OnSurfaceColor      string `json:"onSurfaceColor,omitempty"`
	Surface1Color       string `json:"surface1Color,omitempty"`
	Surface2Color       string `json:"surface2Color,omitempty"`
	Surface3Color       string `json:"surface3Color,omitempty"`
	PrimaryColor        string `json:"primaryColor,omitempty"`
	DarkBackgroundColor     string `json:"darkBackgroundColor,omitempty"`
	DarkOnBackgroundColor   string `json:"darkOnBackgroundColor,omitempty"`
	DarkOnSurfaceColor      string `json:"darkOnSurfaceColor,omitempty"`
	DarkSurface1Color       string `json:"darkSurface1Color,omitempty"`
	DarkSurface2Color       string `json:"darkSurface2Color,omitempty"`
	DarkSurface3Color       string `json:"darkSurface3Color,omitempty"`
	DarkPrimaryColor string `json:"darkPrimaryColor,omitempty"`
}

// BrandingPreferences represents configurable branding for the application footer
type BrandingPreferences struct {
	ProductName    string `json:"productName,omitempty"`
	IssueReportURL string `json:"issueReportUrl,omitempty"`
	FooterMessage  string `json:"footerMessage,omitempty"`
	ShowFooter     *bool  `json:"showFooter,omitempty"`
}

