package client

import (
	"context"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// DeleteHostedAgentRecords runs only after the legacy runtime has been removed.
// These kinm kinds no longer have API endpoints, so drop their tables directly.
func (c *Client) DeleteHostedAgentRecords(ctx context.Context) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{
			"hostedagentinstance", "hostedagentpoolassignment", "hostedagentpooldefaults",
			"hostedagentpool", "hostedagentaccessrule", "hostedagent", "agentcatalog", "harness",
		} {
			if err := tx.Migrator().DropTable(table); err != nil {
				return err
			}
			if tx.Migrator().HasTable("compaction") {
				if err := tx.Table("compaction").Where("name = ?", table).Delete(nil).Error; err != nil {
					return err
				}
			}
		}
		return tx.Where("context = ?", "hosted-agent-credential").Delete(&types.Credential{}).Error
	})
}
