package dispatcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/system"
)

type ports struct {
	daemonPorts            map[string]int64
	daemonsRunning         map[string]func()
	daemonCommands         map[string]*exec.Cmd
	daemonRevisions        map[string]daemonRevision
	desiredDaemonRevisions map[string]daemonRevision
	daemonLock             sync.RWMutex

	startPort, endPort int64
	usedPorts          map[int64]struct{}
	daemonCtx          context.Context
	daemonClose        func()
	daemonWG           sync.WaitGroup
}

type daemonRevision struct {
	instance   string
	generation int64
	value      string
}

func (d daemonRevision) sameConfiguration(other daemonRevision) bool {
	return d.instance == other.instance && d.value == other.value
}

var errDaemonRevisionChanged = errors.New("daemon revision changed")

func newPorts() *ports {
	daemonCtx, cancel := context.WithCancel(context.Background())
	p := &ports{
		daemonCtx:              daemonCtx,
		daemonPorts:            map[string]int64{},
		daemonsRunning:         map[string]func(){},
		daemonCommands:         map[string]*exec.Cmd{},
		daemonRevisions:        map[string]daemonRevision{},
		desiredDaemonRevisions: map[string]daemonRevision{},
		usedPorts:              map[int64]struct{}{},
	}
	p.daemonClose = func() {
		cancel()
		p.daemonCtx = nil
	}

	return p
}

func (d *Dispatcher) closeDaemons() {
	d.ports.daemonClose()
	d.ports.daemonWG.Wait()
}

func (d *Dispatcher) stopDaemon(id string) {
	d.ports.daemonLock.Lock()
	defer d.ports.daemonLock.Unlock()
	d.stopDaemonLocked(id)
}

func (d *Dispatcher) stopDaemonLocked(id string) {
	if stop := d.ports.daemonsRunning[id]; stop != nil {
		stop()
	}

	delete(d.ports.daemonsRunning, id)
	delete(d.ports.daemonCommands, id)
	delete(d.ports.daemonRevisions, id)
	delete(d.ports.daemonPorts, id)
}

func (d *Dispatcher) observeDaemonRevision(id string, revision daemonRevision) bool {
	d.ports.daemonLock.Lock()
	defer d.ports.daemonLock.Unlock()

	if desired, ok := d.ports.desiredDaemonRevisions[id]; ok &&
		desired.instance == revision.instance && desired.generation > revision.generation {
		return desired.sameConfiguration(revision)
	}
	d.ports.desiredDaemonRevisions[id] = revision

	if runningRevision, ok := d.ports.daemonRevisions[id]; ok {
		if !runningRevision.sameConfiguration(revision) {
			d.stopDaemonLocked(id)
		} else {
			d.ports.daemonRevisions[id] = revision
		}
	}
	return true
}

func (d *Dispatcher) forgetDaemon(id string) {
	d.ports.daemonLock.Lock()
	defer d.ports.daemonLock.Unlock()

	d.stopDaemonLocked(id)
	delete(d.ports.desiredDaemonRevisions, id)
}

func (d *Dispatcher) daemonURL(id string, revision daemonRevision) (url.URL, bool) {
	d.ports.daemonLock.RLock()
	defer d.ports.daemonLock.RUnlock()

	port := d.ports.daemonPorts[id]
	_, running := d.ports.daemonsRunning[id]
	return url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)},
		port != 0 && running && d.ports.daemonRevisions[id].sameConfiguration(revision)
}

func (d *Dispatcher) nextPort() int64 {
	if d.ports.startPort == 0 {
		d.ports.startPort = 10240
		d.ports.endPort = 11240
	}
	// This is pretty simple and inefficient approach, but also never releases ports
	count := d.ports.endPort - d.ports.startPort + 1
	toTry := make([]int64, 0, count)
	for i := d.ports.startPort; i <= d.ports.endPort; i++ {
		toTry = append(toTry, i)
	}

	rand.Shuffle(len(toTry), func(i, j int) {
		toTry[i], toTry[j] = toTry[j], toTry[i]
	})

	for _, nextPort := range toTry {
		if _, ok := d.ports.usedPorts[nextPort]; ok {
			continue
		}
		d.ports.usedPorts[nextPort] = struct{}{}
		return nextPort
	}

	panic("Ran out of usable ports")
}

