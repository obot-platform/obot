package mcp

import (
	"testing"
)

func TestHookMappingMatches(t *testing.T) {
	hook := HookMapping{Name: "tools/call", Params: map[string]string{"name": "echo", "direction": "request"}}
	if !hook.Matches("tools/call", map[string]string{"name": "echo", "direction": "request"}) {
		t.Fatal("expected hook to match")
	}
	if hook.Matches("tools/list", map[string]string{"name": "echo", "direction": "request"}) {
		t.Fatal("hook matched a different method")
	}
	if hook.Matches("tools/call", map[string]string{"name": "other", "direction": "request"}) {
		t.Fatal("hook matched different params")
	}
}
