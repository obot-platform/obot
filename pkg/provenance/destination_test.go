package provenance

import (
	"reflect"
	"testing"
)

func TestDestinationHosts(t *testing.T) {
	for _, tt := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{
			name: "explicit destination takes priority over URLs and body contact details",
			args: map[string]any{"destination_url": "https://care.internal.example/intake", "url": "https://other.example", "body": map[string]any{"email": "patient@outside.example"}},
			want: []string{"care.internal.example", "other.example"},
		},
		{
			name: "body email and nested URLs are content",
			args: map[string]any{"body": "Patient: ada@outside.example. Documentation https://outside.example", "payload": map[string]any{"url": "https://outside.example"}},
		},
		{
			name: "all explicit recipients retained",
			args: map[string]any{"to": "Ada <ada@corp.example>", "bcc": "hacker@outside.example", "cc": "Care team <team@corp.example>, bob@partner.example"},
			want: []string{"corp.example", "partner.example", "outside.example"},
		},
		{
			name: "recipient list and nested address objects",
			args: map[string]any{"recipients": []any{"a@corp.example", map[string]any{"emailAddress": map[string]any{"address": "b@outside.example", "body": "https://content.example"}}}},
			want: []string{"corp.example", "outside.example"},
		},
		{
			name: "case and separator aliases",
			args: map[string]any{"Destination-URL": "HTTPS://CARE.EXAMPLE:8443/path", "TO": "mailto:a@corp.example"},
			want: []string{"care.example", "corp.example"},
		},
		{
			name: "explicit nested destination",
			args: map[string]any{"destination": map[string]any{"url": "https://care.example", "body": "hacker@outside.example"}},
			want: []string{"care.example"},
		},
		{
			name: "bare host with port",
			args: map[string]any{"host": "db.internal.example:5432"},
			want: []string{"db.internal.example"},
		},
		{
			name: "non address is not a host",
			args: map[string]any{"to": "bob"},
		},
		{
			name: "nil",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Repeated calls exercise randomized Go map iteration. Route priority
			// and every recipient must stay identical on every call.
			for range 100 {
				if got := DestinationHosts(tt.args); !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("hosts = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
