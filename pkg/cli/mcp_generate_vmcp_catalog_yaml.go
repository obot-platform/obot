package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

const (
	outputModeStdout = "stdout"
	outputModeFile   = "file"
	outputModeDir    = "dir"
)

type MCPGenerateVMCPCatalogYAML struct {
	PromptConfig

	Mode      string `usage:"Where to write the YAML: stdout, file, or dir" default:"stdout"`
	Overwrite bool   `usage:"Replace existing files in file and dir modes"`

	root *Obot
}

func (m *MCPGenerateVMCPCatalogYAML) Customize(cmd *cobra.Command) {
	cmd.Use = "generate-vmcp-catalog-yaml <catalog-source-url> [path]"
	cmd.Short = "Generate catalog YAML for orphaned vMCPs from a catalog source"
	cmd.Long = `Generate catalog YAML for orphaned vMCPs from the given catalog source URL. An orphaned vMCP was migrated from a composite catalog entry synced from that source, and no catalog source defines it yet.

Publish the YAML in that same catalog source in place of the composite definitions. On its next sync, Obot takes over each vMCP and keeps its existing connections, configuration, and credentials.

Modes:
  stdout  Print every vMCP as one YAML list (the default).
  file    Write every vMCP as one YAML list to [path].
  dir     Write each vMCP to its own file in the [path] directory, named after its display name.

Existing files are never replaced unless --overwrite is set.`
	cmd.Example = `  obot mcp generate-vmcp-catalog-yaml https://github.com/example/catalog
  obot mcp generate-vmcp-catalog-yaml https://github.com/example/catalog --mode file vmcps.yaml
  obot mcp generate-vmcp-catalog-yaml https://github.com/example/catalog --mode dir ./vmcps`
	cmd.Args = cobra.RangeArgs(1, 2)
}

func (m *MCPGenerateVMCPCatalogYAML) Run(cmd *cobra.Command, args []string) error {
	switch m.Mode {
	case outputModeStdout:
		if len(args) != 1 {
			return fmt.Errorf("--mode stdout does not take a path")
		}
	case outputModeFile, outputModeDir:
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("--mode %s requires a path", m.Mode)
		}
	default:
		return fmt.Errorf("invalid --mode %q: must be stdout, file, or dir", m.Mode)
	}
	if m.root == nil || m.root.Client == nil {
		return fmt.Errorf("mcp generate-vmcp-catalog-yaml: no API client configured")
	}

	sourceURL := strings.TrimSpace(args[0])
	client := m.root.Client
	// An explicit token only needs API access. A fetched token replaces the
	// stored login, so request the default scopes too rather than narrowing it.
	scopes := []string{types.APIKeyScopeAPI}
	if client.Token == "" {
		scopes = append(types.DefaultCLIAPIKeyScopes(), types.APIKeyScopeAPI)
	}
	token, err := client.GetToken(cmd.Context(), apiclient.TokenFetchOptions{Scopes: scopes})
	if err != nil {
		return fmt.Errorf("authenticate with Obot: %w", err)
	}

	result, err := client.WithToken(token).ListOrphanedVMCPs(cmd.Context(), system.DefaultCatalog, sourceURL)
	if err != nil {
		return err
	}

	stderr := cmd.ErrOrStderr()
	var (
		generated []types.OrphanedVMCPCatalogItem
		skipped   int
	)
	for _, item := range result.Items {
		if item.Error != "" {
			fmt.Fprintf(stderr, "Skipping vMCP %q: %s\n", item.DisplayName, item.Error)
			skipped++
			continue
		}
		generated = append(generated, item)
	}
	if len(generated) == 0 {
		if skipped == 0 {
			fmt.Fprintf(stderr, "No orphaned vMCPs from %s are awaiting catalog sync.\n", sourceURL)
			return nil
		}
		return fmt.Errorf("none of the %d orphaned vMCPs from %s could be generated", skipped, sourceURL)
	}

	switch m.Mode {
	case outputModeStdout:
		err = writeVMCPList(cmd.OutOrStdout(), generated)
	case outputModeFile:
		err = m.writeFile(args[1], generated)
		if err == nil {
			fmt.Fprintf(stderr, "Wrote %d vMCPs to %s\n", len(generated), args[1])
		}
	case outputModeDir:
		var notWritten int
		notWritten, err = m.writeDir(stderr, args[1], generated)
		skipped += notWritten
	}
	if err != nil {
		return err
	}
	if skipped > 0 {
		return fmt.Errorf("%d orphaned vMCPs were not generated", skipped)
	}
	return nil
}

func writeVMCPList(w io.Writer, items []types.OrphanedVMCPCatalogItem) error {
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, item := range items {
		var document yaml.Node
		if err := yaml.Unmarshal([]byte(item.YAML), &document); err != nil {
			return fmt.Errorf("invalid YAML for vMCP %q: %w", item.DisplayName, err)
		}
		if len(document.Content) != 1 {
			return fmt.Errorf("invalid YAML for vMCP %q: expected one document", item.DisplayName)
		}
		list.Content = append(list.Content, document.Content[0])
	}

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(list); err != nil {
		return err
	}
	return encoder.Close()
}

func (m *MCPGenerateVMCPCatalogYAML) writeFile(path string, items []types.OrphanedVMCPCatalogItem) error {
	var buf bytes.Buffer
	if err := writeVMCPList(&buf, items); err != nil {
		return err
	}
	if err := writeOutputFile(path, buf.Bytes(), m.Overwrite); errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists; use --overwrite to replace it", path)
	} else if err != nil {
		return err
	}
	return nil
}

// writeDir writes each vMCP to its own file and returns how many were left
// unwritten because their file already exists.
func (m *MCPGenerateVMCPCatalogYAML) writeDir(stderr io.Writer, dir string, items []types.OrphanedVMCPCatalogItem) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}

	var (
		notWritten int
		seen       = make(map[string]struct{}, len(items))
	)
	for _, item := range items {
		name := vmcpFileName(item.DisplayName)
		// Display names are not unique, and some file systems ignore case.
		if _, ok := seen[strings.ToLower(name)]; ok {
			name = vmcpFileName(item.DisplayName + "-" + item.EntryKey)
		}
		seen[strings.ToLower(name)] = struct{}{}

		path := filepath.Join(dir, name)
		if err := writeOutputFile(path, []byte(item.YAML), m.Overwrite); errors.Is(err, os.ErrExist) {
			fmt.Fprintf(stderr, "Skipping vMCP %q: %s already exists; use --overwrite to replace it\n", item.DisplayName, path)
			notWritten++
			continue
		} else if err != nil {
			return notWritten, err
		}
		fmt.Fprintf(stderr, "Wrote %s\n", path)
	}
	return notWritten, nil
}

// writeOutputFile writes data to path, returning an error wrapping
// os.ErrExist if the file exists and overwrite is false.
func writeOutputFile(path string, data []byte, overwrite bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

// vmcpFileName returns a portable YAML file name for a vMCP display name.
func vmcpFileName(displayName string) string {
	name := strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, displayName)
	// Leading dots hide the file, and trailing dots and spaces are dropped on Windows.
	name = strings.TrimRight(strings.TrimLeft(strings.TrimSpace(name), "."), ". ")
	if name == "" {
		name = "vmcp"
	}
	return name + ".yaml"
}
