package credstores

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/obot-platform/obot/logger"
)

type Options struct {
	AWSKMSKeyARN         string
	EncryptionConfigFile string
}

var log = logger.Package()

func Init(ctx context.Context, toolsRegistry, dsn string, opts Options) (string, []string, error) {
	if err := setupKMS(ctx, opts.AWSKMSKeyARN, opts.EncryptionConfigFile); err != nil {
		return "", nil, fmt.Errorf("failed to setup kms: %w", err)
	}

	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		return setupSQLite(toolsRegistry, dsn)
	case strings.HasPrefix(dsn, "postgres://"):
		return setupPostgres(toolsRegistry, dsn)
	default:
		return "", nil, fmt.Errorf("unsupported database for credentials %s", dsn)
	}
}

func setupKMS(ctx context.Context, arn, configFile string) error {
	if arn == "" {
		return nil
	}

	if configFile != "" {
		if err := os.Setenv("GPTSCRIPT_ENCRYPTION_CONFIG_FILE", configFile); err != nil {
			return fmt.Errorf("failed to set GPTSCRIPT_ENCRYPTION_CONFIG_FILE: %w", err)
		}
	}

	region := strings.Split(arn, ":")[3]

	cmd := exec.CommandContext(ctx,
		"aws-encryption-provider",
		"--health-port=127.0.0.1:0",
		"--region="+region,
		"--key="+arn,
		"--listen=/tmp/aws-cred-socket.sock")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		select {
		case <-ctx.Done():
			// ignore error if we are shutting down
		default:
			log.Fatalf("aws-encryption-provider exited: %v", err)
		}
	}()

	return nil
}

func setupPostgres(toolRegistry, dsn string) (string, []string, error) {
	return toolRegistry + "/credential-stores/postgres", []string{
		"GPTSCRIPT_POSTGRES_DSN=" + dsn,
	}, nil
}

func setupSQLite(toolRegistry, dsn string) (string, []string, error) {
	dbFile, ok := strings.CutPrefix(dsn, "sqlite://file:")
	if !ok {
		return "", nil, fmt.Errorf("invalid sqlite dsn, must start with sqlite://file: %s", dsn)
	}
	dbFile, _, _ = strings.Cut(dbFile, "?")

	if !strings.HasSuffix(dbFile, ".db") {
		return "", nil, fmt.Errorf("invalid sqlite dsn, file must end in .db: %s", dsn)
	}

	dbFile = strings.TrimSuffix(dbFile, ".db") + "-credentials.db"

	return toolRegistry + "/credential-stores/sqlite", []string{
		"GPTSCRIPT_SQLITE_FILE=" + dbFile,
	}, nil
}
