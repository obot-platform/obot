package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StoreMDMAssetBundle stores validated canonical archive content under its
// SHA-256 digest. Re-storing the same digest repairs the private blob row while
// preserving the immutable public identity.
func (c *Client) StoreMDMAssetBundle(ctx context.Context, content []byte) (string, error) {
	if len(content) == 0 {
		return "", fmt.Errorf("MDM asset bundle content is empty")
	}
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	bundle := &types.MDMAssetBundle{Digest: digest, Content: content}
	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "digest"}},
		DoUpdates: clause.AssignmentColumns([]string{"content"}),
	}).Create(bundle).Error; err != nil {
		return "", fmt.Errorf("failed to store MDM asset bundle: %w", err)
	}
	return digest, nil
}

// GetMDMAssetBundle returns the private archive bytes identified by digest.
func (c *Client) GetMDMAssetBundle(ctx context.Context, digest string) (*types.MDMAssetBundle, error) {
	var bundle types.MDMAssetBundle
	if err := c.db.WithContext(ctx).Where("digest = ?", digest).First(&bundle).Error; err != nil {
		return nil, err
	}
	return &bundle, nil
}

// PruneUnusedMDMAssetBundles removes private blobs that are neither the
// source's latest digest nor referenced by an MDM configuration. The
// controller calls this once at startup after pruning matching metadata.
func (c *Client) PruneUnusedMDMAssetBundles(ctx context.Context, latestDigest string) error {
	referenced := c.db.WithContext(ctx).Model(&types.MDMConfiguration{}).
		Select("asset_digest").
		Where("asset_digest IS NOT NULL AND asset_digest <> ?", "")
	query := c.db.WithContext(ctx).Where("digest NOT IN (?)", referenced)
	if latestDigest != "" {
		query = query.Where("digest <> ?", latestDigest)
	}
	if err := query.Delete(&types.MDMAssetBundle{}).Error; err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to prune unused MDM asset bundles: %w", err)
	}
	return nil
}
