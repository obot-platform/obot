package mcpgateway

import (
	"slices"
	"testing"
)

func TestParseAuditLogServerNames(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values []string
		want   []string
	}{
		{
			name:   "commas and escaped characters",
			values: []string{`["Outlook, Calendar","A \"quoted\" \\ server"," spaced "]`},
			want:   []string{"Outlook, Calendar", `A "quoted" \ server`, " spaced "},
		},
		{
			name:   "legacy and repeated parameters",
			values: []string{"one, two", "three", `["Outlook, Calendar"]`},
			want:   []string{"one", "two", "three", "Outlook, Calendar"},
		},
		{
			name:   "empty selections",
			values: []string{"", "[]", `[""]`},
		},
		{
			name:   "non JSON names",
			values: []string{"[server]", "123", "null"},
			want:   []string{"[server]", "123", "null"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAuditLogOpts(map[string][]string{"mcp_server": tt.values}).MCPServer
			if !slices.Equal(got, tt.want) {
				t.Fatalf("MCPServer = %q, want %q", got, tt.want)
			}
		})
	}
}
