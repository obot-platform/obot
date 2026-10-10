package mcp

import (
	"archive/tar"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestPopulateFilesVolume(t *testing.T) {
	files := map[string]string{
		"large":   strings.Repeat("x", 1024*1024),
		"empty":   "",
		"literal": "before\nEOF\n$(do-not-execute)\nafter",
	}

	for _, failure := range []string{"", "copy", "start", "wait", "exit", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			var calls []string
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := strings.TrimPrefix(r.URL.Path, "/v1.44")
				w.Header().Set("Content-Type", "application/json")

				switch {
				case strings.HasPrefix(p, "/images/"):
					_, _ = io.WriteString(w, `{}`)

				case p == "/containers/create":
					calls = append(calls, "create")

					var config container.Config
					require.NoError(t, json.NewDecoder(r.Body).Decode(&config))
					require.Less(t, len(strings.Join(config.Cmd, " ")), 1024)
					require.NotContains(t, strings.Join(config.Cmd, " "), "do-not-execute")
					_, _ = io.WriteString(w, `{"Id":"init"}`)

				case p == "/containers/init/archive":
					calls = append(calls, "copy")
					require.Equal(t, http.MethodPut, r.Method)
					require.Equal(t, "/tmp", r.URL.Query().Get("path"))

					reader := tar.NewReader(r.Body)
					got := map[string]string{}
					for {
						header, err := reader.Next()
						if err == io.EOF {
							break
						}
						require.NoError(t, err)
						if header.Typeflag == tar.TypeDir {
							continue
						}

						require.EqualValues(t, 0644, header.Mode)
						data, err := io.ReadAll(reader)
						require.NoError(t, err)
						got[strings.TrimPrefix(header.Name, "obot-files/")] = string(data)
					}

					require.Equal(t, files, got)
					if failure == "copy" {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = io.WriteString(w, `{"message":"copy failed"}`)
					}

				case p == "/containers/init/start":
					calls = append(calls, "start")
					if failure == "start" {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = io.WriteString(w, `{"message":"start failed"}`)
					} else {
						w.WriteHeader(http.StatusNoContent)
					}

				case p == "/containers/init/wait":
					calls = append(calls, "wait")
					switch failure {
					case "wait":
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = io.WriteString(w, `{"message":"wait failed"}`)
					case "exit":
						_, _ = io.WriteString(w, `{"StatusCode":1}`)
					case "cancel":
						cancel()

					default:
						_, _ = io.WriteString(w, `{"StatusCode":0}`)
					}

				case p == "/containers/init" && r.Method == http.MethodDelete:
					calls = append(calls, "remove")
					require.Equal(t, "1", r.URL.Query().Get("force"))
					w.WriteHeader(http.StatusNoContent)

				default:
					t.Errorf("unexpected Docker API request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer api.Close()

			cli, err := client.NewClientWithOpts(client.WithHost(api.URL), client.WithVersion("1.44"))
			require.NoError(t, err)
			defer cli.Close()

			backend := &dockerBackend{client: cli}
			err = backend.populateFilesVolume(ctx, "volume", "server", files)
			if failure == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			want := []string{"create", "copy"}
			if failure != "copy" {
				want = append(want, "start")
				if failure != "start" {
					want = append(want, "wait")
				}
			}
			want = append(want, "remove")
			require.Equal(t, want, calls)
		})
	}
}

// Opt in to exercise the real Docker volume and non-root wrapper permissions.
func TestDockerFilesVolumeIntegration(t *testing.T) {
	if os.Getenv("OBOT_TEST_DOCKER") != "1" {
		t.Skip("set OBOT_TEST_DOCKER=1 to use the local Docker daemon")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer cli.Close()

	v, err := cli.VolumeCreate(ctx, volume.CreateOptions{})
	require.NoError(t, err)
	defer func() { require.NoError(t, cli.VolumeRemove(context.Background(), v.Name, true)) }()

	backend := &dockerBackend{client: cli}
	require.NoError(t, backend.populateFilesVolume(ctx, v.Name, "openapi-test", map[string]string{"stale": "old", "schema": "old"}))

	data := strings.Repeat("x", 1024*1024-5) + "\nEOF\n"
	require.NoError(t, backend.populateFilesVolume(ctx, v.Name, "openapi-test", map[string]string{"schema": data, "empty": ""}))

	out, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--user", "10001:10001", "-v", v.Name+":/files:ro", "alpine:latest", "sh", "-c", "test ! -e /files/stale && test -f /files/empty && test ! -s /files/empty && cat /files/schema").Output()
	require.NoError(t, err)
	require.Equal(t, data, string(out))
}
