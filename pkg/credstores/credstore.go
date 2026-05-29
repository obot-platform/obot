package credstores

import (
	"fmt"
	"strings"
)

func Init(port int, token string) (string, []string, error) {
	return fmt.Sprintf("http://localhost:%d/api/credentials/tool.gpt", port), []string{
		"GPTSCRIPT_HTTP_ENV=OBOT_CREDSTORE_API_TOKEN",
		"OBOT_CREDSTORE_API_TOKEN=" + token,
	}, nil
}

func GPTScriptSQLiteFile(dsn string) (string, error) {
	dbFile, ok := strings.CutPrefix(dsn, "sqlite://file:")
	if !ok {
		return "", fmt.Errorf("invalid sqlite dsn, must start with sqlite://file: %s", dsn)
	}
	dbFile, _, _ = strings.Cut(dbFile, "?")

	if !strings.HasSuffix(dbFile, ".db") {
		return "", fmt.Errorf("invalid sqlite dsn, file must end in .db: %s", dsn)
	}

	return strings.TrimSuffix(dbFile, ".db") + "-credentials.db", nil
}
