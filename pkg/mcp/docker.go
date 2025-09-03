package mcp

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/filters"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	otypes "github.com/obot-platform/obot/apiclient/types"
)

type dockerBackend struct {
	client       *client.Client
	containerEnv bool
	baseImage    string
}

func newDockerBackend(baseImage string) (backend, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &dockerBackend{
		client:       cli,
		containerEnv: os.Getenv("OBOT_CONTAINER_ENV") == "true",
		baseImage:    baseImage,
	}, nil
}

func (d *dockerBackend) ensureServerDeployment(ctx context.Context, server ServerConfig, id, mcpServerDisplayName, _ string) (ServerConfig, error) {
	// Check if container already exists
	existing, err := d.getContainer(ctx, id)
	if err == nil && existing != nil {
		// Container exists, check if it's running
		if existing.State == "running" {
			// Return existing config
			containerPort := server.ContainerPort
			if containerPort == 0 {
				containerPort = defaultContainerPort
			}
			return d.buildServerConfig(server, existing, containerPort)
		}

		// Container exists but not running, remove it and recreate
		if err := d.client.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true}); err != nil {
			return ServerConfig{}, fmt.Errorf("failed to remove stopped container: %w", err)
		}
	}

	// Create new container
	return d.createAndStartContainer(ctx, server, id, mcpServerDisplayName)
}

func (d *dockerBackend) transformConfig(ctx context.Context, id string, serverConfig ServerConfig) (*ServerConfig, error) {
	existing, err := d.getContainer(ctx, id)
	if err != nil || existing == nil || existing.State != "running" {
		// Container doesn't exist or isn't running, config can't be transformed
		return nil, nil
	}

	containerPort := serverConfig.ContainerPort
	if containerPort == 0 {
		containerPort = defaultContainerPort
	}

	transformed, err := d.buildServerConfig(serverConfig, existing, containerPort)
	return &transformed, err
}

func (d *dockerBackend) streamServerLogs(ctx context.Context, id string) (io.ReadCloser, error) {
	logs, err := d.client.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Tail:       "100",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}

	return logs, nil
}

func (d *dockerBackend) getServerDetails(ctx context.Context, id string) (otypes.MCPServerDetails, error) {
	container, err := d.getContainer(ctx, id)
	if err != nil {
		return otypes.MCPServerDetails{}, fmt.Errorf("failed to get container: %w", err)
	}
	if container == nil {
		return otypes.MCPServerDetails{}, fmt.Errorf("mcp server %s is not running", id)
	}

	// Get container events for the last 24 hours
	since := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	eventFilters := filters.NewArgs()
	eventFilters.Add("container", container.ID)

	eventOptions := events.ListOptions{
		Since:   since,
		Filters: eventFilters,
	}

	eventMessages, errs := d.client.Events(ctx, eventOptions)
	var mcpEvents []otypes.MCPServerEvent

	// Collect events (but don't block if there are none)
	timeout := time.After(100 * time.Millisecond)
eventLoop:
	for {
		select {
		case event := <-eventMessages:
			mcpEvents = append(mcpEvents, otypes.MCPServerEvent{
				Time:         otypes.Time{Time: time.Unix(event.Time, event.TimeNano)},
				Reason:       string(event.Action),
				Message:      fmt.Sprintf("Container %s: %s", event.Actor.Attributes["name"], string(event.Action)),
				EventType:    string(event.Type),
				Action:       string(event.Action),
				Count:        1,
				ResourceName: id,
				ResourceKind: "Container",
			})
		case err := <-errs:
			if err != nil && err != io.EOF {
				log.Warnf("Error getting container events: %v", err)
			}
			break eventLoop
		case <-timeout:
			break eventLoop
		}
	}

	inspect, err := d.client.ContainerInspect(ctx, container.ID)
	if err != nil {
		return otypes.MCPServerDetails{}, fmt.Errorf("failed to inspect container: %w", err)
	}

	var lastRestart time.Time
	if inspect.State != nil && inspect.State.StartedAt != "" {
		lastRestart, err = time.Parse(time.RFC3339, inspect.State.StartedAt)
		if err != nil {
			return otypes.MCPServerDetails{}, fmt.Errorf("failed to parse container start time: %w", err)
		}
	} else {
		// Fallback to creation time
		lastRestart = time.Unix(container.Created, 0)
	}

	var readyReplicas int32
	if container.State == "running" {
		readyReplicas = 1
	}

	return otypes.MCPServerDetails{
		DeploymentName: id,
		Namespace:      "docker",
		LastRestart:    otypes.Time{Time: lastRestart},
		ReadyReplicas:  readyReplicas,
		Replicas:       1,
		IsAvailable:    container.State == "running",
		Events:         mcpEvents,
	}, nil
}

