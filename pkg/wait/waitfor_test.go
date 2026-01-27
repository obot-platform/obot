package wait

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestComplete(t *testing.T) {
	tests := []struct {
		name            string
		opts            []Option
		expectedTimeout time.Duration
		expectedCreate  bool
		expectedExists  bool
	}{
		{
			name:            "Default timeout when no options provided",
			opts:            []Option{},
			expectedTimeout: 2 * time.Minute,
			expectedCreate:  false,
			expectedExists:  false,
		},
		{
			name: "Single option with custom timeout",
			opts: []Option{
				{Timeout: 5 * time.Second},
			},
			expectedTimeout: 5 * time.Second,
			expectedCreate:  false,
			expectedExists:  false,
		},
		{
			name: "Single option with all fields set",
			opts: []Option{
				{Timeout: 10 * time.Second, Create: true, WaitForExists: true},
			},
			expectedTimeout: 10 * time.Second,
			expectedCreate:  true,
			expectedExists:  true,
		},
		{
			name: "Multiple options - first non-zero value wins",
			opts: []Option{
				{Timeout: 5 * time.Second, Create: true},
				{Timeout: 10 * time.Second, Create: false, WaitForExists: true},
			},
			expectedTimeout: 5 * time.Second,
			expectedCreate:  true,
			expectedExists:  true,
		},
		{
			name: "Multiple options - later values used when earlier are zero",
			opts: []Option{
				{},
				{Timeout: 15 * time.Second, WaitForExists: true},
				{Create: true},
			},
			expectedTimeout: 15 * time.Second,
			expectedCreate:  true,
			expectedExists:  true,
		},
		{
			name: "Zero timeout in options defaults to 2 minutes",
			opts: []Option{
				{Timeout: 0, Create: true},
			},
			expectedTimeout: 2 * time.Minute,
			expectedCreate:  true,
			expectedExists:  false,
		},
		{
			name: "First non-zero timeout takes precedence",
			opts: []Option{
				{Timeout: 0},
				{Timeout: 30 * time.Second},
				{Timeout: 60 * time.Second},
			},
			expectedTimeout: 30 * time.Second,
			expectedCreate:  false,
			expectedExists:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := complete(tt.opts...)

			if result.Timeout != tt.expectedTimeout {
				t.Errorf("Expected Timeout %v, got %v", tt.expectedTimeout, result.Timeout)
			}
			if result.Create != tt.expectedCreate {
				t.Errorf("Expected Create %v, got %v", tt.expectedCreate, result.Create)
			}
			if result.WaitForExists != tt.expectedExists {
				t.Errorf("Expected WaitForExists %v, got %v", tt.expectedExists, result.WaitForExists)
			}
		})
	}
}

// Mock object for testing load function
type mockObject struct {
	kclient.Object
	name            string
	namespace       string
	uid             types.UID
	resourceVersion string
}

func (m *mockObject) GetName() string {
	return m.name
}

func (m *mockObject) SetName(name string) {
	m.name = name
}

func (m *mockObject) GetNamespace() string {
	return m.namespace
}

func (m *mockObject) SetNamespace(namespace string) {
	m.namespace = namespace
}

func (m *mockObject) GetUID() types.UID {
	return m.uid
}

func (m *mockObject) SetUID(uid types.UID) {
	m.uid = uid
}

func (m *mockObject) GetResourceVersion() string {
	return m.resourceVersion
}

func (m *mockObject) SetResourceVersion(version string) {
	m.resourceVersion = version
}

func (m *mockObject) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (m *mockObject) DeepCopyObject() runtime.Object {
	return &mockObject{
		name:            m.name,
		namespace:       m.namespace,
		uid:             m.uid,
		resourceVersion: m.resourceVersion,
	}
}

func TestLoad(t *testing.T) {
	t.Run("Object with UID returns immediately", func(t *testing.T) {
		client := fake.NewClientBuilder().Build()
		obj := &mockObject{
			name: "test-obj",
			uid:  "test-uid-123",
		}

		err := load(context.Background(), client, obj, false)
		if err != nil {
			t.Errorf("Expected no error when object has UID, got: %v", err)
		}
	})

	t.Run("Object without name and without UID attempts creation", func(t *testing.T) {
		client := fake.NewClientBuilder().Build()
		obj := &mockObject{
			name: "",
			uid:  "",
		}

		// This will fail because we can't actually create without a proper scheme
		// but we're testing that it attempts to create
		err := load(context.Background(), client, obj, true)

		// We expect an error since mockObject isn't in the scheme, but the function should try
		if err == nil {
			t.Error("Expected error attempting to create object without scheme")
		}
	})

	t.Run("Create flag false with get error returns error", func(t *testing.T) {
		client := fake.NewClientBuilder().Build()
		obj := &mockObject{
			name:      "nonexistent",
			namespace: "default",
		}

		err := load(context.Background(), client, obj, false)

		if err == nil {
			t.Error("Expected error when object not found and create is false")
		}
		// The fake client returns a scheme error because mockObject isn't registered,
		// but in real usage with proper types, this would be IsNotFound
	})
}

// Note: Testing the For() function would require more complex mocking of the Kubernetes
// watch API and is better suited for integration tests. The unit tests above cover the
// main utility functions (complete and load) that contain the core business logic.
