package client

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/obot-platform/obot/pkg/gateway/types"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var credentialGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "credentials",
}

const (
	credentialEncryptedSecretsKey = "_obot_encrypted_env"
	credentialLockRetryInterval   = 25 * time.Millisecond
	credentialLockMaxConnections  = 5
	credentialLockMaxIdleConns    = 2
)

// AcquireCredentialLock serializes operations for one credential key. PostgreSQL
// uses a transaction-scoped advisory lock so independent Obot processes share the
// same lock; non-PostgreSQL databases use a context-aware process-local semaphore.
func (c *Client) AcquireCredentialLock(ctx context.Context, key string) (func(), error) {
	if key == "" {
		return nil, fmt.Errorf("credential lock key is required")
	}

	db := c.db.WithContext(ctx)
	if db.Name() == "postgres" {
		lockPool, err := c.postgresCredentialLockPool(db)
		if err != nil {
			return nil, err
		}
		return acquirePostgresCredentialLock(ctx, lockPool, key)
	}
	return c.acquireProcessCredentialLock(ctx, key)
}

func (c *Client) postgresCredentialLockPool(db *gorm.DB) (*sql.DB, error) {
	c.credentialLockPoolOnce.Do(func() {
		// Advisory-lock holders retain their connection for the protected operation.
		// A separate pool prevents those holders from starving normal database work.
		dialector, ok := db.Dialector.(*gormpostgres.Dialector)
		if !ok || dialector.Config == nil || dialector.DSN == "" {
			c.credentialLockPoolErr = fmt.Errorf("failed to determine PostgreSQL credential lock database configuration")
			return
		}
		config, err := pgx.ParseConfig(dialector.DSN)
		if err != nil {
			c.credentialLockPoolErr = fmt.Errorf("failed to parse credential lock database configuration: %w", err)
			return
		}
		c.credentialLockPool = stdlib.OpenDB(*config)
		c.credentialLockPool.SetMaxOpenConns(credentialLockMaxConnections)
		c.credentialLockPool.SetMaxIdleConns(credentialLockMaxIdleConns)
	})
	return c.credentialLockPool, c.credentialLockPoolErr
}

func acquirePostgresCredentialLock(ctx context.Context, sqlDB *sql.DB, key string) (func(), error) {
	lockID := credentialLockID(key)

	for {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to reserve credential lock connection: %w", err)
		}
		tx, err := conn.BeginTx(context.WithoutCancel(ctx), nil)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to begin credential lock transaction: %w", err)
		}
		var acquired bool
		if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1)", lockID).Scan(&acquired); err != nil {
			_ = tx.Rollback()
			_ = conn.Close()
			return nil, fmt.Errorf("failed to acquire credential lock: %w", err)
		}

		if acquired {
			var release sync.Once
			return func() {
				release.Do(func() {
					_ = tx.Rollback()
					_ = conn.Close()
				})
			}, nil
		}
		_ = tx.Rollback()
		_ = conn.Close()

		timer := time.NewTimer(credentialLockRetryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func credentialLockID(key string) int64 {
	digest := sha256.Sum256([]byte(key))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func (c *Client) acquireProcessCredentialLock(ctx context.Context, key string) (func(), error) {
	permitCandidate := make(chan struct{}, 1)
	permitCandidate <- struct{}{}
	permitValue, _ := c.credentialLocks.LoadOrStore(key, permitCandidate)
	permit := permitValue.(chan struct{})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-permit:
	}

	var release sync.Once
	return func() {
		release.Do(func() {
			permit <- struct{}{}
		})
	}, nil
}

type ListCredentialsOptions struct {
	CredentialContexts []string
	AllContexts        bool
}

// ListCredentials returns the credentials in the given context.
// If AllContexts is true, CredentialContexts is ignored and credentials from all contexts are returned.
// The secrets in the returned credentials are blanked out for security; use RevealCredential to get the secrets for a specific credential.
func (c *Client) ListCredentials(ctx context.Context, opts ListCredentialsOptions) ([]types.Credential, error) {
	var credentials []types.Credential
	if len(opts.CredentialContexts) == 0 && !opts.AllContexts {
		return credentials, nil
	}

	db := c.db.WithContext(ctx)
	if !opts.AllContexts {
		db = db.Where("context IN ?", opts.CredentialContexts)
	}

	if err := db.Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}

	for i := range credentials {
		if err := c.decryptCredential(ctx, &credentials[i]); err != nil {
			return nil, fmt.Errorf("failed to decrypt credential: %w", err)
		}
		credentials[i].Secrets = blankCredentialSecrets(credentials[i].Secrets)
	}

	return credentials, nil
}

