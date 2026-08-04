package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0 Safari/537.36"

// hashedAsset is named the way the build names a file whose contents decide its
// name. Nothing else in the tree is safe to cache for a year.
const hashedAsset = "/_app/immutable/entry/app.D4Z7Uh6d.js"

// withBuild swaps in a stand-in for the compiled UI. The real one exists only
// after the UI has been built, so without this these tests would report on
// whether someone had run make rather than on the handler.
func withBuild(t *testing.T) {
	t.Helper()
	previous := assets
	t.Cleanup(func() { assets = previous })

	assets = fstest.MapFS{
		"user/build/index.html":                           {Data: []byte("<html>shell</html>")},
		"user/build/fallback.html":                        {Data: []byte("<html>fallback</html>")},
		"user/build/_app/version.json":                    {Data: []byte(`{"version":"1"}`)},
		"user/build/_app/immutable/entry/app.D4Z7Uh6d.js": {Data: []byte("export default 1")},
	}
}

func get(t *testing.T, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("User-Agent", browserUA)
	rec := httptest.NewRecorder()
	Handler(0, 0).ServeHTTP(rec, req)
	return rec.Result()
}

// A build asset is named by a hash of its own contents, so a request for one
// that is not here is asking for something that does not exist. Answering with
// the page shell instead returns 200 text/html, which every cache in the path
// then stores -- the CDN edge, and each visitor's browser. A rolling deploy is
// enough to produce the miss, because for a few seconds one replica serves a
// shell naming hashes another replica does not have yet. That window is short;
// what it leaves behind is not, and clearing it means purging the CDN and every
// browser that saw it.
func TestMissingBuildAssetIsNotFound(t *testing.T) {
	withBuild(t)

	for _, path := range []string{
		"/_app/immutable/entry/app.NOTREAL.js",
		"/_app/immutable/chunks/NOTREAL.js",
		"/_app/immutable/assets/0.NOTREAL.css",
	} {
		t.Run(path, func(t *testing.T) {
			resp := get(t, path)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status = %d, want 404 -- a 200 here is cacheable and outlives the deploy that caused it", resp.StatusCode)
			}
		})
	}
}

// A real asset is still served, so the 404 above is about absence rather than
// about the prefix.
func TestPresentBuildAssetIsServed(t *testing.T) {
	withBuild(t)

	resp := get(t, hashedAsset)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// Everything outside the build directory is a client-side route, resolved by
// the shell once it loads. Those must keep falling back, or every deep link
// breaks.
func TestUnknownRouteStillFallsBack(t *testing.T) {
	withBuild(t)

	resp := get(t, "/admin/dashboard")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 so the shell can route it", resp.StatusCode)
	}
}

// Obot stated no cache policy for the UI, so whatever CDN sat in front applied
// its own -- four hours, in the case that went on serving a page shell in place
// of an asset long after the deploy that caused it. Saying it here keeps the
// decision where the meaning of each response is known.
func TestCachePolicy(t *testing.T) {
	withBuild(t)

	for _, tt := range []struct {
		name, path, want string
	}{
		// A change to one of these is a change to its name, so nothing a client
		// holds can ever be wrong.
		{"hashed asset", hashedAsset, "public, max-age=31536000, immutable"},
		// Names the assets of the build it came from, so a cached copy goes on
		// asking for files a later build no longer has.
		{"page shell", "/", "no-cache"},
		// Reached through the same fallback, and carries the same references.
		{"client-side route", "/admin/dashboard", "no-cache"},
		// Under the build directory, but keeps its name across builds.
		{"version.json", "/_app/version.json", "no-cache"},
		// Storing this is what turns seconds of rollout skew into hours of it.
		{"missing asset", "/_app/immutable/chunks/NOTREAL.js", "no-store"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := get(t, tt.path)
			defer resp.Body.Close()

			if got := resp.Header.Get("Cache-Control"); got != tt.want {
				t.Errorf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}
