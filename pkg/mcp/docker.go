package mcp

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
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

var localhostURLRegexp = regexp.MustCompile(`^http://localhost(:\d+)?`)

type dockerBackend struct {
	client                        *client.Client
	containerEnv                  bool
	network                       string
	hostBaseURL                   string
	hostBaseURLWithPort           string
	containerizedBaseImage        string
	webhookBaseImage              string
	remoteShimBaseImage           string
	auditLogsBatchSize            int
	auditLogsFlushIntervalSeconds int
}

func newDockerBackend(ctx context.Context, exposedPort int, opts Options) (backend, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	var (
		host         string
		containerEnv = os.Getenv("OBOT_CONTAINER_ENV") == "true"
		network      = "bridge"
	)
	if containerEnv {
		network, host, err = detectContainerCurrentNetworkIP(ctx, cli)
		if err != nil {
			return nil, fmt.Errorf("failed to detect current IP: %w", err)
		}
	} else {
		host, err = detectCurrentLocalIP()
		if err != nil {
			return nil, fmt.Errorf("failed to detect current IP: %w", err)
		}
	}

	d := &dockerBackend{
		client:                        cli,
		containerEnv:                  containerEnv,
		network:                       network,
		hostBaseURL:                   "http://" + host,
		hostBaseURLWithPort:           "http://" + fmt.Sprintf("%s:%d", host, exposedPort),
		containerizedBaseImage:        opts.MCPBaseImage,
		webhookBaseImage:              opts.MCPHTTPWebhookBaseImage,
		remoteShimBaseImage:           opts.MCPRemoteShimBaseImage,
		auditLogsBatchSize:            opts.MCPAuditLogsPersistBatchSize,
		auditLogsFlushIntervalSeconds: opts.MCPAuditLogPersistIntervalSeconds,
	}
	if err = d.cleanupContainersWithOldID(ctx); err != nil {
		return nil, fmt.Errorf("failed to cleanup containers with old ID: %w", err)
	}

	return d, nil
}

// detectContainerCurrentNetworkIP detects the Docker network and IP of the current container if running inside one.
// Returns empty string if not running in a container or if detection fails.
func detectContainerCurrentNetworkIP(ctx context.Context, cli *client.Client) (string, string, error) {
	// Read container ID from cgroup file
	hostname, err := os.Hostname()
	if err != nil {
		return "", "", fmt.Errorf("failed to get hostname: %w", err)
	}

	// Try to inspect container using hostname as container ID
	inspect, err := cli.ContainerInspect(ctx, hostname)
	if err != nil {
		// Not running in a container or can't access Docker socket
		return "", "", fmt.Errorf("failed to inspect container: %w", err)
	}

	// Get the first network (most containers are on a single network)
	for networkName, networkSettings := range inspect.NetworkSettings.Networks {
		return networkName, networkSettings.IPAddress, nil
	}

	return "bridge", "", nil
}

