package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var propertyGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "properties",
}

func (c *Client) GetProperty(ctx context.Context, key string) (types.Property, error) {
	var p types.Property
	if err := c.db.WithContext(ctx).Where("key = ?", key).First(&p).Error; err != nil {
		return p, err
	}
	return p, c.decryptProperty(ctx, &p)
}

func (c *Client) SetProperty(ctx context.Context, key, value string) (types.Property, error) {
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
		p.Value = value
		p.Encrypted = false
		p.UpdatedAt = time.Now()
		toStore := p
		if err := c.encryptProperty(ctx, &toStore); err != nil {
			return err
		}
		return tx.Save(&toStore).Error
	})
	return p, err
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
	return c.db.WithContext(ctx).Where("key = ?", key).Delete(&types.Property{}).Error
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
