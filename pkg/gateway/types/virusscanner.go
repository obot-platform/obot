package types

import "time"

type FileScannerConfig struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	UpdatedAt         time.Time `json:"updatedAt"`
	ProviderName      string    `json:"providerName"`
	ProviderNamespace string    `json:"providerNamespace"`
}