// detectCurrentLocalIP detects the local IP.
func detectCurrentLocalIP() (string, error) {
	// Use UDP dial to determine the source IP address that would be used to reach an external IP.
	// This is equivalent to `ip route get 1.1.1.1` on Linux.
	// No actual connection is made since UDP is connectionless.
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err != nil {
		return "", fmt.Errorf("failed to determine local IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip := localAddr.IP.String()

	return ip, nil
}

// cleanupContainersWithOldID removes containers with old ID and no config hash label.
// This is a migration for simplifying the container names and updating existing containers
// when configuration changes instead of possibly orphaning them.
func (d *dockerBackend) cleanupContainersWithOldID(ctx context.Context) error {
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers for cleanup: %w", err)
	}

	for _, c := range containers {
		id := c.Labels["mcp.server.id"]
		if _, ok := c.Labels["mcp.deployment.id"]; !ok && id != "" {
			if err := d.removeObjectsForContainer(ctx, &c, id); err != nil {
				return fmt.Errorf("failed to remove container with old ID %s: %w", c.ID, err)
			}
		}
	}

	return nil
}

// deployServer will deploy the underlying container for the server. It will not deploy any shims or webhooks.
// This is only to give users the opportunity to view logs and debug the server they are trying to deploy.
func (d *dockerBackend) deployServer(ctx context.Context, server ServerConfig, _ []Webhook) error {
	configHash := clientID(server)
	// Check if container already exists
	existing, err := d.getContainer(ctx, server.MCPServerName)
	if err == nil && existing != nil {
		// Server is already deployed; nothing to do
		return nil
	}

	_, _, err = d.createAndStartContainer(ctx, server, "", configHash, nil)
	return err
}

func (d *dockerBackend) ensureServerDeployment(ctx context.Context, server ServerConfig, webhooks []Webhook) (ServerConfig, error) {
	serverName := server.MCPServerName
	// Copy the webhooks so we can change the URL without that affecting the original slice.
	transformedWebhooks := slices.Clone(webhooks)

	// List the currently deployed webhooks so we can delete any that are no longer needed.
	currentWebhooks, err := d.getWebhookContainers(ctx, server.MCPServerName)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("failed to list webhooks: %w", err)
	}

	existingWebhooks := make(map[string]struct{}, len(currentWebhooks))
	for _, webhook := range currentWebhooks {
		existingWebhooks[strings.TrimPrefix(webhook.Names[0], "/")] = struct{}{}
	}

	for i, webhook := range transformedWebhooks {
		webhook.URL = strings.Replace(webhook.URL, "http://localhost", d.hostBaseURL, 1)

		c, err := webhookToServerConfig(webhook, d.webhookBaseImage, server.MCPServerName, server.UserID, server.Scope, defaultContainerPort)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("failed to ensure webhook deployment: %w", err)
		} else if c, err = d.ensureDeployment(ctx, c, server.MCPServerName, d.containerEnv, nil); err != nil {
			return ServerConfig{}, fmt.Errorf("failed to ensure server deployment: %w", err)
		} else if existing, err := d.getContainer(ctx, c.MCPServerName); err != nil {
			return ServerConfig{}, fmt.Errorf("failed to build server config: %w", err)
		} else if existing == nil {
			return ServerConfig{}, fmt.Errorf("failed to ensure webhook deployment for %s", c.MCPServerName)
		} else if c, err = d.buildServerConfig(c, existing, c.ContainerPort, true); err != nil {
			return ServerConfig{}, fmt.Errorf("failed to build server config: %w", err)
		}

		webhook.URL = c.URL
		transformedWebhooks[i] = webhook

		delete(existingWebhooks, c.MCPServerName)
	}

	for name := range existingWebhooks {
		if err := d.shutdownServer(ctx, name); err != nil {
			return ServerConfig{}, fmt.Errorf("failed to delete webhook container %s: %w", name, err)
		}
	}

	if server.Runtime != otypes.RuntimeRemote {
		// For non-remote runtimes, we deploy a shim that handles webhooks.
		server, err = d.ensureDeployment(ctx, server, "", true, nil)
		if err != nil {
			return ServerConfig{}, err
		}

		server.MCPServerName += "-shim"
	} else {
		server.URL = strings.Replace(server.URL, "http://localhost", d.hostBaseURL, 1)
	}

	server, err = d.ensureDeployment(ctx, server, "", d.containerEnv, transformedWebhooks)
	// Ensure the name is the same as what it was when we started.
	server.MCPServerName = serverName
	return server, err
}

