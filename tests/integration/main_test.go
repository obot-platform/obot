//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/server"
	"github.com/obot-platform/obot/pkg/services"
)

type obotApplication struct {
	cancel          context.CancelFunc
	done            chan error
	workDir         string
	originalWorkDir string
	exited          bool
}

var providerAPIKeyEnvVars = []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}

func TestMain(m *testing.M) {
	app, err := startObotApplication()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start Obot integration application: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	if err := app.stop(); err != nil {
		fmt.Fprintf(os.Stderr, "stop Obot integration application: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func startObotApplication() (*obotApplication, error) {
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

	app := &obotApplication{
		workDir:         workDir,
		originalWorkDir: originalWorkDir,
		done:            make(chan error, 1),
	}
	if err := os.Setenv("OBOT_INTEGRATION_BASE_URL", baseURL); err != nil {
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
	config := integrationServerConfig(httpPort, storagePort, workDir)
	services.SetupLogBridges()
	go func() {
		app.done <- server.Run(ctx, config)
	}()

	if err := app.waitForHealth(baseURL, 2*time.Minute); err != nil {
		return nil, errors.Join(err, app.stop())
	}
	return app, nil
}

func integrationServerConfig(httpPort, storagePort int, workDir string) services.Config {
	config := services.Config{
		HTTPListenPort:                    httpPort,
		DevMode:                           true,
		ElectionFile:                      filepath.Join(workDir, "election"),
		MCPOAuthClientExpiration:          "30d",
		DisableUpdateCheck:                true,
		MCPServerSearchImage:              "ghcr.io/obot-platform/obot-mcp-server:v0.2.0",
		StorageListenPort:                 storagePort,
		DSN:                               "sqlite://file:" + filepath.Join(workDir, "obot.db") + "?_journal=WAL&_busy_timeout=30000",
		DailyUserInputTokenLimit:          -1,
		DailyUserOutputTokenLimit:         -1,
		UnauthenticatedRateLimit:          100,
		AuthenticatedRateLimit:            200,
		AuditLogsMode:                     "off",
		MCPRuntimeBackend:                 "docker",
		MCPBaseImage:                      "ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.25.0",
		MCPSecretBindingAllowedLabel:      "obot.obot.ai/allow-secret-binding",
		SingleUserIdleServerShutdownHours: -1,
		MultiUserIdleServerShutdownHours:  -1,
		IdleAgentShutdownHours:            -1,
		MCPAuditLogPersistIntervalSeconds: 5,
		MCPAuditLogsPersistBatchSize:      1000,
	}
	return config
}

func availablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
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
	if err := os.Setenv("DOCKER_HOST", host); err != nil {
		return err
	}
	return nil
}

func (a *obotApplication) waitForHealth(baseURL string, timeout time.Duration) error {
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
			return fmt.Errorf("Obot exited before becoming healthy: %w", err)
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for %s/api/healthz", baseURL)
		case <-ticker.C:
			resp, err := client.Get(baseURL + "/api/healthz")
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

func (a *obotApplication) stop() error {
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

func (a *obotApplication) cleanup() error {
	var result error
	result = errors.Join(result, os.Chdir(a.originalWorkDir))
	result = errors.Join(result, os.RemoveAll(a.workDir))
	return result
}
