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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/server"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/sirupsen/logrus"
)

type obotApplication struct {
	cancel          context.CancelFunc
	done            chan error
	workDir         string
	originalWorkDir string
	dockerHostSet   bool
	mcpImage        string
	logLevel        logrus.Level
	exited          bool
}

var integrationMCPImage string

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
		logLevel:        logrus.GetLevel(),
		done:            make(chan error, 1),
	}
	logrus.SetLevel(logrus.WarnLevel)
	if err := os.Setenv("OBOT_INTEGRATION_BASE_URL", baseURL); err != nil {
		_ = app.cleanup()
		return nil, err
	}
	app.dockerHostSet, err = configureDockerHost()
	if err != nil {
		_ = app.cleanup()
		return nil, err
	}
	repositoryRoot, err := repositoryRoot()
	if err != nil {
		_ = app.cleanup()
		return nil, err
	}
	app.mcpImage, err = buildTestMCPImage(repositoryRoot, workDir)
	if err != nil {
		_ = app.cleanup()
		return nil, err
	}
	integrationMCPImage = app.mcpImage

	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel
	config := integrationServerConfig(httpPort, storagePort, workDir, app.mcpImage)
	go func() {
		app.done <- server.Run(ctx, config)
	}()

	if err := app.waitForHealth(baseURL, 2*time.Minute); err != nil {
		return nil, errors.Join(err, app.stop())
	}
	return app, nil
}

func integrationServerConfig(httpPort, storagePort int, workDir, mcpImage string) services.Config {
	config := services.Config{
		HTTPListenPort:           httpPort,
		DevMode:                  true,
		ElectionFile:             filepath.Join(workDir, "election"),
		MCPOAuthClientExpiration: "30d",
		DisableUpdateCheck:       true,
		MCPServerSearchImage:     mcpImage,
	}
	config.StorageListenPort = storagePort
	config.DSN = "sqlite://file:" + filepath.Join(workDir, "obot.db") + "?_journal=WAL&_busy_timeout=30000"
	config.DailyUserInputTokenLimit = -1
	config.DailyUserOutputTokenLimit = -1
	config.UnauthenticatedRateLimit = 100
	config.AuthenticatedRateLimit = 200
	config.AuditLogsMode = "off"
	config.MCPRuntimeBackend = "docker"
	config.MCPSecretBindingAllowedLabel = "obot.obot.ai/allow-secret-binding"
	config.SingleUserIdleServerShutdownHours = -1
	config.MultiUserIdleServerShutdownHours = -1
	config.IdleAgentShutdownHours = -1
	config.MCPAuditLogPersistIntervalSeconds = 5
	config.MCPAuditLogsPersistBatchSize = 1000
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

func configureDockerHost() (bool, error) {
	if os.Getenv("DOCKER_HOST") != "" {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
	if err != nil {
		return false, fmt.Errorf("inspect active Docker context: %w", err)
	}
	host := strings.TrimSpace(string(output))
	if host == "" {
		return false, errors.New("active Docker context has no endpoint")
	}
	if err := os.Setenv("DOCKER_HOST", host); err != nil {
		return false, err
	}
	return true, nil
}

func repositoryRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("determine integration test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("find repository root: %w", err)
	}
	return root, nil
}

func buildTestMCPImage(repositoryRoot, workDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	archOutput, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Arch}}").Output()
	if err != nil {
		return "", fmt.Errorf("determine Docker architecture: %w", err)
	}
	arch := strings.TrimSpace(string(archOutput))
	switch arch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported Docker architecture %q", arch)
	}

	binaryPath := filepath.Join(workDir, "mcp-test-server")
	build := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./tests/integration/testdata/mcpserver")
	build.Dir = repositoryRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
	if output, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build integration MCP server: %w\n%s", err, output)
	}

	image := "obot-integration-mcp:test"
	dockerfile := filepath.Join(repositoryRoot, "tests/integration/testdata/mcpserver/Dockerfile")
	buildImage := exec.CommandContext(ctx, "docker", "build", "--quiet", "--file", dockerfile, "--tag", image, workDir)
	if output, err := buildImage.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build integration MCP image: %w\n%s", err, output)
	}
	return image, nil
}

func removeTestMCPContainers(image string) error {
	if image == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	list := exec.CommandContext(ctx, "docker", "ps", "--all", "--quiet", "--filter", "ancestor="+image)
	output, err := list.Output()
	if err != nil {
		return fmt.Errorf("list integration MCP containers: %w", err)
	}
	if containers := strings.Fields(string(output)); len(containers) > 0 {
		args := append([]string{"rm", "--force"}, containers...)
		if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("remove integration MCP containers: %w\n%s", err, output)
		}
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
	result := removeTestMCPContainers(a.mcpImage)
	result = errors.Join(result, os.Unsetenv("OBOT_INTEGRATION_BASE_URL"))
	if a.dockerHostSet {
		result = errors.Join(result, os.Unsetenv("DOCKER_HOST"))
	}
	logrus.SetLevel(a.logLevel)
	result = errors.Join(result, os.Chdir(a.originalWorkDir))
	result = errors.Join(result, os.RemoveAll(a.workDir))
	return result
}
