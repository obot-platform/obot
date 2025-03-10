package types

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GeneratedImage struct {
	ID        string    `json:"id" gorm:"primaryKey;autoIncrement:false"`
	CreatedAt time.Time `json:"createdAt"`
	Data      []byte    `json:"-" gorm:"type:bytea;not null"`
	MIMEType  string    `json:"mimeType" gorm:"type:varchar(100);not null"`
}

// BeforeCreate will set the ID to a UUID v4.
func (g *GeneratedImage) BeforeCreate(_ *gorm.DB) error {
	g.ID = uuid.NewString()
	return nil
}
