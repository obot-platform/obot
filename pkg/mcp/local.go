package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/obot/apiclient/types"
)

type localBackend struct{}

func newLocalBackend() backend {
	return &localBackend{}
}

func (l *localBackend) deployServer(ctx context.Context, server ServerConfig, userID string, mcpServerDisplayName, mcpServerName string) error {
	// The local backend's ensureServerDeployment function doesn't wait for readiness, so we can just return its result
	_, err := l.ensureServerDeployment(ctx, server, userID, mcpServerDisplayName, mcpServerName)
	return err
}

func (*localBackend) ensureServerDeployment(_ context.Context, server ServerConfig, _, _, _ string) (ServerConfig, error) {
	if server.Runtime == types.RuntimeContainerized {
		// The containerized runtime is not supported when running servers locally.
		return ServerConfig{}, &ErrNotSupportedByBackend{Feature: "containerized runtime", Backend: "local"}
	}

	return transformFileEnvVars(server, server.Scope)
}

func (*localBackend) transformConfig(_ context.Context, server ServerConfig) (*ServerConfig, error) {
	server, err := transformFileEnvVars(server, server.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to transform file environment variables: %w", err)
	}
	return &server, nil
}

func (*localBackend) streamServerLogs(context.Context, string) (io.ReadCloser, error) {
	return nil, &ErrNotSupportedByBackend{Feature: "streaming logs", Backend: "local"}
}

func (*localBackend) getServerDetails(context.Context, string) (types.MCPServerDetails, error) {
	return types.MCPServerDetails{}, &ErrNotSupportedByBackend{Feature: "server details", Backend: "local"}
}

func (*localBackend) restartServer(context.Context, string) error {
	return &ErrNotSupportedByBackend{Feature: "restarting server", Backend: "local"}
}

func (*localBackend) shutdownServer(_ context.Context, id string) error {
	if err := os.RemoveAll(filepath.Join(os.TempDir(), id)); err != nil {
		return fmt.Errorf("failed to remove temporary directory for server %s: %w", id, err)
	}
	return nil
}

func transformFileEnvVars(server ServerConfig, id string) (ServerConfig, error) {
	if len(server.Files) > 0 {
		dir := filepath.Join(os.TempDir(), id)

		err := os.Mkdir(dir, 0755)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return ServerConfig{}, fmt.Errorf("failed to create directory for files: %w", err)
		}
		// Copy the env array so we don't modify the original.
		env := server.Env
		filenames := make(map[string]string, len(server.Files))
		for _, file := range server.Files {
			filenames[file.EnvKey] = filepath.Join(dir, hash.Digest(file))
			env = append(env, fmt.Sprintf("%s=%s", file.EnvKey, filenames[file.EnvKey]))
		}

		server.Env = env

		if err == nil {
			// We're creating the directory, so we need to create the files.
			// If the directory already exists, we assume the files are already there.
			for _, file := range server.Files {
				f, err := os.Create(filenames[file.EnvKey])
				if err != nil {
					return ServerConfig{}, fmt.Errorf("failed to create file for environment variable %s: %w", file.EnvKey, err)
				}

				if _, err = f.WriteString(file.Data); err != nil {
					f.Close()
					return ServerConfig{}, fmt.Errorf("failed to write data to file for environment variable %s: %w", file.EnvKey, err)
				}
				if err = f.Close(); err != nil {
					return ServerConfig{}, fmt.Errorf("failed to close file for environment variable %s: %w", file.EnvKey, err)
				}
			}
		}

		// Update the server config with the file paths.
		if server.Command != "" {
			server.Command = expandEnvVars(server.Command, filenames, nil)
		}

		args := make([]string, len(server.Args))
		for i, arg := range server.Args {
			args[i] = expandEnvVars(arg, filenames, nil)
		}
		server.Args = args
	}

	return server, nil
}