func (d *dockerBackend) ensureDeployment(ctx context.Context, server ServerConfig, mcpServerName string, containerEnv bool, webhooks []Webhook) (ServerConfig, error) {
	server.TokenExchangeEndpoint = localhostURLRegexp.ReplaceAllString(server.TokenExchangeEndpoint, d.hostBaseURLWithPort)
	server.AuditLogEndpoint = localhostURLRegexp.ReplaceAllString(server.AuditLogEndpoint, d.hostBaseURLWithPort)
	server.JWKSEndpoint = localhostURLRegexp.ReplaceAllString(server.JWKSEndpoint, d.hostBaseURLWithPort)

	for i, component := range server.Components {
		component.URL = localhostURLRegexp.ReplaceAllString(component.URL, d.hostBaseURLWithPort)
		server.Components[i] = component
	}

	configHash := clientID(server)
	if len(webhooks) > 0 {
		// Include webhooks in the config hash so that changes to webhooks trigger a redeployment
		configHash += hash.Digest(webhooks)
	}

	// Check if container already exists
	existing, err := d.getContainer(ctx, server.MCPServerName)
	if err == nil && existing != nil {
		if existing.Labels["mcp.config.hash"] != configHash ||
			existing.NetworkSettings == nil ||
			existing.NetworkSettings.Networks[d.network] == nil ||
			(server.Runtime == otypes.RuntimeRemote || server.Runtime == otypes.RuntimeComposite) && existing.Image != d.remoteShimBaseImage {
			// Clear the state. The below logic will remove and recreate the container.
			existing.State = ""
		}

		// Container exists, check state
		switch existing.State {
		case container.StateCreated:
			// Container exists and is created, start it and wait for it to be ready.
			if err := d.client.ContainerStart(ctx, existing.ID, container.StartOptions{}); err != nil {
				return ServerConfig{}, fmt.Errorf("failed to start container: %w", err)
			}

			if err := d.waitForContainer(ctx, existing.ID); err != nil {
				return ServerConfig{}, fmt.Errorf("failed to wait for container: %w", err)
			}

			existing, err = d.getContainer(ctx, server.MCPServerName)
			if err != nil {
				return ServerConfig{}, fmt.Errorf("failed to get running container: %w", err)
			}

			// The container is ready now, so fallthrough to the next case.
			fallthrough
		case container.StateRunning:
			containerPort := server.ContainerPort
			if containerPort == 0 {
				containerPort = defaultContainerPort
			}

			if err = d.ensureServerReady(ctx, existing, server, containerPort); err != nil {
				return ServerConfig{}, fmt.Errorf("server running, but readiness check failed: %w", err)
			}

			return d.buildServerConfig(server, existing, containerPort, containerEnv)
		default:
			// Container exists but not running, remove it and recreate
			if err := d.client.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true}); cerrdefs.IsConflict(err) {
				// The container is already being removed, wait for it to finish
				statusCh, errCh := d.client.ContainerWait(ctx, existing.ID, container.WaitConditionRemoved)
				select {
				case err := <-errCh:
					// It's OK if the container is already gone.
					if err != nil && !cerrdefs.IsNotFound(err) {
						return ServerConfig{}, fmt.Errorf("error waiting for stopped container to be removed: %w", err)
					}
				case <-statusCh:
				}
			} else if err != nil {
				return ServerConfig{}, fmt.Errorf("failed to remove stopped container: %w", err)
			}
		}
	}

	// Create new container
	return d.createAndStartAndWaitForContainer(ctx, server, mcpServerName, configHash, containerEnv, webhooks)
}

func (d *dockerBackend) transformConfig(ctx context.Context, serverConfig ServerConfig) (*ServerConfig, error) {
	containerName := serverConfig.MCPServerName
	if serverConfig.Runtime == otypes.RuntimeContainerized {
		// For containerized runtimes, we want to communicate with the shim.
		containerName += "-shim"
	}

	existing, err := d.getContainer(ctx, containerName)
	if err != nil || existing == nil || existing.State != "running" {
		// Container doesn't exist or isn't running, config can't be transformed
		return nil, nil
	}

	containerPort := serverConfig.ContainerPort
	if containerPort == 0 {
		containerPort = defaultContainerPort
	}

	transformed, err := d.buildServerConfig(serverConfig, existing, containerPort, d.containerEnv)
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
				Time:         otypes.Time{Time: time.Unix(event.Time, 0)},
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
	if err := d.client.ContainerRestart(ctx, id, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to restart container %s: %w", id, err)
	}

	return nil
}

func (d *dockerBackend) shutdownServer(ctx context.Context, id string) error {
	c, err := d.getContainer(ctx, id)
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("failed to get container %s for shutdown: %w", id, err)
	}

	if err := d.removeObjectsForContainer(ctx, c, id); err != nil {
		return fmt.Errorf("failed to remove objects for container %s: %w", id, err)
	}

	c, err = d.getContainer(ctx, id+"-shim")
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("failed to check for shim container %s for shutdown: %w", id, err)
	} else if err == nil {
		if err = d.removeObjectsForContainer(ctx, c, id+"-shim"); err != nil {
			return fmt.Errorf("failed to remove objects for shim container %s: %w", id+"-shim", err)
		}
	}

	webhookContainers, err := d.getWebhookContainers(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get webhook containers: %w", err)
	}

	for _, webhookContainer := range webhookContainers {
		if err := d.removeObjectsForContainer(ctx, &webhookContainer, webhookContainer.ID); err != nil {
			return fmt.Errorf("failed to remove objects for webhook container %s: %w", webhookContainer.ID, err)
		}
	}

	return nil
}

