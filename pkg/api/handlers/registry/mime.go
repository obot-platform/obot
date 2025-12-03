package registry

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type mimeFetcher struct {
	lock  sync.RWMutex
	cache map[string]string
}

func newMimeFetcher() *mimeFetcher {
	return &mimeFetcher{
		lock:  sync.RWMutex{},
		cache: make(map[string]string),
	}
}

func (m *mimeFetcher) guessMimeType(ctx context.Context, iconURL string) string {
	// First, try to guess from the file extension
	lower := strings.ToLower(iconURL)
	if strings.HasSuffix(lower, ".png") {
		return "image/png"
	}
	if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") {
		return "image/jpeg"
	}
	if strings.HasSuffix(lower, ".svg") {
		return "image/svg+xml"
	}
	if strings.HasSuffix(lower, ".webp") {
		return "image/webp"
	}

	// If we couldn't guess from the extension, try to fetch the URL
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return m.fetchAndDetectMimeType(ctx, iconURL)
	}

	return ""
}

func (m *mimeFetcher) fetchAndDetectMimeType(ctx context.Context, url string) string {
	m.lock.RLock()
	mimeType, ok := m.cache[url]
	m.lock.RUnlock()
	if ok {
		return mimeType
	}

	// Create a context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}

	// Perform the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// First, check the Content-Type header
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		// Extract just the MIME type (before any semicolon/parameters)
		if idx := strings.Index(contentType, ";"); idx > 0 {
			contentType = contentType[:idx]
		}
		contentType = strings.TrimSpace(contentType)

		// Validate it's an image MIME type
		if strings.HasPrefix(contentType, "image/") {
			m.lock.Lock()
			m.cache[url] = contentType
			m.lock.Unlock()
			return contentType
		}
	}

	// If header wasn't useful, read first 512 bytes to detect content type
	buffer := make([]byte, 512)
	n, err := resp.Body.Read(buffer)
	if err != nil && n == 0 {
		return ""
	}

	// Detect content type from the actual data
	detectedType := http.DetectContentType(buffer[:n])

	// Only return if it's an image type
	if strings.HasPrefix(detectedType, "image/") {
		m.lock.Lock()
		m.cache[url] = detectedType
		m.lock.Unlock()
		return detectedType
	}

	return ""
}
