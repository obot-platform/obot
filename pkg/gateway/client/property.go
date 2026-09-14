package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var (
	propertyGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "properties",
	}
)

func (c *Client) GetProperty(ctx context.Context, key string) (types.Property, error) {
	var p types.Property
	if err := c.db.WithContext(ctx).Where("key = ?", key).First(&p).Error; err != nil {
		return p, err
	}
	return p, c.decryptProperty(ctx, &p)
}

// PropertyVersionMatchesTx locks a property's database row and reports whether
// its version still matches an earlier read. A nil version expects the row to
// be absent. It intentionally avoids adding decryption and its potential KMS
// I/O while callers coordinate changes across multiple properties.
func (c *Client) PropertyVersionMatchesTx(tx *gorm.DB, key string, expectedVersion *time.Time) (bool, error) {
	var property types.Property
	err := tx.Select("key", "updated_at").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("key = ?", key).
		First(&property).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return expectedVersion == nil, nil
	}
	if err != nil {
		return false, err
	}
	return expectedVersion != nil && property.UpdatedAt.Equal(*expectedVersion), nil
}

func (c *Client) SetProperty(ctx context.Context, key, value string) (types.Property, error) {
	var property types.Property
	err := c.Transaction(ctx, func(tx *gorm.DB) error {
		var err error
		property, err = c.SetPropertyTx(ctx, tx, key, value)
		return err
	})
	return property, err
}

// SetPropertyTx sets a property using an existing transaction. This lets
// callers update multiple related properties without exposing partial state.
func (c *Client) SetPropertyTx(ctx context.Context, tx *gorm.DB, key, value string) (types.Property, error) {
	var property types.Property
	if err := tx.Where("key = ?", key).First(&property).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return property, err
		}
		now := time.Now()
		property = types.Property{
			Key:       key,
			Value:     value,
			CreatedAt: now,
			UpdatedAt: now,
		}
		toStore := property
		if err := c.encryptProperty(ctx, &toStore); err != nil {
			return property, err
		}
		return property, tx.Create(&toStore).Error
	}

	property.Value = value
	property.Encrypted = false
	property.UpdatedAt = time.Now()
	toStore := property
	if err := c.encryptProperty(ctx, &toStore); err != nil {
		return property, err
	}
	return property, tx.Save(&toStore).Error
}

func (c *Client) GetOrCreateProperty(ctx context.Context, key, value string) (types.Property, error) {
	now := time.Now()
	var p types.Property
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("key = ?", key).First(&p).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				p = types.Property{
					Key:       key,
					Value:     value,
					CreatedAt: now,
					UpdatedAt: now,
				}
				toStore := p
				if err := c.encryptProperty(ctx, &toStore); err != nil {
					return err
				}
				return tx.Create(&toStore).Error
			}
			return err
		}
		return c.decryptProperty(ctx, &p)
	})
	return p, err
}

func (c *Client) DeleteProperty(ctx context.Context, key string) error {
	return c.DeletePropertyTx(c.db.WithContext(ctx), key)
}

// DeletePropertyTx deletes a property using an existing transaction. This lets
// callers combine the deletion with changes to other related properties.
func (c *Client) DeletePropertyTx(tx *gorm.DB, key string) error {
	return tx.Where("key = ?", key).Delete(&types.Property{}).Error
}

func (c *Client) encryptProperty(ctx context.Context, property *types.Property) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[propertyGroupResource]
	if transformer == nil {
		return nil
	}

	b, err := transformer.TransformToStorage(ctx, []byte(property.Value), propertyDataCtx(property))
	if err != nil {
		return err
	}

	property.Value = base64.StdEncoding.EncodeToString(b)
	property.Encrypted = true
	return nil
}

func (c *Client) decryptProperty(ctx context.Context, property *types.Property) error {
	if !property.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[propertyGroupResource]
	if transformer == nil {
		return nil
	}

	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(property.Value)))
	n, err := base64.StdEncoding.Decode(decoded, []byte(property.Value))
	if err != nil {
		return err
	}

	out, _, err := transformer.TransformFromStorage(ctx, decoded[:n], propertyDataCtx(property))
	if err != nil {
		return err
	}

	property.Value = string(out)
	return nil
}

func propertyDataCtx(property *types.Property) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", propertyGroupResource.String(), property.Key))
}