func (d *dockerBackend) removeObjectsForContainer(ctx context.Context, c *container.Summary, id string) error {
	var volumeNames []string
	if c != nil {
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
	if err := d.client.ContainerStop(ctx, id, container.StopOptions{}); err != nil && !cerrdefs.IsNotFound(err) {
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

func (d *dockerBackend) getWebhookContainers(ctx context.Context, mcpContainerName string) ([]container.Summary, error) {
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "label",
			Value: fmt.Sprintf("mcp.deployment.id=%s", mcpContainerName),
		}),
	})
	if err != nil {
		return nil, err
	}

	return containers, nil
}

func (d *dockerBackend) getHostPort(container *container.Summary, containerPort int) (int, error) {
	for _, port := range container.Ports {
		if port.PrivatePort == uint16(containerPort) && port.PublicPort != 0 {
			return int(port.PublicPort), nil
		}
	}
	return 0, fmt.Errorf("no public port found for container port %d", containerPort)
}

func (d *dockerBackend) buildServerConfig(server ServerConfig, c *container.Summary, containerPort int, containerEnv bool) (ServerConfig, error) {
	var (
		host string
		port = containerPort
	)

	if containerEnv {
		if c == nil || c.NetworkSettings == nil {
			return ServerConfig{}, fmt.Errorf("container %s not found or has no network settings", c.ID)
		}

		n, ok := c.NetworkSettings.Networks[d.network]
		if !ok || n.IPAddress == "" {
			return ServerConfig{}, fmt.Errorf("container %s is not connected to %s network", c.ID, d.network)
		}

		host = n.IPAddress
	} else {
		host = "localhost"

		var err error
		port, err = d.getHostPort(c, containerPort)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("failed to get host port: %w", err)
		}
	}

	url := fmt.Sprintf("http://%s:%d", host, port)
	if server.ContainerPath != "" {
		url = fmt.Sprintf("%s/%s", url, strings.TrimPrefix(server.ContainerPath, "/"))
	}

	return ServerConfig{
		URL:                       url,
		MCPServerNamespace:        server.MCPServerNamespace,
		MCPServerName:             server.MCPServerName,
		MCPServerDisplayName:      server.MCPServerDisplayName,
		Scope:                     c.ID,
		UserID:                    server.UserID,
		Runtime:                   otypes.RuntimeRemote,
		Audiences:                 server.Audiences,
		Issuer:                    server.Issuer,
		JWKSEndpoint:              server.JWKSEndpoint,
		TokenExchangeEndpoint:     server.TokenExchangeEndpoint,
		AuthorizeEndpoint:         server.AuthorizeEndpoint,
		TokenExchangeClientID:     server.TokenExchangeClientID,
		TokenExchangeClientSecret: server.TokenExchangeClientSecret,
		AuditLogEndpoint:          server.AuditLogEndpoint,
		AuditLogToken:             server.AuditLogToken,
		AuditLogMetadata:          server.AuditLogMetadata,
		ContainerPort:             containerPort,
		ContainerPath:             server.ContainerPath,
	}, nil
}

func (d *dockerBackend) createAndStartAndWaitForContainer(ctx context.Context, server ServerConfig, mcpServerName, configHash string, containerEnv bool, webhooks []Webhook) (retConfig ServerConfig, retErr error) {
	containerID, containerPort, err := d.createAndStartContainer(ctx, server, mcpServerName, configHash, webhooks)
	if err != nil {
		return ServerConfig{}, err
	}

	// Wait for container to be running and healthy
	if err := d.waitForContainer(ctx, containerID); err != nil {
		return retConfig, fmt.Errorf("container failed to become ready: %w", err)
	}

	c, err := d.getContainer(ctx, server.MCPServerName)
	if err != nil {
		return retConfig, fmt.Errorf("failed to get container after starting: %w", err)
	}

	if err = d.ensureServerReady(ctx, c, server, containerPort); err != nil {
		return retConfig, fmt.Errorf("server readiness check failed: %w", err)
	}

	return d.buildServerConfig(server, c, containerPort, containerEnv)
}

