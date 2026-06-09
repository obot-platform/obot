package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/adrg/xdg"
	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/devicescan"
	"github.com/obot-platform/obot/pkg/version"
	"github.com/spf13/cobra"
)

type Scan struct {
	DeviceID string `usage:"Override the persisted device identifier. Empty resolves via OBOT_SCAN_DEVICE_ID env var or the file at $XDG_DATA_HOME/obot/device_id (generated on first run)" env:"OBOT_SCAN_DEVICE_ID" hidden:"true"`
	Submit   bool   `usage:"Submit the scan to the configured Obot server" env:"OBOT_SCAN_SUBMIT"`
	JSON     bool   `usage:"Print the scan result as JSON"`
	Timeout  int    `usage:"Number of seconds to wait for the scan to complete" default:"60" env:"OBOT_SCAN_TIMEOUT"`
	MaxDepth int    `usage:"Maximum path depth (in segments below $HOME) to match when crawling for project-scope configs and skills; e.g. 5 matches files up to $HOME/a/b/c/d/e" default:"5" env:"OBOT_SCAN_MAX_DEPTH"`

	root *Obot
}

func (s *Scan) Customize(cmd *cobra.Command) {
	cmd.Use = "scan"
	cmd.Short = "Inventory local AI client configuration"
	cmd.Args = cobra.NoArgs
}

func (s *Scan) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if s.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(s.Timeout)*time.Second)
		defer cancel()
	}

	manifest, err := s.collectScanManifest(ctx)
	if err != nil {
		return err
	}

	if s.JSON {
		if err := writeJSON(cmd, manifest); err != nil {
			return err
		}
	} else if err := writeScanTable(cmd, manifest); err != nil {
		return err
	}

	if !s.Submit {
		return nil
	}

	return s.submitScanManifest(ctx, cmd, manifest)
}

func (s *Scan) submitScanManifest(ctx context.Context, cmd *cobra.Command, manifest types.DeviceScanManifest) error {
	if s.root.Client == nil {
		return fmt.Errorf("scan: --submit requires an API client")
	}
	resp, err := s.root.Client.SubmitDeviceScan(ctx, manifest)
	if err != nil {
		return fmt.Errorf("submit scan: %w", err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Submitted scan (received_at=%s)\n", resp.ReceivedAt.GetTime().Format(time.RFC3339))
	return nil
}

func (s *Scan) collectScanManifest(ctx context.Context) (types.DeviceScanManifest, error) {
	deviceID, err := ensureDeviceID(s.DeviceID)
	if err != nil {
		return types.DeviceScanManifest{}, fmt.Errorf("resolve device id: %w", err)
	}

	hostname, _ := os.Hostname()
	var username string
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	manifest := types.DeviceScanManifest{
		ScannerVersion: version.Get().String(),
		ScannedAt:      types.Time{Time: time.Now().UTC()},
		DeviceID:       deviceID,
		Hostname:       hostname,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Username:       username,
	}

	homePath, err := os.UserHomeDir()
	if err != nil {
		return types.DeviceScanManifest{}, fmt.Errorf("failed to get user home dir: %w", err)
	}

	collected, err := devicescan.Scan(ctx, os.DirFS(homePath), homePath, s.MaxDepth)
	if err != nil {
		return types.DeviceScanManifest{}, fmt.Errorf("scan: %w", err)
	}

	manifest.Files = collected.Files
	manifest.MCPServers = collected.MCPServers
	manifest.Skills = collected.Skills
	manifest.Plugins = collected.Plugins
	manifest.Clients = collected.Clients
	return manifest, nil
}

func writeScanTable(cmd *cobra.Command, manifest types.DeviceScanManifest) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Device:    %s (%s/%s)\n", tableCell(manifest.Hostname), tableCell(manifest.OS), tableCell(manifest.Arch))
	if manifest.Username != "" {
		fmt.Fprintf(out, "User:      %s\n", tableCell(manifest.Username))
	}
	fmt.Fprintf(out, "Device ID: %s\n", tableCell(manifest.DeviceID))
	fmt.Fprintf(out, "Scanned:   %s\n", manifest.ScannedAt.GetTime().Format(time.RFC3339))
	fmt.Fprintf(out, "Found:     %d clients, %d MCP servers, %d skills, %d plugins, %d files\n\n",
		len(manifest.Clients), len(manifest.MCPServers), countPhysicalSkills(manifest.Skills), len(manifest.Plugins), len(manifest.Files))

	if len(manifest.Clients) == 0 {
		fmt.Fprintln(out, "No clients found")
		return nil
	}

	mcpCounts := map[string]int{}
	for _, server := range manifest.MCPServers {
		mcpCounts[server.Client]++
	}
	skillCounts := map[string]int{}
	for _, skill := range manifest.Skills {
		skillCounts[skill.Client]++
	}
	pluginCounts := map[string]int{}
	for _, plugin := range manifest.Plugins {
		pluginCounts[plugin.Client]++
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CLIENT\tMCP SERVERS\tSKILLS\tPLUGINS\tCONFIG PATH")
	for _, client := range manifest.Clients {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n",
			tableCell(client.Name),
			mcpCounts[client.Name],
			skillCounts[client.Name],
			pluginCounts[client.Name],
			tableCell(client.ConfigPath),
		)
	}
	return w.Flush()
}

// countPhysicalSkills counts distinct physical SKILL.md observations.
// The scanner emits one manifest entry per client attribution, so the
// same skill directory can appear under multiple clients; the per-client
// table columns want that fan-out, but the headline count shouldn't.
func countPhysicalSkills(skills []types.DeviceScanSkill) int {
	seen := map[string]bool{}
	for _, skill := range skills {
		seen[skill.File+"\x00"+skill.ProjectPath+"\x00"+skill.Name] = true
	}
	return len(seen)
}

// ensureDeviceID returns deviceID if it is non-empty after trimming.
// Otherwise it reads (or, on first call, generates and persists) a UUIDv4 at
// xdg.DataFile("obot/device_id") with mode 0600.
//
// On macOS the file lands at ~/Library/Application Support/obot/device_id;
// on Linux at $XDG_DATA_HOME/obot/device_id (defaulting to
// ~/.local/share/obot/device_id); on Windows at %LocalAppData%\obot\device_id.
func ensureDeviceID(deviceID string) (string, error) {
	if deviceID = strings.TrimSpace(deviceID); deviceID != "" {
		return deviceID, nil
	}
	path, err := xdg.DataFile(filepath.Join("obot", "device_id"))
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	if id := strings.TrimSpace(string(data)); id != "" {
		return id, nil
	}
	id := uuid.NewString()
	if err := os.WriteFile(path, []byte(id), 0600); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return id, nil
}
