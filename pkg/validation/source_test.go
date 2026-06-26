package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceIDForURL(t *testing.T) {
	assert.Equal(t, "github.com/company/mcp-catalog", SourceIDForURL("https://github.com/company/mcp-catalog"))
	assert.Equal(t, "github.com/company/mcp-catalog", SourceIDForURL("http://github.com/company/mcp-catalog/"))
	assert.Equal(t, "github.com/company/mcp-catalog", SourceIDForURL("github.com/company/mcp-catalog"))
	assert.Equal(t, "/tmp/catalog", SourceIDForURL("/tmp/catalog"))
	assert.Equal(t, "/", SourceIDForURL("/"))
}
