package ui

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"testing/fstest"
)

const (
	chromeUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
)

func testUIServer() *uiServer {
	return newUIServer(fstest.MapFS{
		"user/build/index.html":                       {Data: []byte("<html>index</html>")},
		"user/build/admin.html":                       {Data: []byte("<html>admin</html>")},
		"user/build/fallback.html":                    {Data: []byte("<html>fallback</html>")},
		"user/build/mcp-servers.html":                 {Data: []byte("<html>mcp-servers</html>")},
		"user/build/_app/immutable/nodes/0.abc123.js": {Data: []byte("export const x = 1")},
		"user/build/favicon.ico":                      {Data: []byte("icon")},
	})
}

// serveConditional repeats a request with the ETag the first response carried,
// which is what a browser does when it revalidates a copy it already holds.
func serveConditional(t *testing.T, urlPath, etag string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://obot.example.com"+urlPath, nil)
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	testUIServer().ServeHTTP(rec, req)

	return rec
}

func serve(t *testing.T, urlPath, userAgent string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://obot.example.com"+urlPath, nil)
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	rec := httptest.NewRecorder()
	testUIServer().ServeHTTP(rec, req)
	return rec
}

// A cached 404 for a hashed asset breaks the app for every client behind that
// cache, so files that exist must serve regardless of User-Agent.
func TestExistingAssetsServeToNonBrowserClients(t *testing.T) {
	for _, ua := range []string{
		"Python/3.12 aiohttp/3.14.3",
		"curl/8.7.1",
		"Go-http-client/2.0",
		"", // no User-Agent header at all
		chromeUA,
	} {
		rec := serve(t, "/_app/immutable/nodes/0.abc123.js", ua)
		if rec.Code != http.StatusOK {
			t.Errorf("User-Agent %q: expected 200 for an asset that exists, got %d", ua, rec.Code)
		}
	}
}

// Unknown paths must not hand programmatic clients the SPA fallback, or a
// mistyped API path looks like a 200 full of HTML.
func TestUnknownPathReturns404ForNonBrowsers(t *testing.T) {
	rec := serve(t, "/api/definitely-not-a-route", "Go-http-client/2.0")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for a non-browser client, got %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("expected Cache-Control no-store so the 404 is never cached, got %q", got)
	}
}

// Browsers still get the SPA fallback so client-side routes survive a refresh.
func TestUnknownPathServesFallbackForBrowsers(t *testing.T) {
	rec := serve(t, "/some/client/side/route", chromeUA)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 fallback for a browser, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>fallback</html>" {
		t.Errorf("expected the SPA fallback body, got %q", body)
	}
}

// Hashed assets can be cached forever; the HTML that names them must not be, or
// a client keeps asking for chunks from a build that is no longer deployed.
func TestCacheHeaders(t *testing.T) {
	for _, tt := range []struct {
		name     string
		urlPath  string
		expected string
	}{
		{"hashed asset", "/_app/immutable/nodes/0.abc123.js", "public, max-age=31536000, immutable"},
		{"index", "/", "no-cache"},
		{"mcp-servers entry point", "/mcp-servers", "no-cache"},
		{"SPA fallback", "/some/client/side/route", "no-cache"},
		// Not content-hashed, so it must not be marked immutable. Left without a
		// directive rather than forced to revalidate on every request.
		{"favicon", "/favicon.ico", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, tt.urlPath, chromeUA)
			if got := rec.Header().Get("Cache-Control"); got != tt.expected {
				t.Errorf("expected Cache-Control %q, got %q", tt.expected, got)
			}
		})
	}
}