func (d *dockerBackend) createAndStartContainer(ctx context.Context, server ServerConfig, mcpServerName, configHash string, webhooks []Webhook) (string, int, error) {
	var (
		volumeMounts  []mount.Mount
		entrypoint    []string
		cmd           []string
		env           []string
		containerPort int
		image         string
	)

	// Prepare file volumes and environment variables
	fileVolumeName, fileEnvVars, err := d.prepareContainerFiles(ctx, server, mcpServerName)
	if err != nil {
		return "", 0, fmt.Errorf("failed to prepare container files: %w", err)
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
	case otypes.RuntimeUVX, otypes.RuntimeNPX, otypes.RuntimeRemote, otypes.RuntimeComposite:
		// Use base image with nanobot
		image = d.containerizedBaseImage
		if server.Runtime == otypes.RuntimeRemote || server.Runtime == otypes.RuntimeComposite {
			image = d.remoteShimBaseImage
			// Set nanobot environment variables
			env = []string{
				"NANOBOT_RUN_TRUSTED_ISSUER=" + server.Issuer,
				"NANOBOT_RUN_OAUTH_JWKSURL=" + localhostURLRegexp.ReplaceAllString(server.JWKSEndpoint, d.hostBaseURLWithPort),
				"NANOBOT_RUN_TRUSTED_AUDIENCES=" + strings.Join(server.Audiences, ","),
				"NANOBOT_RUN_OAUTH_CLIENT_ID=" + server.TokenExchangeClientID,
				"NANOBOT_RUN_OAUTH_CLIENT_SECRET=" + server.TokenExchangeClientSecret,
				"NANOBOT_RUN_OAUTH_TOKEN_URL=" + server.TokenExchangeEndpoint,
				"NANOBOT_RUN_OAUTH_AUTHORIZE_URL=" + server.AuthorizeEndpoint,
				"NANOBOT_RUN_AUDIT_LOG_TOKEN=" + server.AuditLogToken,
				"NANOBOT_RUN_AUDIT_LOG_SEND_URL=" + server.AuditLogEndpoint,
				"NANOBOT_RUN_AUDIT_LOG_BATCH_SIZE=" + strconv.Itoa(d.auditLogsBatchSize),
				"NANOBOT_RUN_AUDIT_LOG_FLUSH_INTERVAL_SECONDS=" + strconv.Itoa(d.auditLogsFlushIntervalSeconds),
				"NANOBOT_RUN_AUDIT_LOG_METADATA=" + server.AuditLogMetadata,
				"NANOBOT_RUN_OAUTH_SCOPES=profile",
				"NANOBOT_RUN_FORCE_FETCH_TOOL_LIST=true",
				"NANOBOT_DISABLE_HEALTH_CHECKER=true",
				"NANOBOT_RUN_APIKEY_AUTH_WEBHOOK_URL=" + localhostURLRegexp.ReplaceAllString(server.Issuer+"/api/api-keys/auth", d.hostBaseURLWithPort),
				"NANOBOT_RUN_MCPSERVER_ID=" + strings.TrimSuffix(server.MCPServerName, "-shim"),
			}
		}

		containerPort = defaultContainerPort

		// Prepare nanobot configuration
		nanobotVolumeName, err := d.prepareNanobotConfig(ctx, server, fileEnvVars, webhooks)
		if err != nil {
			return "", 0, fmt.Errorf("failed to prepare nanobot config: %w", err)
		}

		volumeMounts = append(volumeMounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: nanobotVolumeName,
			Target: "/run",
		})

		// Use nanobot command
		cmd = []string{"run", "--disable-ui", "--listen-address", fmt.Sprintf(":%d", defaultContainerPort), "/run/nanobot.yaml"}

		// Set nanobot environment variables
		env = append(env, "NANOBOT_RUN_HEALTHZ_PATH=/healthz", "OBOT_KUBERNETES_MODE=true")

	case otypes.RuntimeContainerized:
		// Use specified container image or base image
		if server.ContainerImage == "" {
			return "", 0, fmt.Errorf("container image must be specified for containerized runtime")
		}

		image = server.ContainerImage
		containerPort = server.ContainerPort

		// Use server's command and args
		if server.Command != "" {
			entrypoint = []string{server.Command}
		}
		cmd = server.Args

		// Use server's environment variables plus file env vars
		metaEnvVar := make([]string, 0, len(server.Env)+len(fileEnvVars))
		for _, val := range server.Env {
			k, _, ok := strings.Cut(val, "=")
			if ok {
				metaEnvVar = append(metaEnvVar, k)
			}
			env = append(env, val)
		}
		for k, v := range fileEnvVars {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
			metaEnvVar = append(metaEnvVar, k)
		}

		env = append(env, fmt.Sprintf("NANOBOT_META_ENV=%s", strings.Join(metaEnvVar, ",")))
	default:
		return "", 0, fmt.Errorf("unsupported runtime: %s", server.Runtime)
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
			"mcp.server.displayName": server.MCPServerDisplayName,
			"mcp.deployment.id":      mcpServerName,
			"mcp.server.id":          server.MCPServerName,
			"mcp.user.id":            server.UserID,
			"mcp.config.hash":        configHash,
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

	if err := d.pullImage(ctx, image, false); err != nil {
		return "", 0, fmt.Errorf("failed to ensure image exists: %w", err)
	}

	// Configure network
	networkingConfig := &network.NetworkingConfig{}
	if d.network != "" {
		networkingConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			d.network: {},
		}
	}

	var containerID string
	// There seems to be a race condition in the Docker API where creating the container fails with a conflict,
	// but getting the container with the name returns no results.
	// This hack addresses this by retrying 3 times, waiting a second each time.
	for range 3 {
		// Create container
		resp, err := d.client.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, server.MCPServerName)
		if err != nil {
			if !cerrdefs.IsConflict(err) && !cerrdefs.IsAlreadyExists(err) {
				return "", 0, fmt.Errorf("failed to create container: %w", err)
			}

			cont, getErr := d.getContainer(ctx, server.MCPServerName)
			if getErr != nil {
				return "", 0, fmt.Errorf("failed to create container: %w", err)
			}
			if cont == nil {
				time.Sleep(time.Second)
				continue
			}

			containerID = cont.ID
		} else {
			containerID = resp.ID
		}
	}
	if containerID == "" {
		return "", 0, fmt.Errorf("failed to create container")
	}

	// Start container
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return "", 0, fmt.Errorf("failed to start container: %w", err)
	}

	return containerID, containerPort, nil
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
				// Container is running
				return nil
			}

			if inspect.State.Dead || inspect.State.OOMKilled || inspect.State.ExitCode != 0 {
				return fmt.Errorf("container failed to start: %s", inspect.State.Status)
			}
		}
	}
}

