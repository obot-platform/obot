package connectroute

import "testing"

func TestParseVersionedPath(t *testing.T) {
	tests := []struct {
		path    string
		matched bool
		want    Versioned
		wantErr bool
	}{
		{path: "/mcp-connect/entry/2"},
		{path: "/versioned-mcp-connect/entry/2", matched: true, want: Versioned{EntryID: "entry", Version: 2}},
		{path: "/versioned-mcp-connect/entry/2/messages/1", matched: true, want: Versioned{EntryID: "entry", Version: 2, Rest: "messages/1"}},
		{path: "/versioned-mcp-connect/entry/0", matched: true, want: Versioned{EntryID: "entry", Version: 0}},
		{path: "/versioned-mcp-connect/entry", matched: true, wantErr: true},
		{path: "/versioned-mcp-connect/entry/latest", matched: true, wantErr: true},
		{path: "/versioned-mcp-connect//2", matched: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, matched, err := ParseVersionedPath(tt.path)
			if matched != tt.matched {
				t.Fatalf("matched = %v, want %v", matched, tt.matched)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("route = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestVersionedResourceOmitsServerIDAndSubpath(t *testing.T) {
	route := Versioned{EntryID: "catalog-entry", Version: 3, Rest: "messages/1"}
	if got, want := route.Resource("https://obot.example.com/"), "https://obot.example.com/versioned-mcp-connect/catalog-entry/3"; got != want {
		t.Fatalf("resource = %q, want %q", got, want)
	}
}
