package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerSetupUsesMCPBaseImage(t *testing.T) {
	for _, setup := range []string{"mmmcp", "files"} {
		for _, image := range []struct {
			name string
			repo string
			tag  string
			ref  string
		}{
			{
				name: "default",
				repo: "ghcr.io/obot-platform/mcp-images/stdio-wrapper",
				tag:  "v0.26.2",
				ref:  "ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.26.2",
			},
			{
				name: "mirror digest",
				repo: "registry.example.com/mcp/stdio-wrapper",
				tag:  "sha256:" + strings.Repeat("a", 64),
				ref:  "registry.example.com/mcp/stdio-wrapper@sha256:" + strings.Repeat("a", 64),
			},
		} {
			for _, cached := range []bool{true, false} {
				cacheName := "missing"
				if cached {
					cacheName = "cached"
				}
				t.Run(setup+"/"+image.name+"/"+cacheName, func(t *testing.T) {
					var inspected, pulled, created, copied, started, waited atomic.Bool
					target := "/files"
					if setup == "mmmcp" {
						target = "/config"
					}
					api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						requestPath := strings.TrimPrefix(r.URL.Path, "/v1.44")
						switch {
						case r.Method == http.MethodPost && requestPath == "/volumes/create":
							w.WriteHeader(http.StatusCreated)
							_, _ = io.WriteString(w, `{"Name":"setup-volume"}`)
						case r.Method == http.MethodGet && strings.HasPrefix(requestPath, "/images/") && strings.HasSuffix(requestPath, "/json"):
							inspected.Store(true)
							assert.Equal(t, "/images/"+image.ref+"/json", requestPath)
							if cached {
								_, _ = io.WriteString(w, `{}`)
							} else {
								w.WriteHeader(http.StatusNotFound)
								_, _ = io.WriteString(w, `{"message":"image missing"}`)
							}
						case r.Method == http.MethodPost && requestPath == "/images/create":
							pulled.Store(true)
							assert.Equal(t, image.repo, r.URL.Query().Get("fromImage"))
							assert.Equal(t, image.tag, r.URL.Query().Get("tag"))
							_, _ = io.WriteString(w, `{"status":"pulled"}`)
						case r.Method == http.MethodPost && requestPath == "/containers/create":
							created.Store(true)
							var config struct {
								container.Config
								HostConfig container.HostConfig
							}
							if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&config)) {
								w.WriteHeader(http.StatusBadRequest)
								return
							}
							assert.Equal(t, image.ref, config.Image)
							assert.Equal(t, "0", config.User)
							assert.Equal(t, []string{"sh", "-c"}, []string(config.Entrypoint))
							assert.True(t, config.HostConfig.AutoRemove)
							if assert.Len(t, config.HostConfig.Mounts, 1) {
								assert.Equal(t, mount.TypeVolume, config.HostConfig.Mounts[0].Type)
								assert.Equal(t, target, config.HostConfig.Mounts[0].Target)
							}
							w.WriteHeader(http.StatusCreated)
							_, _ = io.WriteString(w, `{"Id":"init"}`)
						case r.Method == http.MethodPut && requestPath == "/containers/init/archive":
							copied.Store(true)
							assert.Equal(t, "/", r.URL.Query().Get("path"))
							_, err := io.Copy(io.Discard, r.Body)
							assert.NoError(t, err)
						case r.Method == http.MethodPost && requestPath == "/containers/init/start":
							started.Store(true)
							w.WriteHeader(http.StatusNoContent)
						case r.Method == http.MethodPost && requestPath == "/containers/init/wait":
							waited.Store(true)
							_, _ = io.WriteString(w, `{"StatusCode":0}`)
						default:
							t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL)
							w.WriteHeader(http.StatusNotFound)
						}
					}))
					t.Cleanup(api.Close)
					cli, err := client.NewClientWithOpts(client.WithHost(api.URL), client.WithVersion("1.44"))
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, cli.Close()) })
					d := &dockerBackend{
						client:                 cli,
						containerizedBaseImage: image.ref,
					}
					if setup == "mmmcp" {
						_, err = d.prepareMCPServerMMMCPConfig(t.Context(), ServerConfig{
							MCPServerName: "test-server",
							Command:       "npx",
						}, nil)
					} else {
						_, _, err = d.createVolumeWithFiles(t.Context(), []File{{
							EnvKey: "CONFIG",
							Data:   "file contents",
						}}, "test-server", "deployment")
					}
					require.NoError(t, err)
					assert.True(t, inspected.Load())
					assert.Equal(t, !cached, pulled.Load())
					assert.True(t, created.Load())
					assert.True(t, copied.Load())
					assert.True(t, started.Load())
					assert.True(t, waited.Load())
				})
			}
		}
	}
}
