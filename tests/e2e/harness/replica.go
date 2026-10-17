//go:build e2e

package harness

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
	"sync"
	"syscall"
	"time"
)

// ReplicaConfig describes one Obot server process.
type ReplicaConfig struct {
	// Name labels the replica in logs and its directory name.
	Name string
	// Binary is the obot executable.
	Binary string
	// Dir is the replica's working directory; its data dir and log live under it.
	Dir string
	// DSN is Obot's database.
	DSN string
	// Env is added to the inherited environment.
	Env map[string]string
	// Args are added after the standard server flags.
	Args []string
}

// Replica is a running Obot server process.
type Replica struct {
	Config  ReplicaConfig
	Port    int
	BaseURL string
	LogPath string

	cmd     *exec.Cmd
	logFile *os.File
	done    chan struct{}
	waitErr error
	once    sync.Once
}

// FreePort returns a free localhost TCP port.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// PrepareBinary copies the obot binary into dir under a name other than "obot", so that
// tooling that signals processes named "obot" on this machine can't stop the test replicas,
// and so rebuilding the source binary mid-run doesn't affect them.
func PrepareBinary(src, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "obot-e2e-replica")
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("open obot binary %s (build it with make build, or set OBOT_E2E_BINARY): %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return "", err
	}
	return dst, out.Close()
}

// StartReplica starts an Obot server and returns once its process is running. Call
// WaitHealthy to wait for it to serve.
func StartReplica(cfg ReplicaConfig) (*Replica, error) {
	port, err := FreePort()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(cfg.Dir, "obot.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	args := []string{
		"server",
		"--http-listen-port", fmt.Sprint(port),
		"--disable-update-check",
		"--unauthenticated-rate-limit", "100000",
		"--authenticated-rate-limit", "100000",
	}
	args = append(args, cfg.Args...)

	cmd := exec.Command(cfg.Binary, args...)
	cmd.Dir = cfg.Dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		"XDG_DATA_HOME="+filepath.Join(cfg.Dir, "data"),
		"OBOT_SERVER_DSN="+cfg.DSN,
	)
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	r := &Replica{
		Config:  cfg,
		Port:    port,
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		LogPath: logPath,
		cmd:     cmd,
		logFile: logFile,
		done:    make(chan struct{}),
	}
	go func() {
		r.waitErr = cmd.Wait()
		close(r.done)
	}()
	return r, nil
}

// PID is the replica's process ID.
func (r *Replica) PID() int {
	return r.cmd.Process.Pid
}

// Exited reports whether the process has exited.
func (r *Replica) Exited() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// WaitHealthy waits for /api/healthz to answer 200.
func (r *Replica) WaitHealthy(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	check := func(path, want string) bool {
		resp, err := client.Get(r.BaseURL + path)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode == http.StatusOK && (want == "" || strings.Contains(string(body), want))
	}
	for {
		if r.Exited() {
			return fmt.Errorf("replica %s exited before becoming healthy (%v); log: %s\n%s", r.Config.Name, r.waitErr, r.LogPath, r.LogTail(40))
		}
		// The gateway's status needs authentication; callers check it with a client.
		if check("/api/healthz", "") {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("replica %s did not become healthy: %w; log tail:\n%s", r.Config.Name, ctx.Err(), r.LogTail(40))
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// LogTail returns the last n lines of the replica's log.
func (r *Replica) LogTail(n int) string {
	data, err := os.ReadFile(r.LogPath)
	if err != nil {
		return err.Error()
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Log returns the replica's whole log.
func (r *Replica) Log() string {
	data, _ := os.ReadFile(r.LogPath)
	return string(data)
}

// Stop terminates the replica (SIGTERM, then SIGKILL after a grace period) and every
// process it started. It only signals the process group this harness created.
func (r *Replica) Stop() error {
	var err error
	r.once.Do(func() {
		if r.Exited() {
			_ = r.logFile.Close()
			return
		}
		pgid := r.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		select {
		case <-r.done:
		case <-time.After(20 * time.Second):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			select {
			case <-r.done:
			case <-time.After(10 * time.Second):
				err = errors.New("replica did not exit after SIGKILL")
			}
		}
		_ = r.logFile.Close()
	})
	return err
}
