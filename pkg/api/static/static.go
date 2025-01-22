package static

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func Wrap(next http.Handler, dir string) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.Handle("/", next)

	fs := http.FileServer(http.Dir(dir))

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			mux.Handle("GET /"+entry.Name()+"/", fs)
			continue
		}

		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		mux.Handle("GET /"+entry.Name(), fs)

		if entry.Name() == "index.html" {
			mux.Handle("GET /{$}", fs)
		}
	}

	return mux, nil
}
