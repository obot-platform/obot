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

	target := http.FileServer(http.Dir(dir))
	fs := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if len(req.URL.Path) > 1 {
			if strings.HasSuffix(req.URL.Path, "/") {
				req.URL.Path = req.URL.Path[:len(req.URL.Path)-1] + ".html"
			} else {
				parts := strings.Split(req.URL.Path, "/")
				if !strings.Contains(parts[len(parts)-1], ".") {
					req.URL.Path += ".html"
				}
			}
		}
		target.ServeHTTP(rw, req)
	})

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
		} else if trimmed, ok := strings.CutSuffix(entry.Name(), ".html"); ok {
			mux.Handle("GET /"+trimmed, fs)
		}
	}

	return mux, nil
}
