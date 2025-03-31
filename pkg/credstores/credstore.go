package credstores

import (
	"fmt"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/input"
	"github.com/gptscript-ai/gptscript/pkg/loader"
)

func Init(toolRegistries []string, dsn, encryptionConfigFile string) (string, []string, error) {
	// Set up database
	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		return setUpSQLite(toolRegistries, dsn, encryptionConfigFile)
	case strings.HasPrefix(dsn, "postgres://"):
		return setUpPostgres(toolRegistries, dsn, encryptionConfigFile)
	default:
		return "", nil, fmt.Errorf("unsupported database for credentials %s", strings.Split(dsn, "://")[0])
	}
}

func setUpPostgres(toolRegistries []string, dsn, encryptionConfigFile string) (string, []string, error) {
	toolRef, err := resolveToolRef(toolRegistries, "credential-stores/postgres")
	if err != nil {
		return "", nil, err
	}

	return toolRef, []string{
		"GPTSCRIPT_POSTGRES_DSN=" + dsn,
		"GPTSCRIPT_ENCRYPTION_CONFIG_FILE=" + encryptionConfigFile,
	}, nil
}

func setUpSQLite(toolRegistries []string, dsn, encryptionConfigFile string) (string, []string, error) {
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
		"GPTSCRIPT_ENCRYPTION_CONFIG_FILE=" + encryptionConfigFile,
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
