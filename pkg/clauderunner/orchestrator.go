//go:build !windows

package clauderunner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/client"
)

// Reserve ports until their child exits, including the gap before the CLI binds.
var (
	orchestratorHealthPorts sync.Map
)

// StartOrchestrator supervises Anthropic's CLI on every Obot replica. Anthropic
// claims work server-side; Obot's admission handler deduplicates retried orders.
func StartOrchestrator(ctx context.Context, key, token, environment, obotURL string) (func(), error) {
	binary, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("the Claude orchestrator requires the claude binary: %w", err)
	}

	if len(strings.TrimSpace(key)) == 0 || len(token) < 32 {
		return nil, fmt.Errorf("the Claude orchestrator requires an environment key and a provisioning token of at least 32 characters")
	}
	if _, err := os.Stat("/opt/obot/hooks/spawn-runner"); err != nil {
		return nil, fmt.Errorf("the Claude orchestrator requires the Obot spawn hook: %w", err)
	}

	home, err := os.MkdirTemp("", "obot-claude-orchestrator-")
	if err != nil {
		return nil, err
	}
	keyFile := filepath.Join(home, "environment-key")
	if err := os.WriteFile(keyFile, []byte(key), 0o600); err != nil {
		_ = os.RemoveAll(home)
		return nil, err
	}
	tokenFile := filepath.Join(home, "provisioning-token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		_ = os.RemoveAll(home)
		return nil, err
	}

	// Keep Obot's database/API credentials and user Claude config out of the child.
	env := []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "OBOT_URL=" + obotURL,
		"OBOT_PROVISIONING_TOKEN_FILE=" + tokenFile, "OBOT_CLAUDE_ENVIRONMENT=" + environment, "CLAUDE_CONFIG_DIR=" + home,
		"PYTHONDONTWRITEBYTECODE=1"}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = os.RemoveAll(home) }()

		runOrchestrator(ctx, binary, keyFile, home, environment, env)
	}()

	return func() {
		cancel()
		<-done
	}, nil
}

func runOrchestrator(ctx context.Context, binary, keyFile, home, environment string, env []string) {
	for ctx.Err() == nil {
		runOrchestratorProcess(ctx, binary, keyFile, home, environment, env)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func runOrchestratorProcess(ctx context.Context, binary, keyFile, home, environment string, env []string) {
	log := slog.With("environment", environment)
	var address string
	for {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			log.Error("Cannot allocate Claude orchestrator health port", "error", err)
			return
		}
		address = listener.Addr().String()
		_ = listener.Close()
		if _, reserved := orchestratorHealthPorts.LoadOrStore(address, true); !reserved {
			break
		}
	}
	defer orchestratorHealthPorts.Delete(address)
	_, port, _ := net.SplitHostPort(address)

	cmd := exec.Command(binary, "self-hosted-runner", "orchestrator",
		"--environment-secret-file", keyFile, "--hooks-dir", "/opt/obot/hooks",
		"--expected-spawn-seconds", "600", "--health-port", port)
	cmd.Env = env
	cmd.Dir = home
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		log.Error("Failed to start Claude orchestrator", "error", err)
	} else {
		log.Info("Started Claude orchestrator")
		wait := make(chan error, 1)
		go func() { wait <- cmd.Wait() }()

		watchCtx, cancelWatch := context.WithCancel(ctx)
		defer cancelWatch()
		unhealthy := make(chan error, 1)
		go func() {
			unhealthy <- watchOrchestrator(watchCtx, "http://"+address+"/healthz", 15*time.Second, 2*time.Minute)
		}()

		select {
		case err := <-wait:
			log.Warn("Claude orchestrator exited; restarting", "error", err)
		case err := <-unhealthy:
			if ctx.Err() == nil {
				log.Warn("Claude orchestrator polling stalled; restarting", "error", err)
			}
			stopOrchestrator(cmd, wait)
		case <-ctx.Done():
			stopOrchestrator(cmd, wait)
		}
		// A failed CLI may leave a hook behind. Reap its process group too.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

func stopOrchestrator(cmd *exec.Cmd, wait <-chan error) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-wait: //nolint:errcheck // Termination signals are expected to produce an exit error.
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-wait //nolint:errcheck // SIGKILL is expected to produce an exit error.
	}
}

// Only successful, advancing polls count as progress; a healthy HTTP listener alone does not.
func watchOrchestrator(ctx context.Context, endpoint string, interval, staleAfter time.Duration) error {
	client := &http.Client{
		Timeout:       2 * time.Second,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastProgress := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			poll, err := orchestratorLastPoll(ctx, client, endpoint)
			now := time.Now()
			if err == nil && poll.After(lastProgress) && !poll.After(now) {
				lastProgress = poll
			}
			if now.Sub(lastProgress) >= staleAfter {
				return fmt.Errorf("no successful polling progress for %s", staleAfter)
			}
		}
	}
}

func orchestratorLastPoll(ctx context.Context, client *http.Client, endpoint string) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return time.Time{}, err
	}
	response, err := client.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("health endpoint returned HTTP %d", response.StatusCode)
	}
	var health struct {
		Connected bool      `json:"connected"`
		LastPoll  time.Time `json:"last_poll_at"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&health); err != nil {
		return time.Time{}, err
	}
	if !health.Connected || health.LastPoll.IsZero() {
		return time.Time{}, fmt.Errorf("orchestrator has no successful poll")
	}
	return health.LastPoll, nil
}

// ManageOrchestrators refreshes database-backed environments on every replica.
// ponytail: polling every five seconds; add notifications if the environment count warrants it.
func ManageOrchestrators(ctx context.Context, gateway *client.Client, obotURL string) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		type running struct {
			revision string
			stop     func()
		}
		processes := map[string]running{}
		defer func() {
			for _, process := range processes {
				process.stop()
			}
		}()
		timer := time.NewTicker(5 * time.Second)
		defer timer.Stop()
		for ctx.Err() == nil {
			environments, err := gateway.ClaudeEnvironments(ctx)
			if err != nil {
				slog.ErrorContext(ctx, "Unable to load Claude environments", "error", err)
			} else {
				desired := map[string]bool{}
				for _, environment := range environments {
					desired[environment.ID] = true
					process, exists := processes[environment.ID]
					if exists && process.revision == environment.Revision {
						continue
					}
					if exists {
						process.stop()
						delete(processes, environment.ID)
					}
					credential, err := gateway.RevealCredential(ctx, []string{"claude-environment"}, environment.ID)
					if err != nil {
						slog.ErrorContext(ctx, "Unable to load Claude environment credential", "environment", environment.ID)
						continue
					}
					stop, err := StartOrchestrator(ctx, credential.Secrets["key"], credential.Secrets["token"], environment.ID, obotURL)
					if err != nil {
						slog.ErrorContext(ctx, "Unable to start Claude environment", "environment", environment.ID, "error", err)
						continue
					}
					processes[environment.ID] = running{revision: environment.Revision, stop: stop}
				}
				for id, process := range processes {
					if !desired[id] {
						process.stop()
						delete(processes, id)
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}()
	return sync.OnceFunc(func() { cancel(); <-done })
}