func (d *dockerBackend) ensureServerReady(ctx context.Context, c *container.Summary, server ServerConfig, containerPort int) error {
	var (
		host string
		err  error
		port = containerPort
	)
	if d.containerEnv {
		if c == nil || c.NetworkSettings == nil {
			return fmt.Errorf("container %s not found or has no network settings", server.MCPServerName)
		}

		n, ok := c.NetworkSettings.Networks[d.network]
		if !ok || n.IPAddress == "" {
			return fmt.Errorf("container %s is not connected to %s network", server.MCPServerName, d.network)
		}

		host = n.IPAddress
	} else {
		port, err = d.getHostPort(c, containerPort)
		if err != nil {
			return fmt.Errorf("failed to get host port: %w", err)
		}

		host = "localhost"
	}

	if err = ensureServerReady(ctx, fmt.Sprintf("http://%s:%d", host, port), server); err != nil {
		return fmt.Errorf("server readiness check failed: %w", err)
	}

	return nil
}

// prepareContainerFiles creates a volume for server.Files and returns volume name and env vars
func (d *dockerBackend) prepareContainerFiles(ctx context.Context, server ServerConfig, mcpServerName string) (string, map[string]string, error) {
	if len(server.Files) == 0 {
		return "", nil, nil
	}

	volumeName, envVars, err := d.createVolumeWithFiles(ctx, server.Files, server.MCPServerName, mcpServerName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create volume with files: %w", err)
	}

	return volumeName, envVars, nil
}

