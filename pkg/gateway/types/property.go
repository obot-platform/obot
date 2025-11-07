//nolint:revive
package types

import "time"

type Property struct {
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Key       string    `json:"key" gorm:"primaryKey"`
	Value     string    `json:"value"`
}
