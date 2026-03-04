package blob

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

// MockBlobStore is an in-memory BlobStore implementation for testing.
// It records all calls and stores objects in a map keyed by "bucket/key".
type MockBlobStore struct {
	mu      sync.Mutex
	objects map[string][]byte

	UploadCalls   int
	DownloadCalls int
	DeleteCalls   int
	TestCalls     int

	// Set these to inject errors into specific operations.
	UploadErr   error
	DownloadErr error
	DeleteErr   error
	TestErr     error
}

// NewMockBlobStore creates a new MockBlobStore.
func NewMockBlobStore() *MockBlobStore {
	return &MockBlobStore{
		objects: make(map[string][]byte),
	}
}

func (m *MockBlobStore) Upload(ctx context.Context, bucket, key string, data io.Reader) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UploadCalls++
	if m.UploadErr != nil {
		return m.UploadErr
	}

	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	m.objects[bucket+"/"+key] = b
	return nil
}

func (m *MockBlobStore) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DownloadCalls++
	if m.DownloadErr != nil {
		return nil, m.DownloadErr
	}

	data, ok := m.objects[bucket+"/"+key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s/%s", bucket, key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MockBlobStore) Delete(ctx context.Context, bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteCalls++
	if m.DeleteErr != nil {
		return m.DeleteErr
	}

	delete(m.objects, bucket+"/"+key)
	return nil
}

func (m *MockBlobStore) Test(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TestCalls++
	return m.TestErr
}

// GetObject returns the stored bytes for a given bucket and key, or false if not found.
func (m *MockBlobStore) GetObject(bucket, key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.objects[bucket+"/"+key]
	return data, ok
}

// ObjectCount returns the number of stored objects.
func (m *MockBlobStore) ObjectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.objects)
}
