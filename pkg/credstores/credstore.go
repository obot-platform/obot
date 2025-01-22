package credstores

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/input"
	"github.com/gptscript-ai/gptscript/pkg/loader"
	"github.com/obot-platform/obot/logger"
)

type Options struct {
	AWSKMSKeyARN         string
	EncryptionConfigFile string
}

var log = logger.Package()

func Init(ctx context.Context, toolRegistries []string, dsn string, opts Options) (string, []string, error) {
	if err := setupKMS(ctx, opts.AWSKMSKeyARN, opts.EncryptionConfigFile); err != nil {
		return "", nil, fmt.Errorf("failed to setup kms: %w", err)
	}

	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		return setupSQLite(toolRegistries, dsn)
	case strings.HasPrefix(dsn, "postgres://"):
		return setupPostgres(toolRegistries, dsn)
	default:
		return "", nil, fmt.Errorf("unsupported database for credentials %s", strings.Split(dsn, "://")[0])
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

func setupPostgres(toolRegistries []string, dsn string) (string, []string, error) {
	toolRef, err := resolveToolRef(toolRegistries, "credential-stores/postgres")
	if err != nil {
		return "", nil, err
	}

	return toolRef, []string{
		"GPTSCRIPT_POSTGRES_DSN=" + dsn,
	}, nil
}

func setupSQLite(toolRegistries []string, dsn string) (string, []string, error) {
	dbFile, ok := strings.CutPrefix(dsn, "sqlite://file:")
	if !ok {
		return "", nil, fmt.Errorf("invalid sqlite dsn, must start with sqlite://file: %s", dsn)
	}
	dbFile, _, _ = strings.Cut(dbFile, "?")

	if !strings.HasSuffix(dbFile, ".db") {
		return "", nil, fmt.Errorf("invalid sqlite dsn, file must end in .db: %s", dsn)
	}

	dbFile = strings.TrimSuffix(dbFile, ".db") + "-credentials.db"

	toolRef, err := resolveToolRef(toolRegistries, "credential-stores/sqlite")
	if err != nil {
		return "", nil, err
	}

	return toolRef, []string{
		"GPTSCRIPT_SQLITE_FILE=" + dbFile,
	}, nil
}

func resolveToolRef(toolRegistries []string, relToolPath string) (string, error) {
	for _, toolRegistry := range toolRegistries {
		if remapped := loader.Remap[toolRegistry]; remapped != "" {
			toolRegistry = remapped
		}

		// This doesn't support registry references with revisions; e.g. `<registry>@<revision>`
		ref := fmt.Sprintf("%s/%s", toolRegistry, relToolPath)
		content, err := input.FromLocation(ref+"/tool.gpt", true)
		if err != nil || content == "" {
			continue
		}

		// Note: We could parse the content here to be extra sure the tool we're looking for exists,
		// but this is probably good enough for now.
		return ref, nil
	}

	return "", fmt.Errorf("%q not found in provided tool registries", relToolPath)
}
