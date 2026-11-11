//go:build integration

package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/server"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	// BaseURLEnv names the environment variable that carries the app's URL from TestMain to New.
	BaseURLEnv = "OBOT_INTEGRATION_BASE_URL"
)

var (
	providerAPIKeyEnvVars = []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}
)

// App is an Obot server running in this process on SQLite, in a working directory of its own.
type App struct {
	BaseURL     string
	StoragePort int

	cancel          context.CancelFunc
	done            chan error
	workDir         string
	originalWorkDir string
	exited          bool
}

// StartApp starts Obot and waits for it to be healthy. configure, if set, changes the server's configuration before
// it starts. The MCP backend is Docker, and DOCKER_HOST is taken from the active Docker context unless it's set.
func StartApp(configure func(*services.Config)) (*App, error) {
	httpPort, err := availablePort()
	if err != nil {
		return nil, fmt.Errorf("choose HTTP port: %w", err)
	}
	storagePort := httpPort
	for storagePort == httpPort {
		storagePort, err = availablePort()
		if err != nil {
			return nil, fmt.Errorf("choose storage port: %w", err)
		}
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", httpPort)

	workDir, err := os.MkdirTemp("", "obot-integration.*")
	if err != nil {
		return nil, err
	}
	originalWorkDir, err := os.Getwd()
	if err != nil {
		_ = os.RemoveAll(workDir)
		return nil, err
	}
	if err := os.Chdir(workDir); err != nil {
		_ = os.RemoveAll(workDir)
		return nil, err
	}

	app := &App{
		BaseURL:         baseURL,
		StoragePort:     storagePort,
		workDir:         workDir,
		originalWorkDir: originalWorkDir,
		done:            make(chan error, 1),
	}
	if err := os.Setenv(BaseURLEnv, baseURL); err != nil {
		_ = app.cleanup()
		return nil, err
	}
	for _, name := range providerAPIKeyEnvVars {
		if err := os.Unsetenv(name); err != nil {
			_ = app.cleanup()
			return nil, fmt.Errorf("unset %s for integration test: %w", name, err)
		}
	}
	if err := configureDockerHost(); err != nil {
		_ = app.cleanup()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel
	config := serverConfig(httpPort, storagePort, workDir)
	if configure != nil {
		configure(&config)
	}
	services.SetupLogBridges()
	go func() {
		app.done <- server.Run(ctx, config)
	}()

	if err := app.waitFor(baseURL+"/api/healthz", 2*time.Minute); err != nil {
		return nil, errors.Join(err, app.Stop())
	}
	return app, nil
}

// WaitForDefaultCatalog waits for the default MCPCatalog to exist. The controller creates it in
// its PostStart hook, independently of the HTTP server, so a healthy server does not mean the
// catalog is there yet. It needs an app without authentication.
func (a *App) WaitForDefaultCatalog(timeout time.Duration) error {
	return a.waitFor(a.BaseURL+"/api/mcp-catalogs/"+system.DefaultCatalog, timeout)
}

// Stop stops the server and removes its working directory.
func (a *App) Stop() error {
	var result error
	if !a.exited {
		a.cancel()
		select {
		case err := <-a.done:
			result = errors.Join(result, err)
		case <-time.After(30 * time.Second):
			result = errors.Join(result, errors.New("timed out waiting for Obot to stop"))
		}
	}
	return errors.Join(result, a.cleanup())
}

func serverConfig(httpPort, storagePort int, workDir string) services.Config {
	return services.Config{
		GitMaxRepoSizeMB:                  100,
		HTTPListenPort:                    httpPort,
		DevMode:                           true,
		ElectionFile:                      filepath.Join(workDir, "election"),
		MCPOAuthClientExpiration:          "30d",
		DisableUpdateCheck:                true,
		StorageListenPort:                 storagePort,
		DSN:                               "sqlite://file:" + filepath.Join(workDir, "obot.db") + "?_journal=WAL&_busy_timeout=30000",
		DailyUserInputTokenLimit:          -1,
		DailyUserOutputTokenLimit:         -1,
		UnauthenticatedRateLimit:          100,
		AuthenticatedRateLimit:            200,
		AuditLogsMode:                     "off",
		MCPRuntimeBackend:                 "docker",
		MCPBaseImage:                      "ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.26.2",
		MCPSecretBindingAllowedLabel:      "obot.obot.ai/allow-secret-binding",
		SingleUserIdleServerShutdownHours: -1,
		MultiUserIdleServerShutdownHours:  -1,
		MCPAuditLogPersistIntervalSeconds: 5,
		MCPAuditLogsPersistBatchSize:      1000,
	}
}

// availablePort returns a free port below the ranges the OS takes outgoing connections' ports from. A port from
// those ranges can be taken again, by a connection the server itself makes, before the server listens on it.
func availablePort() (int, error) {
	const first, count = 20000, 10000
	var lastErr error
	for range 100 {
		port := first + rand.IntN(count)
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			lastErr = err
			continue
		}
		if err := listener.Close(); err != nil {
			return 0, err
		}
		return port, nil
	}
	return 0, fmt.Errorf("no free port found: %w", lastErr)
}

func configureDockerHost() error {
	if os.Getenv("DOCKER_HOST") != "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	if err != nil {
		return fmt.Errorf("inspect active Docker context: %w", err)
	}
	host := strings.TrimSpace(string(output))
	if host == "" {
		return errors.New("active Docker context has no endpoint")
	}
	return os.Setenv("DOCKER_HOST", host)
}

// waitFor polls url until it answers 200, the server exits, or timeout passes.
func (a *App) waitFor(url string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}

	for {
		select {
		case err := <-a.done:
			a.exited = true
			if err == nil {
				err = errors.New("server exited without an error")
			}
			return fmt.Errorf("the server exited before %s answered: %w", url, err)
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for %s", url)
		case <-ticker.C:
			resp, err := client.Get(url)
			if err != nil {
				continue
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

func (a *App) cleanup() error {
	var result error
	result = errors.Join(result, os.Chdir(a.originalWorkDir))
	result = errors.Join(result, os.RemoveAll(a.workDir))
	return result
}
