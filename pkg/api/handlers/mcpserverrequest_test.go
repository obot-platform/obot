package handlers

import (
	"testing"
)

func TestComponentName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "GitHub", want: "github"},
		{in: "  Linear  MCP!! ", want: "linear-mcp"},
		{in: "@modelcontextprotocol/server-filesystem", want: "modelcontextprotocol-server-filesystem"},
		{in: "!!!", want: "server"},
	}
	for _, tt := range tests {
		if got := componentName(tt.in); got != tt.want {
			t.Errorf("componentName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
