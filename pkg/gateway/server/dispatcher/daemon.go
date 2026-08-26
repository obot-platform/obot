package dispatcher

import (
	"bytes"
	"context"
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
	daemons    map[string]daemonState
	daemonLock sync.RWMutex

	startPort, endPort int64
	usedPorts          map[int64]struct{}
	daemonCtx          context.Context
	daemonClose        func()
	daemonWG           sync.WaitGroup
}

type daemonState struct {
	port              int64
	stop              func()
	command           *exec.Cmd
	configurationHash string
}

func newPorts() *ports {
	daemonCtx, cancel := context.WithCancel(context.Background())
	p := &ports{
		daemonCtx: daemonCtx,
		daemons:   map[string]daemonState{},
		usedPorts: map[int64]struct{}{},
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
	state, ok := d.ports.daemons[id]
	if ok {
		delete(d.ports.daemons, id)
	}
	d.ports.daemonLock.Unlock()

	if ok {
		state.stop()
	}
}

func (d *Dispatcher) stopDaemonCommand(id string, command *exec.Cmd) {
	d.ports.daemonLock.Lock()
	state, ok := d.ports.daemons[id]
	if ok && state.command == command {
		delete(d.ports.daemons, id)
	} else {
		ok = false
	}
	d.ports.daemonLock.Unlock()

	if ok {
		state.stop()
	}
}

func (d *Dispatcher) daemonState(id string) (daemonState, bool) {
	d.ports.daemonLock.RLock()
	defer d.ports.daemonLock.RUnlock()

	state, ok := d.ports.daemons[id]
	return state, ok
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

func (d *Dispatcher) startDaemon(env map[string]string, id, configurationHash, command string, args ...string) (url.URL, *exec.Cmd, error) {
	d.ports.daemonLock.Lock()
	ctx := d.ports.daemonCtx
	port := d.nextPort()
	d.ports.daemonLock.Unlock()
	u := url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}

	if env == nil {
		env = make(map[string]string, 3)
	}
	env["PORT"] = fmt.Sprintf("%d", port)
	cmd, stop, err := d.newCommand(ctx, env, command, args...)
	if err != nil {
		d.ports.daemonLock.Lock()
		delete(d.ports.usedPorts, port)
		d.ports.daemonLock.Unlock()
		return u, nil, err
	}

	slog.Info("Launched provider daemon", "command", command, "args", cmd.Args, "port", port)
	if err := cmd.Start(); err != nil {
		stop()
		d.ports.daemonLock.Lock()
		delete(d.ports.usedPorts, port)
		d.ports.daemonLock.Unlock()
		return u, nil, err
	}

	d.ports.daemonLock.Lock()
	d.ports.daemons[id] = daemonState{
		port:              port,
		stop:              stop,
		command:           cmd,
		configurationHash: configurationHash,
	}
	d.ports.daemonLock.Unlock()

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
		if state, ok := d.ports.daemons[id]; ok && state.command == cmd {
			delete(d.ports.daemons, id)
		}
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
				return u, cmd, nil
			}
		}
		select {
		case <-killedCtx.Done():
			d.stopDaemonCommand(id, cmd)
			return u, nil, fmt.Errorf("daemon failed to start: %w", context.Cause(killedCtx))
		case <-time.After(time.Second):
		}
	}

	d.stopDaemonCommand(id, cmd)
	return u, nil, fmt.Errorf("timeout waiting for 200 response from GET %s", u.String())
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
	internalServerURL := d.internalServerURL
	if d.sessionManager != nil {
		internalServerURL = d.sessionManager.TransformObotHostname(internalServerURL)
	}
	envMap["OBOT_SERVER_URL"] = internalServerURL
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
