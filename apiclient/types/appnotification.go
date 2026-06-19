package types

// BannerType represents the visual style of a notification banner
type BannerType string

const (
	BannerTypeInfo    BannerType = "info"
	BannerTypeWarning BannerType = "warning"
)

// AppNotification represents a global notification
type AppNotification struct {
	Banner  BannerNotification `json:"banner,omitempty"`
	Updated *Time              `json:"updated,omitempty"`
}

type BannerNotification struct {
	Dismissible bool       `json:"dismissible,omitempty"`
	Type        BannerType `json:"type,omitempty"`
	Enabled     bool       `json:"enabled,omitempty"`
	Text        string     `json:"text,omitempty"`
	// ResetDismissed indicates that previously dismissed banners should be shown again.
	ResetDismissed bool `json:"resetDismissed,omitempty"`
}