func (d *Dispatcher) startDaemon(env map[string]string, id string, revision daemonRevision, command string, args ...string) (url.URL, error) {
	d.ports.daemonLock.RLock()
	port, portExists := d.ports.daemonPorts[id]
	_, isRunning := d.ports.daemonsRunning[id]
	runningRevision := d.ports.daemonRevisions[id]
	desiredRevision, hasDesiredRevision := d.ports.desiredDaemonRevisions[id]
	d.ports.daemonLock.RUnlock()

	u := url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	if portExists && isRunning && runningRevision.sameConfiguration(revision) &&
		(!hasDesiredRevision || desiredRevision.sameConfiguration(revision)) {
		return u, nil
	}

	d.ports.daemonLock.Lock()
	defer d.ports.daemonLock.Unlock()

	port, portExists = d.ports.daemonPorts[id]
	_, isRunning = d.ports.daemonsRunning[id]
	runningRevision = d.ports.daemonRevisions[id]
	if desired, ok := d.ports.desiredDaemonRevisions[id]; ok {
		if !desired.sameConfiguration(revision) {
			return u, errDaemonRevisionChanged
		}
		revision = desired
	}
	if portExists && isRunning && runningRevision.sameConfiguration(revision) {
		return url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}, nil
	}
	if portExists || isRunning {
		d.stopDaemonLocked(id)
	}

	ctx := d.ports.daemonCtx
	port = d.nextPort()
	u.Host = fmt.Sprintf("127.0.0.1:%d", port)

	if env == nil {
		env = make(map[string]string, 3)
	}
	env["PORT"] = fmt.Sprintf("%d", port)
	cmd, stop, err := d.newCommand(ctx, env, command, args...)
	if err != nil {
		return u, err
	}

	slog.Info("Launched provider daemon", "command", command, "args", cmd.Args, "port", port)
	if err := cmd.Start(); err != nil {
		stop()
		delete(d.ports.usedPorts, port)
		return u, err
	}

	d.ports.daemonPorts[id] = port
	d.ports.daemonsRunning[id] = stop
	d.ports.daemonCommands[id] = cmd
	d.ports.daemonRevisions[id] = revision

	killedCtx, killedCancel := context.WithCancelCause(ctx)
	defer killedCancel(nil)

	d.ports.daemonWG.Go(func() {
		err := cmd.Wait()
		if err != nil {
			slog.Debug("Provider daemon exited", "command", command, "args", cmd.Args, "error", err)
		}

		killedCancel(err)
		stop()

		d.ports.daemonLock.Lock()
		defer d.ports.daemonLock.Unlock()

		delete(d.ports.usedPorts, port)
		if d.ports.daemonCommands[id] != cmd {
			return
		}
		delete(d.ports.daemonPorts, id)
		delete(d.ports.daemonsRunning, id)
		delete(d.ports.daemonCommands, id)
		delete(d.ports.daemonRevisions, id)
	})

	client := &http.Client{Timeout: 2 * time.Second}

	for range 120 {
		resp, err := client.Get(u.String())
		if err == nil {
			go func(body io.ReadCloser) {
				_, _ = io.ReadAll(body)
				_ = body.Close()
			}(resp.Body)

			if resp.StatusCode == http.StatusOK {
				return u, nil
			}
		}
		select {
		case <-killedCtx.Done():
			return u, fmt.Errorf("daemon failed to start: %w", context.Cause(killedCtx))
		case <-time.After(time.Second):
		}
	}

	return u, fmt.Errorf("timeout waiting for 200 response from GET %s", u.String())
}

func (d *Dispatcher) runCommand(ctx context.Context, envMap map[string]string, command string, args ...string) error {
	cmd, stop, err := d.newCommand(ctx, envMap, command, args...)
	if err != nil {
		return err
	}
	defer stop()

	var stdOutAndErr bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdOutAndErr)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stdOutAndErr)

	slog.Info("Launched provider command", "command", command, "args", cmd.Args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ERROR: %v: %w", stdOutAndErr.String(), err)
	}

	return nil
}

func (d *Dispatcher) newCommand(ctx context.Context, envMap map[string]string, command string, args ...string) (*exec.Cmd, func(), error) {
	ctx, cancel := context.WithCancel(ctx)

	// Expand and/or normalize env references
	for i, arg := range args {
		args[i] = os.Expand(arg, func(s string) string {
			return envMap[s]
		})
	}

	if runtime.GOOS == "windows" {
		command = strings.ReplaceAll(command, "/", "\\")
	}

	// Loop back to obot to help with process supervision
	cmd := exec.CommandContext(ctx, system.Bin(), append([]string{"daemon", command}, args...)...)

	if envMap == nil {
		envMap = make(map[string]string, 2)
	}
	envMap["OBOT_SERVER_PUBLIC_URL"] = d.serverURL
	envMap["OBOT_SERVER_URL"] = d.sessionManager.TransformObotHostname(d.internalServerURL)
	cmd.Env = envAsSlice(envMap)

	r, w, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, nil, err
	}

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = r
	cmd.Cancel = func() error {
		_ = r.Close()
		return w.Close()
	}

	stop := func() {
		cancel()
	}

	return cmd, stop, nil
}

func envAsSlice(env map[string]string) []string {
	keys := slices.Collect(maps.Keys(env))
	slices.Sort(keys)

	sortedEnv := make([]string, len(env))
	for i, key := range keys {
		sortedEnv[i] = fmt.Sprintf("%s=%s", strings.ToUpper(toEnvLike(key)), env[key])
	}

	return sortedEnv
}

func toEnvLike(v string) string {
	v = strings.ReplaceAll(v, ".", "_")
	v = strings.ReplaceAll(v, "-", "_")
	return strings.ToUpper(v)
}