// createVolumeWithFiles creates an anonymous volume and populates it with file data using an init container
func (d *dockerBackend) createVolumeWithFiles(ctx context.Context, files []File, containerName, mcpServerName string) (string, map[string]string, error) {
	if len(files) == 0 {
		return "", nil, nil
	}

	volumeName := containerName + "-files"

	// Create anonymous volume
	_, err := d.client.VolumeCreate(ctx, volume.CreateOptions{
		Labels: map[string]string{
			"mcp.server.id":     containerName,
			"mcp.deployment.id": mcpServerName,
			"mcp.purpose":       "files",
		},
		Name: volumeName,
	})
	if err != nil && !cerrdefs.IsAlreadyExists(err) {
		return "", nil, fmt.Errorf("failed to create volume: %w", err)
	}

	// Create init container to populate the volume
	initImage := "alpine:latest"
	if err := d.pullImage(ctx, initImage, true); err != nil {
		return "", nil, fmt.Errorf("failed to ensure init image exists: %w", err)
	}

	// Build script to create files in the volume
	var script strings.Builder
	script.WriteString("#!/bin/sh\nset -e\n")

	envVars := make(map[string]string, len(files))
	for _, file := range files {
		// Generate unique filename for container
		filename := fmt.Sprintf("%s-%s", containerName, hash.Digest(file)[:24])
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

	resp, err := d.client.ContainerCreate(ctx, initConfig, initHostConfig, &network.NetworkingConfig{}, nil, fmt.Sprintf("%s-init-%s", containerName, strings.ToLower(rand.Text())))
	if err != nil {
		return "", nil, fmt.Errorf("failed to create init container: %w", err)
	}

	// Start and wait for init container to complete
	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", nil, fmt.Errorf("failed to start init container: %w", err)
	}

	// Wait for init container to complete
	statusCh, errCh := d.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
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

func (d *dockerBackend) pullImage(ctx context.Context, imageName string, ifNotExists bool) error {
	if ifNotExists {
		// Check if image exists locally
		_, err := d.client.ImageInspect(ctx, imageName)
		if err == nil {
			// Image exists locally
			return nil
		}
	}

	reader, err := d.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		if cerrdefs.IsUnauthorized(err) || cerrdefs.IsPermissionDenied(err) || cerrdefs.IsNotFound(err) || cerrdefs.IsInternal(err) && strings.HasSuffix(err.Error(), "unauthorized") {
			// Check if image exists locally
			_, err := d.client.ImageInspect(ctx, imageName)
			if err == nil {
				// Image exists locally
				return nil
			}
		}
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
func (d *dockerBackend) prepareNanobotConfig(ctx context.Context, server ServerConfig, envVars map[string]string, webhooks []Webhook) (string, error) {
	// Create all environment variables map
	allEnvVars := make(map[string]string, len(server.Env)+len(envVars))
	headers := make(map[string]string, len(server.Headers))

	// Add server environment variables
	for _, env := range server.Env {
		if k, v, ok := strings.Cut(env, "="); ok {
			allEnvVars[k] = v
		}
	}
	maps.Copy(allEnvVars, envVars)

	// Add server headers
	for _, header := range server.Headers {
		if k, v, ok := strings.Cut(header, "="); ok {
			headers[k] = v
		}
	}

	var (
		nanobotYAML string
		err         error
	)
	if server.Runtime == otypes.RuntimeComposite {
		nanobotYAML, err = constructNanobotYAMLForCompositeServer(server.Components)
	} else {
		nanobotYAML, err = constructNanobotYAMLForServer(server.MCPServerDisplayName, server.URL, server.Command, server.Args, allEnvVars, headers, webhooks)
	}
	if err != nil {
		return "", fmt.Errorf("failed to construct nanobot YAML: %w", err)
	}

	volumeName := server.MCPServerName + "-nanobot-config"
	// Create volume for nanobot config
	_, err = d.client.VolumeCreate(ctx, volume.CreateOptions{
		Labels: map[string]string{
			"mcp.server.id": server.MCPServerName,
			"mcp.purpose":   "nanobot-config",
		},
		Name: volumeName,
	})
	if err != nil && !cerrdefs.IsAlreadyExists(err) {
		return "", fmt.Errorf("failed to create nanobot config volume: %w", err)
	}

	// Create init container to populate the volume with nanobot config
	initImage := "alpine:latest"
	if err = d.pullImage(ctx, initImage, true); err != nil {
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

	// Configure network (same as main containers)
	initNetworkingConfig := &network.NetworkingConfig{}
	if d.network != "" {
		initNetworkingConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			d.network: {},
		}
	}

	resp, err := d.client.ContainerCreate(ctx, initConfig, initHostConfig, initNetworkingConfig, nil, fmt.Sprintf("%s-nanobot-init-%s", server.MCPServerName, strings.ToLower(rand.Text())))
	if err != nil {
		return "", fmt.Errorf("failed to create nanobot init container: %w", err)
	}

	// Start and wait for init container to complete
	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start init container: %w", err)
	}

	// Wait for init container to complete
	statusCh, errCh := d.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
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
