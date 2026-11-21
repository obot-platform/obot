package mcpcatalog

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestDocumentedSecretBindingExamplesUseCurrentConfiguration(t *testing.T) {
	// Inspect the actual Markdown, independently of the documentation exporter.
	data, err := os.ReadFile("../../docs/docs/configuration/mcp-server-gitops.md")
	require.NoError(t, err)
	_, section, ok := strings.Cut(string(data), "### Kubernetes Secret Bindings\n")
	require.True(t, ok)
	section, _, ok = strings.Cut(section, "### OpenAPI schemas\n")
	require.True(t, ok)
	blocks := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindAllStringSubmatch(section, -1)
	usages := []types.Usage{types.Env, types.File, types.DynamicFile, types.Header}
	require.Len(t, blocks, len(usages))
	for i, block := range blocks {
		t.Run(string(usages[i]), func(t *testing.T) {
			text := []byte(block[1])
			require.NoError(t, ValidateConfigurationFields(text))
			var fragment types.MCPServerCatalogEntryManifest
			require.NoError(t, yaml.UnmarshalStrict(text, &fragment))
			require.NoError(t, fragment.ValidateConfig())
			require.NoError(t, mcp.ValidateSecretBindingsCatalogEntry(fragment, true, false, mcp.RuntimeBackendKubernetes))
			require.Len(t, fragment.Config, 1)
			require.Equal(t, usages[i], fragment.Config[0].Usage)
			require.NotNil(t, fragment.Config[0].SecretBinding)
			require.Empty(t, fragment.Config[0].Value)
		})
	}
}
