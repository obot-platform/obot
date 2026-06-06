package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

	"github.com/obot-platform/nah/pkg/log"
	"github.com/obot-platform/nanobot/pkg/system"
)

type ports struct {
	daemonPorts    map[string]int64
	daemonsRunning map[string]func()
	daemonLock     sync.RWMutex

	startPort, endPort int64
	usedPorts          map[int64]struct{}
	daemonCtx          context.Context
	daemonClose        func()
	daemonWG           sync.WaitGroup
}

func newPorts() *ports {
	daemonCtx, cancel := context.WithCancel(context.Background())
	p := &ports{
		daemonCtx:      daemonCtx,
		daemonPorts:    map[string]int64{},
		daemonsRunning: map[string]func(){},
		usedPorts:      map[int64]struct{}{},
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

	if stop := d.ports.daemonsRunning[id]; stop != nil {
		stop()
	}

	delete(d.ports.daemonsRunning, id)
	delete(d.ports.usedPorts, d.ports.daemonPorts[id])
	delete(d.ports.daemonPorts, id)
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

func (d *Dispatcher) startDaemon(env map[string]string, id, command string, args ...string) (url.URL, error) {
	d.ports.daemonLock.RLock()
	port, portExists := d.ports.daemonPorts[id]
	_, isRunning := d.ports.daemonsRunning[id]
	d.ports.daemonLock.RUnlock()

	u := url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	if portExists && isRunning {
		return u, nil
	}

	d.ports.daemonLock.Lock()
	defer d.ports.daemonLock.Unlock()

	port, portExists = d.ports.daemonPorts[id]
	_, isRunning = d.ports.daemonsRunning[id]
	if portExists && isRunning {
		return url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}, nil
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

	log.Infof("launched [%s][%v] port [%d]", command, cmd.Args, port)
	if err := cmd.Start(); err != nil {
		stop()
		delete(d.ports.usedPorts, port)
		return u, err
	}

	d.ports.daemonPorts[id] = port
	d.ports.daemonsRunning[id] = stop

	killedCtx, killedCancel := context.WithCancelCause(ctx)
	defer killedCancel(nil)

	d.ports.daemonWG.Go(func() {
		err := cmd.Wait()
		if err != nil {
			log.Debugf("daemon exited tool [%s] %v: %v", command, cmd.Args, err)
		}

		killedCancel(err)
		stop()

		d.ports.daemonLock.Lock()
		defer d.ports.daemonLock.Unlock()

		delete(d.ports.usedPorts, port)
		delete(d.ports.daemonPorts, id)
		delete(d.ports.daemonsRunning, id)
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

	log.Infof("launched [%s][%v]", command, cmd.Args)
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
