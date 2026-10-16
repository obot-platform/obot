//go:build e2e

package harness

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// AdminEmail is the initial local-auth owner of every cluster.
	AdminEmail = "admin@e2e.example.com"
	// AdminPassword is the owner's password.
	AdminPassword = "E2e-Admin-Passw0rd!"
)

// ClusterOptions configures a test cluster.
type ClusterOptions struct {
	// Name prefixes container names and the cluster directory.
	Name string
	// Dir holds replica working directories and logs.
	Dir string
	// PretendContainer sets FakeDocker.PretendContainer.
	PretendContainer bool
}

// Cluster is a set of Obot replicas sharing one Postgres database.
type Cluster struct {
	Name       string
	Dir        string
	PG         *Postgres
	Docker     *FakeDocker
	DB         string
	SetupToken string

	mu       sync.Mutex
	replicas []*Replica
}

// NewCluster starts a Postgres container and the fake docker API. Replicas are
// started with StartReplica.
func NewCluster(ctx context.Context, opts ClusterOptions) (*Cluster, error) {
	c := &Cluster{
		Name:       opts.Name,
		Dir:        opts.Dir,
		DB:         "obot",
		SetupToken: "e2e-setup-token-" + RandomSuffix() + RandomSuffix() + RandomSuffix() + RandomSuffix(),
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return nil, err
	}
	var err error
	if c.Docker, err = StartFakeDocker(); err != nil {
		return nil, err
	}
	c.Docker.PretendContainer = opts.PretendContainer
	if c.PG, err = StartPostgres(ctx, opts.Name+"-pg"); err != nil {
		c.Close()
		return nil, err
	}
	if err := c.PG.CreateDatabase(ctx, c.DB); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// ReplicaArgs are flags every replica gets: authentication with the initial owner, remote
// MCP servers on localhost allowed and the fake docker API.
func (c *Cluster) replicaConfig(name, binary string, args []string, env map[string]string) ReplicaConfig {
	e := map[string]string{
		"OBOT_SERVER_ENABLE_AUTHENTICATION":                "true",
		"OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL":       AdminEmail,
		"OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN": c.SetupToken,
		"DOCKER_HOST": c.Docker.Host(),
		"OBOT_SERVER_MCPAUDIT_LOG_PERSIST_INTERVAL_SECONDS": "1",
	}
	maps.Copy(e, env)
	a := []string{"--disallow-localhost-mcp=false", "--disallow-private-ipmcp=false"}
	a = append(a, args...)
	return ReplicaConfig{
		Name:   name,
		Binary: binary,
		Dir:    filepath.Join(c.Dir, name),
		DSN:    c.PG.DSN(c.DB),
		Env:    e,
		Args:   a,
	}
}

// StartReplica starts a replica of binary and waits for it to be healthy.
func (c *Cluster) StartReplica(ctx context.Context, name, binary string, args []string, env map[string]string) (*Replica, error) {
	r, err := c.StartReplicaNoWait(name, binary, args, env)
	if err != nil {
		return nil, err
	}
	return r, r.WaitHealthy(ctx, 3*time.Minute)
}

// StartReplicaNoWait starts a replica without waiting for it.
func (c *Cluster) StartReplicaNoWait(name, binary string, args []string, env map[string]string) (*Replica, error) {
	r, err := StartReplica(c.replicaConfig(name, binary, args, env))
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.replicas = append(c.replicas, r)
	c.mu.Unlock()
	return r, nil
}

// ActivateAdmin sets the owner's password through replica r and logs in.
func (c *Cluster) ActivateAdmin(ctx context.Context, r *Replica) (*Client, error) {
	if err := ActivateOwner(ctx, r.BaseURL, c.SetupToken, AdminPassword); err != nil {
		return nil, err
	}
	return Login(ctx, r.BaseURL, AdminEmail, AdminPassword)
}

// Close stops every replica, removes the containers, and stops the fake docker API. Data
// directories are left for inspection.
func (c *Cluster) Close() error {
	var errs []error
	c.mu.Lock()
	replicas := c.replicas
	c.mu.Unlock()
	for _, r := range replicas {
		if err := r.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", r.Config.Name, err))
		}
	}
	if c.PG != nil {
		errs = append(errs, RemoveContainer(c.PG.Name))
	}
	if c.Docker != nil {
		c.Docker.Close()
	}
	return errors.Join(errs...)
}

// PanicLines returns lines of a replica log that show a crash.
func PanicLines(log string) []string {
	var out []string
	for line := range strings.SplitSeq(log, "\n") {
		if strings.Contains(line, "panic:") || strings.Contains(line, "fatal error:") || strings.Contains(line, "goroutine ") && strings.Contains(line, "[running]") {
			out = append(out, line)
		}
	}
	return out
}
