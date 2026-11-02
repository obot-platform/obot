package llmengine

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testOptions(t *testing.T, opts Options) Options {
	t.Helper()
	if opts.Store != nil {
		return opts
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(opts.DataDir, "obot.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrations, err := gatewaydb.New(db, sqlDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	opts.Store = llmcoreclient.New(db)
	return opts
}
