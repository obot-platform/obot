package client

import (
	"context"
	"errors"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// RunOnce runs f the first time it is called for name on this installation, and
// never again.
//
// Seeding cannot ask "is this a new installation" and get a useful answer. The
// existing seeds infer it from another resource being absent, which conflates
// two different things: an installation that has never run, and one that has
// run for a year and is meeting this seed for the first time because it just
// upgraded. An upgrade looks like neither a new installation nor a seeded one,
// so seeds gated that way silently never run there.
//
// The name is that seed's own record, so adding one later is a new name rather
// than a change to what an existing gate means. It is also what separates
// "never seeded" from "seeded, and the administrator deleted it" -- the record
// outlives the resource, so a deletion stays deleted.
//
// f and the record are written in one transaction: a seed that fails leaves no
// record and is retried on the next start, and a record that fails to write
// rolls the seed back with it.
func (c *Client) RunOnce(ctx context.Context, name string, f func(context.Context) error) error {
	db := c.db.WithContext(ctx)

	var record gatewaytypes.Migration
	if err := db.Where("name = ?", name).First(&record).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := f(ctx); err != nil {
			return err
		}
		return tx.Create(&gatewaytypes.Migration{Name: name}).Error
	})
}
