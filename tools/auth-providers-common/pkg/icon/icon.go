package icon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Fetcher func(context.Context, string) (string, error)

func ObotGetIconURL(iconFetcher Fetcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization header", http.StatusBadRequest)
			return
		}

		accessToken := strings.TrimPrefix(auth, "Bearer ")
		if accessToken == "" {
			http.Error(w, "missing access token", http.StatusBadRequest)
			return
		}

		iconURL, err := iconFetcher(r.Context(), accessToken)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch profile icon URL: %v", err), http.StatusBadRequest)
			return
		}

		type response struct {
			IconURL string `json:"iconURL"`
		}

		if err := json.NewEncoder(w).Encode(response{IconURL: iconURL}); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
			return
		}
	}
}
