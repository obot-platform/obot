//go:build e2e

// Package harness runs real Obot server processes, with their Postgres containers
// and in-process mock upstreams, for end-to-end tests.
package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// Container is a docker container started for a test run.
type Container struct {
	Name string
	// HostPort is the host port the container's service port is published on (127.0.0.1).
	HostPort int
}

// Postgres is a Postgres container.
type Postgres struct {
	*Container
	Password string
}

// RandomSuffix returns a short random hex string for unique names.
func RandomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func docker(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// StartContainer runs image detached as name, publishing containerPort on a random
// 127.0.0.1 port.
func StartContainer(ctx context.Context, name, image string, containerPort int, env map[string]string, args ...string) (*Container, error) {
	runArgs := []string{"run", "-d", "--name", name, "--label", "obot-e2e=true", "-p", fmt.Sprintf("127.0.0.1::%d", containerPort)}
	for k, v := range env {
		runArgs = append(runArgs, "-e", k+"="+v)
	}
	runArgs = append(runArgs, image)
	runArgs = append(runArgs, args...)
	if _, err := docker(ctx, runArgs...); err != nil {
		return nil, err
	}
	out, err := docker(ctx, "port", name, fmt.Sprintf("%d/tcp", containerPort))
	if err != nil {
		_ = RemoveContainer(name)
		return nil, err
	}
	// Output is like "127.0.0.1:49153"; take the first line.
	line, _, _ := strings.Cut(out, "\n")
	_, portStr, err := net.SplitHostPort(strings.TrimSpace(line))
	if err != nil {
		_ = RemoveContainer(name)
		return nil, fmt.Errorf("parse docker port output %q: %w", out, err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		_ = RemoveContainer(name)
		return nil, err
	}
	return &Container{Name: name, HostPort: port}, nil
}

// RemoveContainer force-removes a container started by the harness.
func RemoveContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := docker(ctx, "rm", "-f", "-v", name)
	return err
}

// StartPostgres starts a Postgres container and waits until it accepts connections.
func StartPostgres(ctx context.Context, name string) (*Postgres, error) {
	c, err := StartContainer(ctx, name, "postgres:17-alpine", 5432, map[string]string{
		"POSTGRES_PASSWORD": "e2e",
		"POSTGRES_DB":       "postgres",
	})
	if err != nil {
		return nil, err
	}
	p := &Postgres{Container: c, Password: "e2e"}
	deadline := time.Now().Add(90 * time.Second)
	for {
		// pg_isready reports ready during the image's init restart, so check with a query
		// over TCP instead.
		if _, err := docker(ctx, "exec", name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-c", "select 1"); err == nil {
			return p, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("postgres %s did not become ready", name)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// CreateDatabase creates a database in the container.
func (p *Postgres) CreateDatabase(ctx context.Context, db string) error {
	_, err := docker(ctx, "exec", p.Name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-c", "CREATE DATABASE "+db)
	return err
}

// Exec runs SQL in db and returns psql's unaligned, tuples-only output.
func (p *Postgres) Exec(ctx context.Context, db, sql string) (string, error) {
	return docker(ctx, "exec", p.Name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", db, "-v", "ON_ERROR_STOP=1", "-At", "-c", sql)
}

// DSN returns the DSN of db for a process on the host.
func (p *Postgres) DSN(db string) string {
	return fmt.Sprintf("postgres://postgres:%s@127.0.0.1:%d/%s?sslmode=disable", p.Password, p.HostPort, db)
}
