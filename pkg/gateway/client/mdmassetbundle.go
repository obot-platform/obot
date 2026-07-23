package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm/clause"
)

// StoreMDMAssetBundle stores an immutable source archive under its SHA-256
// digest. Re-storing the same archive repairs the stored content.
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

// GetMDMAssetBundle returns and verifies a source archive by digest.
func (c *Client) GetMDMAssetBundle(ctx context.Context, digest string) (*types.MDMAssetBundle, error) {
	var bundle types.MDMAssetBundle
	if err := c.db.WithContext(ctx).Where("digest = ?", digest).First(&bundle).Error; err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bundle.Content)
	if hex.EncodeToString(sum[:]) != bundle.Digest {
		return nil, fmt.Errorf("MDM asset bundle %.12s failed its content digest check", bundle.Digest)
	}
	return &bundle, nil
}

// PruneUnusedMDMAssetBundles removes source archives absent from retainDigests.
func (c *Client) PruneUnusedMDMAssetBundles(ctx context.Context, retainDigests ...string) error {
	retained := make([]string, 0, len(retainDigests))
	seen := map[string]struct{}{}
	for _, digest := range retainDigests {
		if digest == "" {
			continue
		}
		if _, ok := seen[digest]; ok {
			continue
		}
		seen[digest] = struct{}{}
		retained = append(retained, digest)
	}

	query := c.db.WithContext(ctx)
	if len(retained) > 0 {
		query = query.Where("digest NOT IN ?", retained)
	} else {
		// GORM blocks a DELETE with no WHERE clause. With nothing to retain,
		// "1 = 1" intentionally prunes every bundle.
		query = query.Where("1 = 1")
	}
	if err := query.Delete(&types.MDMAssetBundle{}).Error; err != nil {
		return fmt.Errorf("failed to prune unused MDM asset bundles: %w", err)
	}
	return nil
}