func (d *dockerBackend) restartServer(ctx context.Context, id string) error {
	if err := d.client.ContainerRestart(ctx, id, container.StopOptions{Timeout: &[]int{30}[0]}); err != nil {
		return fmt.Errorf("failed to restart container %s: %w", id, err)
	}

	return nil
}

func (d *dockerBackend) shutdownServer(ctx context.Context, id string) error {
	var volumeNames []string
	// Get container info to retrieve associated volumes for cleanup
	c, err := d.getContainer(ctx, id)
	if err == nil && c != nil {
		// Get container inspection to find volumes
		inspect, err := d.client.ContainerInspect(ctx, c.ID)
		if err == nil {
			// Clean up volumes associated with this container
			for _, mount := range inspect.Mounts {
				if mount.Type == "volume" {
					// Check if this is our volume based on labels
					volumeInspect, err := d.client.VolumeInspect(ctx, mount.Name)
					if err == nil {
						if serverID, exists := volumeInspect.Labels["mcp.server.id"]; exists && serverID == id {
							// This is our volume, remove it
							volumeNames = append(volumeNames, mount.Name)
						}
					}
				}
			}
		}
	}

	// Stop and remove the container
	if err := d.client.ContainerStop(ctx, id, container.StopOptions{Timeout: &[]int{30}[0]}); err != nil && !cerrdefs.IsNotFound(err) {
		// If container doesn't exist, that's fine
		return fmt.Errorf("failed to stop container %s: %w", id, err)
	}

	if err := d.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
		// If container doesn't exist, that's fine
		return fmt.Errorf("failed to remove container %s: %w", id, err)
	}

	for _, volumeName := range volumeNames {
		if err := d.client.VolumeRemove(ctx, volumeName, true); err != nil && !cerrdefs.IsNotFound(err) {
			log.Warnf("Failed to remove volume %s: %v", volumeName, err)
		}
	}

	return nil
}

// Helper methods

func (d *dockerBackend) getContainer(ctx context.Context, name string) (*container.Summary, error) {
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "name",
			Value: name,
		}),
	})
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		for _, containerName := range c.Names {
			if strings.TrimPrefix(containerName, "/") == name {
				return &c, nil
			}
		}
	}

	return nil, nil
}

func (d *dockerBackend) getHostPort(container *container.Summary, containerPort int) (int, error) {
	for _, port := range container.Ports {
		if port.PrivatePort == uint16(containerPort) && port.PublicPort != 0 {
			return int(port.PublicPort), nil
		}
	}
	return 0, fmt.Errorf("no public port found for container port %d", containerPort)
}

func (d *dockerBackend) buildServerConfig(server ServerConfig, c *container.Summary, port int) (ServerConfig, error) {
	var host string
	if d.containerEnv {
		if c == nil || c.NetworkSettings == nil {
			return ServerConfig{}, fmt.Errorf("container %s not found or has no network settings", c.ID)
		}

		n, ok := c.NetworkSettings.Networks["bridge"]
		if !ok || n.IPAddress == "" {
			return ServerConfig{}, fmt.Errorf("container %s is not connected to bridge network", c.ID)
		}

		host = n.IPAddress
	} else {
		host = "localhost"

		var err error
		port, err = d.getHostPort(c, port)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("failed to get host port: %w", err)
		}
	}

	url := fmt.Sprintf("http://%s:%d", host, port)
	if server.ContainerPath != "" {
		url = fmt.Sprintf("%s/%s", url, strings.TrimPrefix(server.ContainerPath, "/"))
	}

	return ServerConfig{
		URL:          url,
		Scope:        c.ID,
		AllowedTools: server.AllowedTools,
		Runtime:      otypes.RuntimeRemote,
	}, nil
}