type CredentialNotFoundError struct {
	Contexts []string
	Name     string
}

func (e CredentialNotFoundError) Unwrap() error {
	// This allows errors.Is(err, gorm.ErrRecordNotFound) to work for CredentialNotFoundError.
	return gorm.ErrRecordNotFound
}

func (e CredentialNotFoundError) Error() string {
	return fmt.Sprintf("credential not found: contexts=%v, name=%s", e.Contexts, e.Name)
}

// RevealCredential returns the first credential matching name in the ordered list of contexts.
func (c *Client) RevealCredential(ctx context.Context, contexts []string, name string) (types.Credential, error) {
	var credential types.Credential
	if len(contexts) == 0 {
		return credential, CredentialNotFoundError{Contexts: contexts, Name: name}
	}

	for _, credentialContext := range contexts {
		if err := c.db.WithContext(ctx).Where("context = ? AND name = ?", credentialContext, name).First(&credential).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return credential, err
		}
		if err := c.decryptCredential(ctx, &credential); err != nil {
			return credential, fmt.Errorf("failed to decrypt credential: %w", err)
		}
		return credential, nil
	}

	return credential, CredentialNotFoundError{Contexts: contexts, Name: name}
}

// UpsertCredential creates or replaces a credential identified by context+name.
func (c *Client) UpsertCredential(ctx context.Context, credential types.Credential) error {
	if credential.Context == "" || credential.Name == "" {
		return fmt.Errorf("credential context and name are required")
	}
	if credential.Secrets == nil {
		credential.Secrets = map[string]string{}
	}
	credential.Encrypted = false
	if err := c.encryptCredential(ctx, &credential); err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	return c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "context"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"secrets", "encrypted"}),
	}).Create(&credential).Error
}

// DeleteCredential deletes a credential if it exists and returns whether a credential was deleted.
func (c *Client) DeleteCredential(ctx context.Context, context, name string) (bool, error) {
	result := c.db.WithContext(ctx).Where("context = ? AND name = ?", context, name).Delete(&types.Credential{})
	if result.Error != nil {
		return false, fmt.Errorf("failed to delete credential: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (c *Client) encryptCredential(ctx context.Context, credential *types.Credential) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[credentialGroupResource]
	if transformer == nil {
		return nil
	}

	secretsJSON, err := json.Marshal(credential.Secrets)
	if err != nil {
		return fmt.Errorf("failed to marshal secrets: %w", err)
	}

	b, err := transformer.TransformToStorage(ctx, secretsJSON, credentialDataCtx(credential))
	if err != nil {
		return err
	}

	credential.Secrets = map[string]string{
		credentialEncryptedSecretsKey: base64.StdEncoding.EncodeToString(b),
	}
	credential.Encrypted = true
	return nil
}

func (c *Client) decryptCredential(ctx context.Context, credential *types.Credential) error {
	if !credential.Encrypted || len(credential.Secrets) != 1 || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[credentialGroupResource]
	if transformer == nil {
		return nil
	}

	encryptedSecrets := credential.Secrets[credentialEncryptedSecretsKey]
	if encryptedSecrets == "" {
		return fmt.Errorf("encrypted secrets is missing")
	}

	decoded, err := base64.StdEncoding.DecodeString(encryptedSecrets)
	if err != nil {
		return fmt.Errorf("failed to decode encrypted secrets: %w", err)
	}

	out, _, err := transformer.TransformFromStorage(ctx, decoded, credentialDataCtx(credential))
	if err != nil {
		return err
	}

	var secrets map[string]string
	if err := json.Unmarshal(out, &secrets); err != nil {
		return fmt.Errorf("failed to unmarshal secrets: %w", err)
	}

	credential.Secrets = secrets
	return nil
}

func credentialDataCtx(credential *types.Credential) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s///%s", credential.Name, credential.Context))
}

func blankCredentialSecrets(secrets map[string]string) map[string]string {
	if len(secrets) == 0 {
		return secrets
	}
	blank := make(map[string]string, len(secrets))
	for key := range secrets {
		blank[key] = ""
	}
	return blank
}
