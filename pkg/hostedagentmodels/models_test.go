package hostedagentmodels

import (
	"testing"

	"github.com/obot-platform/obot/pkg/system"
)

func TestDatabricksProviderCapabilities(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"OpenAIResponses", "OpenResponses", ""} {
		apis := APIsFor(system.DatabricksModelProvider, dialect)
		if len(apis) != 1 || apis[0] != APIOpenAIResponses {
			t.Errorf("APIsFor(%q) = %#v, want Responses", dialect, apis)
		}
	}
	if path, ok := ProxyPath(system.DatabricksModelProvider); !ok || path != "databricks" {
		t.Errorf("ProxyPath() = %q, %v, want databricks, true", path, ok)
	}
}