func (d *dockerBackend) createAndStartContainer(ctx context.Context, server ServerConfig, containerName, displayName string) (retConfig ServerConfig, retErr error) {
	var (
		volumeMounts  []mount.Mount
		entrypoint    []string
		cmd           []string
		env           []string
		containerPort int
		image         string
	)

	// Prepare file volumes and environment variables
	fileVolumeName, fileEnvVars, err := d.prepareContainerFiles(ctx, server, containerName)
	if err != nil {
		return retConfig, fmt.Errorf("failed to prepare container files: %w", err)
	}
	if fileVolumeName != "" {
		volumeMounts = append(volumeMounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: fileVolumeName,
			Target: "/files",
		})
	}

	if len(fileEnvVars) > 0 {
		if server.Command != "" {
			server.Command = expandEnvVars(server.Command, fileEnvVars, nil)
		}
		if server.ContainerImage != "" {
			server.ContainerImage = expandEnvVars(server.ContainerImage, fileEnvVars, nil)
		}

		if len(server.Args) > 0 {
			// Copy the args to a new slice, expanding environment variables as needed.
			// We need a copy here so we don't modify the original server.Args slice.
			args := make([]string, len(server.Args))
			for i, arg := range server.Args {
				args[i] = expandEnvVars(arg, fileEnvVars, nil)
			}

			server.Args = args
		}
	}

	// Configure based on runtime
	switch server.Runtime {
	case otypes.RuntimeUVX, otypes.RuntimeNPX:
		// Use base image with nanobot
		image = d.baseImage
		containerPort = defaultContainerPort

		// Prepare nanobot configuration
		nanobotVolumeName, err := d.prepareNanobotConfig(ctx, server, displayName, fileEnvVars, containerName)
		if err != nil {
			return retConfig, fmt.Errorf("failed to prepare nanobot config: %w", err)
		}

		volumeMounts = append(volumeMounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: nanobotVolumeName,
			Target: "/run",
		})

		// Use nanobot command
		cmd = []string{"run", "--listen-address", fmt.Sprintf(":%d", defaultContainerPort), "/run/nanobot.yaml"}

		// Set nanobot environment variables
		env = []string{"NANOBOT_RUN_HEALTHZ_PATH=/healthz", "OBOT_KUBERNETES_MODE=true"}

	case otypes.RuntimeContainerized:
		// Use specified container image or base image
		if server.ContainerImage == "" {
			return retConfig, fmt.Errorf("container image must be specified for containerized runtime")
		}

		image = server.ContainerImage
		containerPort = server.ContainerPort

		// Use server's command and args
		if server.Command != "" {
			entrypoint = []string{server.Command}
		}
		cmd = server.Args

		// Use server's environment variables plus file env vars
		env = append(env, server.Env...)
		for k, v := range fileEnvVars {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

	default:
		return retConfig, fmt.Errorf("unsupported runtime: %s", server.Runtime)
	}

	// Prepare port binding
	containerPortStr := fmt.Sprintf("%d/tcp", containerPort)

	// Container config
	config := &container.Config{
		Image:        image,
		ExposedPorts: nat.PortSet{nat.Port(containerPortStr): struct{}{}},
		Env:          env,
		Entrypoint:   entrypoint,
		Cmd:          cmd,
		Labels: map[string]string{
			"mcp.server.name": displayName,
			"mcp.server.id":   containerName,
		},
	}

	// Host config with port bindings and volume mounts
	hostConfig := &container.HostConfig{
		PortBindings: map[nat.Port][]nat.PortBinding{nat.Port(containerPortStr): {{HostIP: "127.0.0.1"}}},
		Mounts:       volumeMounts,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	// Pull image if it doesn't exist locally
	if err := d.ensureImageExists(ctx, image); err != nil {
		return retConfig, fmt.Errorf("failed to ensure image exists: %w", err)
	}

	// Create container
	resp, err := d.client.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, containerName)
	if err != nil {
		return retConfig, fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return retConfig, fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for container to be running and healthy
	if err := d.waitForContainer(ctx, resp.ID); err != nil {
		return retConfig, fmt.Errorf("container failed to become ready: %w", err)
	}

	c, err := d.getContainer(ctx, containerName)
	if err != nil {
		return retConfig, fmt.Errorf("failed to get container after starting: %w", err)
	}

	var (
		host string
		port = containerPort
	)
	if d.containerEnv {
		if c == nil || c.NetworkSettings == nil {
			return retConfig, fmt.Errorf("container %s not found or has no network settings", containerName)
		}

		n, ok := c.NetworkSettings.Networks["bridge"]
		if !ok || n.IPAddress == "" {
			return retConfig, fmt.Errorf("container %s is not connected to bridge network", containerName)
		}

		host = n.IPAddress
	} else {
		port, err = d.getHostPort(c, containerPort)
		if err != nil {
			return retConfig, fmt.Errorf("failed to get host port: %w", err)
		}

		host = "localhost"
	}

	if err = ensureServerReady(ctx, fmt.Sprintf("http://%s:%d", host, port), server); err != nil {
		return retConfig, fmt.Errorf("server readiness check failed: %w", err)
	}

	return d.buildServerConfig(server, c, containerPort)
}

