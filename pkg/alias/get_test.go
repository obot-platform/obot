package alias

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestKeyFromScopeID(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		id       string
		wantFmt  string // Expected format pattern
		testHash bool   // Whether to test hash consistency
	}{
		{
			name:     "standard scope and id",
			scope:    "Agent",
			id:       "my-agent",
			wantFmt:  system.AliasPrefix, // Must start with al1
			testHash: true,
		},
		{
			name:     "empty scope",
			scope:    "",
			id:       "test-id",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
		{
			name:     "empty id",
			scope:    "Workflow",
			id:       "",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
		{
			name:     "both empty",
			scope:    "",
			id:       "",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
		{
			name:     "special characters in id",
			scope:    "Model",
			id:       "test-id_with.special/chars",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
		{
			name:     "unicode in id",
			scope:    "Tool",
			id:       "测试-id",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
		{
			name:     "long scope and id",
			scope:    "VeryLongScopeNameThatExceedsNormalLength",
			id:       "very-long-id-name-that-also-exceeds-normal-length-constraints",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
		{
			name:     "scope with spaces",
			scope:    "My Scope",
			id:       "my-id",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
		{
			name:     "id with newlines",
			scope:    "Agent",
			id:       "id\nwith\nnewlines",
			wantFmt:  system.AliasPrefix,
			testHash: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KeyFromScopeID(tt.scope, tt.id)

			// Check format: must start with system.AliasPrefix and be followed by 8 chars
			if !strings.HasPrefix(got, tt.wantFmt) {
				t.Errorf("KeyFromScopeID() = %v, want prefix %v", got, tt.wantFmt)
			}

			// Check total length: "al1" (3 chars) + 8 hash chars = 11 chars
			expectedLen := len(system.AliasPrefix) + 8
			if len(got) != expectedLen {
				t.Errorf("KeyFromScopeID() length = %v, want %v", len(got), expectedLen)
			}

			// Test hash consistency (same inputs should produce same output)
			if tt.testHash {
				got2 := KeyFromScopeID(tt.scope, tt.id)
				if got != got2 {
					t.Errorf("KeyFromScopeID() not consistent: first call = %v, second call = %v", got, got2)
				}
			}

			// Verify only valid characters (al1 prefix + hex hash)
			hashPart := got[len(system.AliasPrefix):]
			for _, c := range hashPart {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("KeyFromScopeID() hash part contains non-hex character: %v", string(c))
				}
			}
		})
	}
}

func TestKeyFromScopeID_DifferentInputs(t *testing.T) {
	// Test that different inputs produce different outputs
	key1 := KeyFromScopeID("Agent", "my-agent")
	key2 := KeyFromScopeID("Workflow", "my-agent")
	key3 := KeyFromScopeID("Agent", "other-agent")
	key4 := KeyFromScopeID("Agent", "my-agent") // Same as key1

	if key1 == key2 {
		t.Errorf("Different scopes should produce different keys: %v == %v", key1, key2)
	}
	if key1 == key3 {
		t.Errorf("Different ids should produce different keys: %v == %v", key1, key3)
	}
	if key1 != key4 {
		t.Errorf("Same inputs should produce same keys: %v != %v", key1, key4)
	}
}

// Mock types for testing
type mockAliasable struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	aliasName         string
	aliasScope        string
	observedGeneration int64
}

func (m *mockAliasable) GetAliasName() string {
	return m.aliasName
}

func (m *mockAliasable) SetAssigned(bool) {}

func (m *mockAliasable) IsAssigned() bool {
	return false
}

func (m *mockAliasable) GetGeneration() int64 {
	return m.observedGeneration
}

func (m *mockAliasable) GetObservedGeneration() int64 {
	return m.observedGeneration
}

func (m *mockAliasable) SetObservedGeneration(gen int64) {
	m.observedGeneration = gen
}

func (m *mockAliasable) GetAliasScope() string {
	return m.aliasScope
}

func (m *mockAliasable) DeepCopyObject() runtime.Object {
	return m
}

// mockAliasableNotScoped implements Aliasable but NOT AliasScoped
type mockAliasableNotScoped struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	aliasName         string
	observedGeneration int64
}

func (m *mockAliasableNotScoped) GetAliasName() string {
	return m.aliasName
}

func (m *mockAliasableNotScoped) SetAssigned(bool) {}

func (m *mockAliasableNotScoped) IsAssigned() bool {
	return false
}

func (m *mockAliasableNotScoped) GetGeneration() int64 {
	return m.observedGeneration
}

func (m *mockAliasableNotScoped) GetObservedGeneration() int64 {
	return m.observedGeneration
}

func (m *mockAliasableNotScoped) SetObservedGeneration(gen int64) {
	m.observedGeneration = gen
}

func (m *mockAliasableNotScoped) DeepCopyObject() runtime.Object {
	return m
}

func TestGetScope(t *testing.T) {
	tests := []struct {
		name string
		gvk  schema.GroupVersionKind
		obj  v1.Aliasable
		want string
	}{
		{
			name: "object with custom alias scope",
			gvk: schema.GroupVersionKind{
				Group:   "obot.obot.ai",
				Version: "v1",
				Kind:    "Agent",
			},
			obj: &mockAliasable{
				aliasScope: "custom-scope",
			},
			want: "custom-scope",
		},
		{
			name: "object without custom scope falls back to Kind",
			gvk: schema.GroupVersionKind{
				Group:   "obot.obot.ai",
				Version: "v1",
				Kind:    "Workflow",
			},
			obj: &mockAliasableNotScoped{
				aliasName: "", // Empty scope (doesn't implement AliasScoped)
			},
			want: "Workflow",
		},
		{
			name: "object with empty alias scope uses Kind",
			gvk: schema.GroupVersionKind{
				Group:   "obot.obot.ai",
				Version: "v1",
				Kind:    "Model",
			},
			obj: &mockAliasable{
				aliasScope: "",
			},
			want: "Model",
		},
		{
			name: "empty Kind returns empty string",
			gvk: schema.GroupVersionKind{
				Group:   "obot.obot.ai",
				Version: "v1",
				Kind:    "",
			},
			obj:  &mockAliasableNotScoped{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetScope(tt.gvk, tt.obj)
			if got != tt.want {
				t.Errorf("GetScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

// mockGVKLookup implements GVKLookup for testing
type mockGVKLookup struct {
	gvk schema.GroupVersionKind
	err error
}

func (m *mockGVKLookup) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return m.gvk, m.err
}

// mockAliasableNoRuntime implements Aliasable interfaces but NOT runtime.Object
// This is used to test the error path when an Aliasable doesn't implement runtime.Object
type mockAliasableNoRuntime struct {
	metav1.ObjectMeta
	aliasName string
}

func (m *mockAliasableNoRuntime) GetAliasName() string {
	return m.aliasName
}

func (m *mockAliasableNoRuntime) SetAssigned(bool) {}

func (m *mockAliasableNoRuntime) IsAssigned() bool {
	return false
}

func (m *mockAliasableNoRuntime) GetGeneration() int64 {
	return 0
}

func (m *mockAliasableNoRuntime) GetObservedGeneration() int64 {
	return 0
}

// Intentionally does NOT implement DeepCopyObject() to not satisfy runtime.Object

func TestName(t *testing.T) {
	tests := []struct {
		name       string
		lookup     GVKLookup
		obj        v1.Aliasable
		want       string
		wantErr    bool
		errContain string
	}{
		{
			name: "object with alias name",
			lookup: &mockGVKLookup{
				gvk: schema.GroupVersionKind{
					Group:   "obot.obot.ai",
					Version: "v1",
					Kind:    "Agent",
				},
			},
			obj: &mockAliasable{
				aliasName: "my-agent",
			},
			want:    KeyFromScopeID("Agent", "my-agent"),
			wantErr: false,
		},
		{
			name: "object with empty alias name returns empty string",
			lookup: &mockGVKLookup{
				gvk: schema.GroupVersionKind{
					Group:   "obot.obot.ai",
					Version: "v1",
					Kind:    "Workflow",
				},
			},
			obj: &mockAliasable{
				aliasName: "",
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "object with custom scope",
			lookup: &mockGVKLookup{
				gvk: schema.GroupVersionKind{
					Group:   "obot.obot.ai",
					Version: "v1",
					Kind:    "Tool",
				},
			},
			obj: &mockAliasable{
				aliasName:  "my-tool",
				aliasScope: "custom-scope",
			},
			want:    KeyFromScopeID("custom-scope", "my-tool"),
			wantErr: false,
		},
		{
			name: "lookup returns error",
			lookup: &mockGVKLookup{
				gvk: schema.GroupVersionKind{},
				err: errors.New("lookup failed"),
			},
			obj: &mockAliasable{
				aliasName: "my-agent",
			},
			want:       "",
			wantErr:    true,
			errContain: "lookup failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Name(tt.lookup, tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("Name() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errContain != "" {
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("Name() error = %v, want error containing %v", err, tt.errContain)
				}
			}
			if got != tt.want {
				t.Errorf("Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromGVK_GroupVersionKindFor(t *testing.T) {
	tests := []struct {
		name    string
		fromGVK FromGVK
		obj     runtime.Object
		want    schema.GroupVersionKind
		wantErr bool
	}{
		{
			name: "returns wrapped GVK",
			fromGVK: FromGVK{
				Group:   "obot.obot.ai",
				Version: "v1",
				Kind:    "Agent",
			},
			obj: &mockAliasable{},
			want: schema.GroupVersionKind{
				Group:   "obot.obot.ai",
				Version: "v1",
				Kind:    "Agent",
			},
			wantErr: false,
		},
		{
			name: "ignores input object",
			fromGVK: FromGVK{
				Group:   "test.io",
				Version: "v2",
				Kind:    "TestKind",
			},
			obj: nil, // Should be ignored
			want: schema.GroupVersionKind{
				Group:   "test.io",
				Version: "v2",
				Kind:    "TestKind",
			},
			wantErr: false,
		},
		{
			name: "empty GVK returns empty",
			fromGVK: FromGVK{
				Group:   "",
				Version: "",
				Kind:    "",
			},
			obj: &mockAliasable{},
			want: schema.GroupVersionKind{
				Group:   "",
				Version: "",
				Kind:    "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fromGVK.GroupVersionKindFor(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromGVK.GroupVersionKindFor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FromGVK.GroupVersionKindFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestName_Integration tests the integration between Name, GetScope, and KeyFromScopeID
func TestName_Integration(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "obot.obot.ai",
		Version: "v1",
		Kind:    "Agent",
	}
	lookup := &mockGVKLookup{gvk: gvk}

	// Test with custom scope
	objWithScope := &mockAliasable{
		aliasName:  "test-agent",
		aliasScope: "custom-scope",
	}

	got, err := Name(lookup, objWithScope)
	if err != nil {
		t.Fatalf("Name() unexpected error = %v", err)
	}

	expected := KeyFromScopeID("custom-scope", "test-agent")
	if got != expected {
		t.Errorf("Name() = %v, want %v", got, expected)
	}

	// Test without custom scope (should use Kind)
	objWithoutScope := &mockAliasableNotScoped{
		aliasName: "test-agent-2",
	}

	got2, err := Name(lookup, objWithoutScope)
	if err != nil {
		t.Fatalf("Name() unexpected error = %v", err)
	}

	expected2 := KeyFromScopeID("Agent", "test-agent-2")
	if got2 != expected2 {
		t.Errorf("Name() = %v, want %v", got2, expected2)
	}
}

// TestKeyFromScopeID_Formatting verifies the exact format of generated keys
func TestKeyFromScopeID_Formatting(t *testing.T) {
	testCases := []struct {
		scope string
		id    string
	}{
		{"Agent", "my-agent"},
		{"Workflow", "test-workflow"},
		{"Tool", "special-tool"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s/%s", tc.scope, tc.id), func(t *testing.T) {
			key := KeyFromScopeID(tc.scope, tc.id)

			// Verify format: al1[8 hex chars]
			if len(key) != 11 {
				t.Errorf("Expected length 11, got %d: %s", len(key), key)
			}

			if !strings.HasPrefix(key, "al1") {
				t.Errorf("Expected prefix 'al1', got: %s", key)
			}

			// Verify hash portion is hex
			hashPart := key[3:]
			if len(hashPart) != 8 {
				t.Errorf("Expected hash length 8, got %d: %s", len(hashPart), hashPart)
			}

			// All chars should be lowercase hex
			for i, c := range hashPart {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("Character at position %d is not lowercase hex: %c", i, c)
				}
			}
		})
	}
}
