package db

import (
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// The activity tables as they were before rows were attributed by principal: only a user_id.

type llmAuditLogBeforePrincipals struct {
	ID        string `gorm:"primaryKey;type:text"`
	CreatedAt time.Time
	UserID    string `gorm:"type:text"`
}

type mcpAuditLogBeforePrincipals struct {
	ID            uint `gorm:"primaryKey"`
	CreatedAt     time.Time
	SourceType    string `gorm:"default:mcp"`
	UserID        string
	ClientName    string
	ClientVersion string
}

type runTokenActivityBeforePrincipals struct {
	ID           uint
	CreatedAt    time.Time
	UserID       string
	InputTokens  int
	OutputTokens int
}

func (llmAuditLogBeforePrincipals) TableName() string { return "llm_audit_logs" }

func (mcpAuditLogBeforePrincipals) TableName() string { return "mcp_audit_logs" }

func (runTokenActivityBeforePrincipals) TableName() string { return "run_token_activities" }

func TestActivityPrincipalColumnsMigrationOnSQLite(t *testing.T) {
	database, gormDB := newSQLiteTestDatabase(t)
	testActivityPrincipalColumnsMigration(t, database, gormDB)
}

func TestActivityPrincipalColumnsMigrationOnPostgres(t *testing.T) {
	database, gormDB := newPostgresTestDatabase(t, "obot_activity_principals_")
	testActivityPrincipalColumnsMigration(t, database, gormDB)
}

// testActivityPrincipalColumnsMigration migrates activity tables of the release before principals. Existing rows of
// users keep reading as users through the principal_kind default, without being rewritten.
func testActivityPrincipalColumnsMigration(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	if err := gormDB.AutoMigrate(&llmAuditLogBeforePrincipals{}, &mcpAuditLogBeforePrincipals{}, &runTokenActivityBeforePrincipals{}); err != nil {
		t.Fatalf("failed to create the previous tables: %v", err)
	}
	now := time.Now().UTC()
	for _, record := range []any{
		&llmAuditLogBeforePrincipals{ID: "user-row", CreatedAt: now, UserID: "7"},
		&mcpAuditLogBeforePrincipals{CreatedAt: now, UserID: "7"},
		&runTokenActivityBeforePrincipals{CreatedAt: now, UserID: "7", InputTokens: 3},
	} {
		if err := gormDB.Create(record).Error; err != nil {
			t.Fatalf("failed to create %T: %v", record, err)
		}
	}

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	// Migrating again finds nothing to change.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate again: %v", err)
	}

	migrator := gormDB.Migrator()
	for model, index := range map[any]string{
		&types.LLMAuditLog{}:      "idx_llm_audit_principal",
		&types.MCPAuditLog{}:      "idx_mcp_audit_principal",
		&types.RunTokenActivity{}: "idx_run_token_activity_principal",
	} {
		if !migrator.HasIndex(model, index) {
			t.Errorf("%T has no %s index", model, index)
		}
	}

	type attribution struct {
		kind, id, userID string
	}
	for name, test := range map[string]struct {
		model any
		// userWhere selects the table's previous row of the user.
		userWhere string
		wantUser  attribution
	}{
		"llm audit logs": {
			model:     &types.LLMAuditLog{},
			userWhere: "id = 'user-row'",
			wantUser:  attribution{kind: "user", id: "", userID: "7"},
		},
		"mcp audit logs": {
			model:     &types.MCPAuditLog{},
			userWhere: "user_id = '7'",
			wantUser:  attribution{kind: "user", id: "", userID: "7"},
		},
		"run token activities": {
			model:     &types.RunTokenActivity{},
			userWhere: "input_tokens = 3",
			wantUser:  attribution{kind: "user", id: "", userID: "7"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			read := func(where string) (got attribution) {
				t.Helper()
				var row struct {
					PrincipalKind string
					PrincipalID   string
					UserID        string
				}
				if err := gormDB.Model(test.model).Where(where).Take(&row).Error; err != nil {
					t.Fatalf("failed to read the row where %s: %v", where, err)
				}
				return attribution{kind: row.PrincipalKind, id: row.PrincipalID, userID: row.UserID}
			}
			if got := read(test.userWhere); got != test.wantUser {
				t.Errorf("user row = %+v, want %+v", got, test.wantUser)
			}
		})
	}

	// A row of a user from before principals is read as the user's.
	if got, want := (types.Actor{PrincipalKind: "user", UserID: "7"}).Ref(), (apitypes.PrincipalRef{Kind: apitypes.PrincipalKindUser, ID: "7"}); got != want {
		t.Errorf("legacy user row principal = %+v, want %+v", got, want)
	}
}