func (d *dockerBackend) waitForContainer(ctx context.Context, containerID string) error {
	// Wait up to 30 seconds for container to be running
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for container to start")
		case <-ticker.C:
			inspect, err := d.client.ContainerInspect(ctx, containerID)
			if err != nil {
				return fmt.Errorf("failed to inspect container: %w", err)
			}

			if inspect.State == nil {
				continue
			}

			if inspect.State.Running {
				// Container is running, optionally check health endpoint
				return nil
			}

			if inspect.State.Dead || inspect.State.OOMKilled || inspect.State.ExitCode != 0 {
				return fmt.Errorf("container failed to start: %s", inspect.State.Status)
			}
		}
	}
}

// prepareContainerFiles creates a volume for server.Files and returns volume name and env vars
func (d *dockerBackend) prepareContainerFiles(ctx context.Context, server ServerConfig, containerID string) (string, map[string]string, error) {
	if len(server.Files) == 0 {
		return "", nil, nil
	}

	volumeName, envVars, err := d.createVolumeWithFiles(ctx, server.Files, containerID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create volume with files: %w", err)
	}

	return volumeName, envVars, nil
}

// ensureImageExists checks if an image exists locally and pulls it if not
// createVolumeWithFiles creates an anonymous volume and populates it with file data using an init container
func (d *dockerBackend) createVolumeWithFiles(ctx context.Context, files []File, containerID string) (string, map[string]string, error) {
	if len(files) == 0 {
		return "", nil, nil
	}

	volumeName := containerID + "-files"

	// Create anonymous volume
	_, err := d.client.VolumeCreate(ctx, volume.CreateOptions{
		Labels: map[string]string{
			"mcp.server.id": containerID,
			"mcp.purpose":   "files",
		},
		Name: volumeName,
	})
	if err != nil && !cerrdefs.IsAlreadyExists(err) {
		return "", nil, fmt.Errorf("failed to create volume: %w", err)
	}

	// Create init container to populate the volume
	initImage := "alpine:latest"
	if err := d.ensureImageExists(ctx, initImage); err != nil {
		return "", nil, fmt.Errorf("failed to ensure init image exists: %w", err)
	}

	// Build script to create files in the volume
	var script strings.Builder
	script.WriteString("#!/bin/sh\nset -e\n")

	envVars := make(map[string]string, len(files))
	for _, file := range files {
		// Generate unique filename for container
		filename := fmt.Sprintf("%s-%s", containerID[:12], hash.Digest(file)[:24])
		containerPath := path.Join("/files", filename)

		// Add to script
		script.WriteString(fmt.Sprintf("cat > '%s' << 'EOF'\n%s\nEOF\n", containerPath, file.Data))

		// Set environment variable if specified
		if file.EnvKey != "" {
			envVars[file.EnvKey] = containerPath
		}
	}

	// Create and run init container
	initConfig := &container.Config{
		Image:      initImage,
		Entrypoint: []string{"sh", "-c"},
		Cmd:        []string{script.String()},
		WorkingDir: "/",
	}

	initHostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: "/files",
			},
		},
		AutoRemove: true,
	}

	var initContainerID string
	initContainerName := fmt.Sprintf("%s-init", containerID)
	resp, err := d.client.ContainerCreate(ctx, initConfig, initHostConfig, &network.NetworkingConfig{}, nil, initContainerName)
	if cerrdefs.IsAlreadyExists(err) {
		// Init container already exists, get its containerID
		resp, err := d.client.ContainerList(ctx, container.ListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("name", initContainerName),
			),
		})
		if err != nil {
			return "", nil, fmt.Errorf("failed to inspect nanobot init container: %w", err)
		}
		if len(resp) == 0 {
			return "", nil, fmt.Errorf("failed to find existing nanobot init container")
		}

		initContainerID = resp[0].ID
	} else if err != nil {
		return "", nil, fmt.Errorf("failed to create init container: %w", err)
	} else {
		initContainerID = resp.ID
		// Start and wait for init container to complete
		if err := d.client.ContainerStart(ctx, initContainerID, container.StartOptions{}); err != nil {
			return "", nil, fmt.Errorf("failed to start init container: %w", err)
		}
	}

	// Wait for init container to complete
	statusCh, errCh := d.client.ContainerWait(ctx, initContainerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		// It's OK if the container is already gone.
		if err != nil && !cerrdefs.IsNotFound(err) {
			return "", nil, fmt.Errorf("error waiting for init container: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return "", nil, fmt.Errorf("init container failed with exit code %d", status.StatusCode)
		}
	}

	return volumeName, envVars, nil
}

