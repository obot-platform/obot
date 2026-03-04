package blob

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

// Compile-time interface compliance checks.
var (
	_ BlobStore = (*S3Store)(nil)
	_ BlobStore = (*GCSStore)(nil)
	_ BlobStore = (*AzureStore)(nil)
	_ BlobStore = (*CustomS3Store)(nil)
	_ BlobStore = (*MockBlobStore)(nil)
)

func TestNew_ValidProviders(t *testing.T) {
	tests := []struct {
		name         string
		providerType types.StorageProviderType
		config       types.StorageConfig
	}{
		{
			name:         "s3",
			providerType: types.StorageProviderS3,
			config:       types.StorageConfig{S3Config: &types.S3Config{Region: "us-east-1"}},
		},
		{
			name:         "gcs",
			providerType: types.StorageProviderGCS,
			config:       types.StorageConfig{GCSConfig: &types.GCSConfig{}},
		},
		{
			name:         "azure",
			providerType: types.StorageProviderAzureBlob,
			config:       types.StorageConfig{AzureConfig: &types.AzureConfig{StorageAccount: "test"}},
		},
		{
			name:         "custom_s3",
			providerType: types.StorageProviderCustomS3,
			config: types.StorageConfig{CustomS3Config: &types.CustomS3Config{
				Endpoint:        "http://localhost:9000",
				Region:          "us-east-1",
				AccessKeyID:     "key",
				SecretAccessKey: "secret",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := New(tt.providerType, tt.config)
			if err != nil {
				t.Fatalf("New() returned error: %v", err)
			}
			if store == nil {
				t.Fatal("New() returned nil store")
			}
		})
	}
}

func TestNew_UnsupportedProvider(t *testing.T) {
	_, err := New("unsupported", types.StorageConfig{})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNew_MissingConfig(t *testing.T) {
	tests := []struct {
		name         string
		providerType types.StorageProviderType
		config       types.StorageConfig
	}{
		{
			name:         "s3 nil config",
			providerType: types.StorageProviderS3,
			config:       types.StorageConfig{},
		},
		{
			name:         "gcs nil config",
			providerType: types.StorageProviderGCS,
			config:       types.StorageConfig{},
		},
		{
			name:         "azure nil config",
			providerType: types.StorageProviderAzureBlob,
			config:       types.StorageConfig{},
		},
		{
			name:         "custom_s3 nil config",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.providerType, tt.config)
			if err == nil {
				t.Fatal("expected error for missing config")
			}
		})
	}
}

func TestNew_ValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		providerType types.StorageProviderType
		config       types.StorageConfig
	}{
		{
			name:         "s3 missing region",
			providerType: types.StorageProviderS3,
			config:       types.StorageConfig{S3Config: &types.S3Config{}},
		},
		{
			name:         "azure missing storage account",
			providerType: types.StorageProviderAzureBlob,
			config:       types.StorageConfig{AzureConfig: &types.AzureConfig{}},
		},
		{
			name:         "custom_s3 missing endpoint",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{CustomS3Config: &types.CustomS3Config{Region: "us-east-1", AccessKeyID: "k", SecretAccessKey: "s"}},
		},
		{
			name:         "custom_s3 missing region",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{CustomS3Config: &types.CustomS3Config{Endpoint: "http://localhost", AccessKeyID: "k", SecretAccessKey: "s"}},
		},
		{
			name:         "custom_s3 missing credentials",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{CustomS3Config: &types.CustomS3Config{Endpoint: "http://localhost", Region: "us-east-1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.providerType, tt.config)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMockBlobStore_UploadAndDownload(t *testing.T) {
	m := NewMockBlobStore()
	ctx := context.Background()

	data := []byte("hello world")
	if err := m.Upload(ctx, "bucket", "key", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload() error: %v", err)
	}

	if m.UploadCalls != 1 {
		t.Fatalf("expected 1 upload call, got %d", m.UploadCalls)
	}

	rc, err := m.Download(ctx, "bucket", "key")
	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
	if m.DownloadCalls != 1 {
		t.Fatalf("expected 1 download call, got %d", m.DownloadCalls)
	}
}

func TestMockBlobStore_Delete(t *testing.T) {
	m := NewMockBlobStore()
	ctx := context.Background()

	if err := m.Upload(ctx, "bucket", "key", bytes.NewReader([]byte("data"))); err != nil {
		t.Fatalf("Upload() error: %v", err)
	}
	if m.ObjectCount() != 1 {
		t.Fatalf("expected 1 object, got %d", m.ObjectCount())
	}

	if err := m.Delete(ctx, "bucket", "key"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if m.DeleteCalls != 1 {
		t.Fatalf("expected 1 delete call, got %d", m.DeleteCalls)
	}
	if m.ObjectCount() != 0 {
		t.Fatalf("expected 0 objects after delete, got %d", m.ObjectCount())
	}
}

func TestMockBlobStore_DownloadNotFound(t *testing.T) {
	m := NewMockBlobStore()
	_, err := m.Download(context.Background(), "bucket", "missing")
	if err == nil {
		t.Fatal("expected error for missing object")
	}
}

func TestMockBlobStore_InjectedErrors(t *testing.T) {
	m := NewMockBlobStore()
	ctx := context.Background()
	injected := fmt.Errorf("injected")

	m.UploadErr = injected
	if err := m.Upload(ctx, "b", "k", bytes.NewReader(nil)); err != injected {
		t.Fatalf("expected injected upload error, got: %v", err)
	}

	m.DownloadErr = injected
	if _, err := m.Download(ctx, "b", "k"); err != injected {
		t.Fatalf("expected injected download error, got: %v", err)
	}

	m.DeleteErr = injected
	if err := m.Delete(ctx, "b", "k"); err != injected {
		t.Fatalf("expected injected delete error, got: %v", err)
	}

	m.TestErr = injected
	if err := m.Test(ctx); err != injected {
		t.Fatalf("expected injected test error, got: %v", err)
	}
}

func TestMockBlobStore_GetObject(t *testing.T) {
	m := NewMockBlobStore()
	ctx := context.Background()

	if _, ok := m.GetObject("b", "k"); ok {
		t.Fatal("expected not found before upload")
	}

	data := []byte("content")
	if err := m.Upload(ctx, "b", "k", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload() error: %v", err)
	}

	got, ok := m.GetObject("b", "k")
	if !ok {
		t.Fatal("expected object to be found after upload")
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}