// The proxy was migrated from the deprecated ReverseProxy.Director to Rewrite.
// Director made ReverseProxy set X-Forwarded-For automatically; Rewrite does not
// unless SetXForwarded is called, so this pins the forwarding behavior.
func TestUIProxyForwardsToLocalhostWithXForwardedFor(t *testing.T) {
	var (
		gotHost  string
		gotXFF   string
		gotProto string
		gotPath  string
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotXFF = r.Header.Get("X-Forwarded-For")
		gotProto = r.Header.Get("X-Forwarded-Proto")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, err := net.SplitHostPort(upstreamURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://obot.example.com/some/path", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()

	newUIProxy(port).ServeHTTP(rec, req)

	// Reaching the upstream at all is what proves URL.Host routing works, since
	// the upstream only exists on localhost.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from upstream, got %d", rec.Code)
	}
	if gotPath != "/some/path" {
		t.Errorf("expected the path to be preserved, got %q", gotPath)
	}
	// Director only rewrote URL.Host, never the Host header, so the upstream still
	// sees the original host. Pinned here because Rewrite could easily change it.
	if gotHost != "obot.example.com" {
		t.Errorf("expected the inbound Host header to be passed through, got %q", gotHost)
	}
	if gotXFF != "192.0.2.10" {
		t.Errorf("expected X-Forwarded-For to carry the client IP (as Director used to), got %q", gotXFF)
	}
	if gotProto != "http" {
		t.Errorf("expected X-Forwarded-Proto http for a non-TLS hop, got %q", gotProto)
	}
}

// Serving from an embed.FS leaves Go with no validator to send, because every
// embedded file reports a zero ModTime and ServeContent omits Last-Modified for
// one. Every file we serve needs an ETag so a client holding a copy can
// revalidate it.
func TestETagIsSetOnEveryServedFile(t *testing.T) {
	for _, urlPath := range []string{
		"/",
		"/admin",
		"/mcp-servers",
		"/some/client/side/route",
		"/_app/immutable/nodes/0.abc123.js",
		"/favicon.ico",
	} {
		t.Run(urlPath, func(t *testing.T) {
			rec := serve(t, urlPath, chromeUA)
			if etag := rec.Header().Get("ETag"); etag == "" {
				t.Error("expected an ETag so the client can revalidate, got none")
			}
		})
	}
}

// Without a validator, no-cache means the client downloads the whole page again
// on every navigation. With one, revalidation costs a 304 and no body.
func TestRevalidationReturns304(t *testing.T) {
	for _, urlPath := range []string{
		"/",
		"/some/client/side/route",
		"/_app/immutable/nodes/0.abc123.js",
	} {
		t.Run(urlPath, func(t *testing.T) {
			first := serve(t, urlPath, chromeUA)
			if first.Code != http.StatusOK {
				t.Fatalf("expected 200 on the first request, got %d", first.Code)
			}

			second := serveConditional(t, urlPath, first.Header().Get("ETag"))
			if second.Code != http.StatusNotModified {
				t.Errorf("expected 304 when the client already has this copy, got %d", second.Code)
			}
			if body := second.Body.String(); body != "" {
				t.Errorf("expected no body on a 304, got %q", body)
			}
		})
	}
}

// Cloudflare rewrites a strong ETag to a weak one when it compresses a
// response, so the browser revalidates with the weak form against the strong
// one we set. RFC 7232 uses weak comparison for If-None-Match, and this pins
// that we get the 304 rather than the whole body.
func TestRevalidationAcceptsAWeakenedETag(t *testing.T) {
	first := serve(t, "/", chromeUA)

	rec := serveConditional(t, "/", "W/"+first.Header().Get("ETag"))
	if rec.Code != http.StatusNotModified {
		t.Errorf("expected 304 for a weakened form of our own ETag, got %d", rec.Code)
	}
}

// A 304 that drops Cache-Control would leave the client with a stored copy and
// no instruction to revalidate it next time.
func TestCacheControlSurvivesA304(t *testing.T) {
	first := serve(t, "/", chromeUA)

	rec := serveConditional(t, "/", first.Header().Get("ETag"))
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("expected Cache-Control no-cache on the 304, got %q", got)
	}
}

// A stale ETag has to serve the new file, or a client keeps its old copy after
// a deploy. This is the case that matters for the HTML, since each page names
// the hashed assets of the build it came from.
func TestChangedContentServesTheNewFile(t *testing.T) {
	rec := serveConditional(t, "/", `"etag-from-an-older-build"`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when the client's copy is stale, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>index</html>" {
		t.Errorf("expected the current index body, got %q", body)
	}
}

// The ETag has to identify the file, so that two different files never share
// one and the same file keeps its ETag across replicas and restarts.
func TestETagsIdentifyContent(t *testing.T) {
	index := serve(t, "/", chromeUA).Header().Get("ETag")
	asset := serve(t, "/_app/immutable/nodes/0.abc123.js", chromeUA).Header().Get("ETag")

	if index == asset {
		t.Error("expected different files to have different ETags")
	}
	if again := serve(t, "/", chromeUA).Header().Get("ETag"); again != index {
		t.Errorf("expected a stable ETag for unchanged content, got %q then %q", index, again)
	}
}

// A 404 is not a representation of anything, and it is already marked no-store
// so that a deploy adding the path is not shadowed by a cached miss.
func TestNoETagOn404(t *testing.T) {
	rec := serve(t, "/api/definitely-not-a-route", "Go-http-client/2.0")
	if etag := rec.Header().Get("ETag"); etag != "" {
		t.Errorf("expected no ETag on a 404, got %q", etag)
	}
}