func (d *dockerBackend) ensureImageExists(ctx context.Context, imageName string) error {
	// Check if image exists locally
	_, err := d.client.ImageInspect(ctx, imageName)
	if err == nil {
		// Image exists locally
		return nil
	}

	reader, err := d.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Read the pull response to completion (required for the pull to actually happen)
	if _, err := io.ReadAll(reader); err != nil {
		return fmt.Errorf("failed to read image pull response: %w", err)
	}

	return nil
}

// prepareNanobotConfig creates a volume with nanobot YAML configuration for UVX/NPX runtimes
func (d *dockerBackend) prepareNanobotConfig(ctx context.Context, server ServerConfig, displayName string, envVars map[string]string, containerID string) (string, error) {
	// Create all environment variables map
	allEnvVars := make(map[string]string, len(server.Env)+len(envVars)+2)

	// Add server environment variables
	for _, env := range server.Env {
		if k, v, ok := strings.Cut(env, "="); ok {
			allEnvVars[k] = v
		}
	}

	maps.Copy(allEnvVars, envVars)

	// Add nanobot-specific environment variables
	allEnvVars["OBOT_KUBERNETES_MODE"] = "true"
	allEnvVars["NANOBOT_RUN_HEALTHZ_PATH"] = "/healthz"

	// Generate nanobot YAML using the existing function from loader.go
	nanobotYAML, err := constructNanobotYAML(displayName, server.Command, server.Args, allEnvVars)
	if err != nil {
		return "", fmt.Errorf("failed to construct nanobot YAML: %w", err)
	}

	volumeName := containerID + "-nanobot-config"
	// Create volume for nanobot config
	_, err = d.client.VolumeCreate(ctx, volume.CreateOptions{
		Labels: map[string]string{
			"mcp.server.id": containerID,
			"mcp.purpose":   "nanobot-config",
		},
		Name: volumeName,
	})
	if err != nil && !cerrdefs.IsAlreadyExists(err) {
		return "", fmt.Errorf("failed to create nanobot config volume: %w", err)
	}

	// Create init container to populate the volume with nanobot config
	initImage := "alpine:latest"
	if err = d.ensureImageExists(ctx, initImage); err != nil {
		return "", fmt.Errorf("failed to ensure init image exists: %w", err)
	}

	// Create script to write nanobot config
	script := fmt.Sprintf("cat > /run/nanobot.yaml << 'EOF'\n%s\nEOF\n", nanobotYAML)

	// Create and run init container
	initConfig := &container.Config{
		Image:      initImage,
		Entrypoint: []string{"sh", "-c"},
		Cmd:        []string{script},
	}

	initHostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: "/run",
			},
		},
		AutoRemove: true,
	}

	var initContainerID string
	initContainerName := fmt.Sprintf("%s-nanobot-init", containerID)
	resp, err := d.client.ContainerCreate(ctx, initConfig, initHostConfig, &network.NetworkingConfig{}, nil, initContainerName)
	if cerrdefs.IsAlreadyExists(err) {
		// Container already exists, so get its ID
		resp, err := d.client.ContainerList(ctx, container.ListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("name", initContainerName),
			),
		})
		if err != nil {
			return "", fmt.Errorf("failed to inspect nanobot init container: %w", err)
		}
		if len(resp) == 0 {
			return "", fmt.Errorf("failed to find existing nanobot init container")
		}

		initContainerID = resp[0].ID
	} else if err != nil {
		return "", fmt.Errorf("failed to create nanobot init container: %w", err)
	} else {
		initContainerID = resp.ID
		// Start and wait for init container to complete
		if err := d.client.ContainerStart(ctx, initContainerID, container.StartOptions{}); err != nil {
			return "", fmt.Errorf("failed to start init container: %w", err)
		}
	}

	// Wait for init container to complete
	statusCh, errCh := d.client.ContainerWait(ctx, initContainerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil && !cerrdefs.IsNotFound(err) {
			return "", fmt.Errorf("error waiting for nanobot init container: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return "", fmt.Errorf("nanobot init container failed with exit code %d", status.StatusCode)
		}
	}

	return volumeName, nil
}
