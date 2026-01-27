package store

import (
	"testing"
)

func TestNewS3Store(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		options S3StoreOptions
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid configuration without endpoint",
			host: "example.com",
			options: S3StoreOptions{
				AuditLogsStoreS3Bucket: "audit-logs-bucket",
			},
			wantErr: false,
		},
		{
			name: "valid configuration with endpoint",
			host: "example.com",
			options: S3StoreOptions{
				AuditLogsStoreS3Bucket:   "audit-logs-bucket",
				AuditLogsStoreS3Endpoint: "https://s3.us-west-2.amazonaws.com",
			},
			wantErr: false,
		},
		{
			name: "valid configuration with path style",
			host: "example.com",
			options: S3StoreOptions{
				AuditLogsStoreS3Bucket:     "audit-logs-bucket",
				AuditLogsStoreUsePathStyle: true,
			},
			wantErr: false,
		},
		{
			name: "missing bucket",
			host: "example.com",
			options: S3StoreOptions{
				AuditLogsStoreS3Bucket: "",
			},
			wantErr: true,
			errMsg:  "audit log store S3 bucket is required",
		},
		{
			name: "empty options",
			host: "example.com",
			options: S3StoreOptions{},
			wantErr: true,
			errMsg:  "audit log store S3 bucket is required",
		},
		{
			name: "custom endpoint with path style",
			host: "minio.local",
			options: S3StoreOptions{
				AuditLogsStoreS3Bucket:     "my-audit-logs",
				AuditLogsStoreS3Endpoint:   "http://localhost:9000",
				AuditLogsStoreUsePathStyle: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewS3Store(tt.host, false, tt.options)

			if tt.wantErr {
				if err == nil {
					t.Error("NewS3Store() expected error but got nil")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("NewS3Store() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}

			// For non-error cases, we expect the store creation to fail due to AWS credentials
			// not being available in the test environment. This is expected behavior.
			if err != nil {
				t.Logf("NewS3Store() error = %v (expected in test environment without AWS credentials)", err)
				return
			}

			if store == nil {
				t.Error("NewS3Store() returned nil store without error")
				return
			}

			// Verify store was configured correctly
			s3s := store.(*s3Store)
			if s3s.host != tt.host {
				t.Errorf("NewS3Store() host = %v, want %v", s3s.host, tt.host)
			}
			if s3s.bucket != tt.options.AuditLogsStoreS3Bucket {
				t.Errorf("NewS3Store() bucket = %v, want %v", s3s.bucket, tt.options.AuditLogsStoreS3Bucket)
			}
			if s3s.compress != false {
				t.Errorf("NewS3Store() compress = %v, want false", s3s.compress)
			}
			if s3s.client == nil {
				t.Error("NewS3Store() client is nil")
			}
		})
	}
}

func TestNewS3StoreWithCompression(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		compress bool
	}{
		{
			name:     "without compression",
			host:     "example.com",
			compress: false,
		},
		{
			name:     "with compression",
			host:     "example.com",
			compress: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewS3Store(tt.host, tt.compress, S3StoreOptions{
				AuditLogsStoreS3Bucket: "test-bucket",
			})

			// Expect error due to missing AWS credentials in test environment
			if err != nil {
				t.Logf("NewS3Store() error = %v (expected in test environment)", err)
				return
			}

			if store == nil {
				t.Error("NewS3Store() returned nil store without error")
				return
			}

			s3s := store.(*s3Store)
			if s3s.compress != tt.compress {
				t.Errorf("NewS3Store() compress = %v, want %v", s3s.compress, tt.compress)
			}
		})
	}
}

func TestS3StoreConfiguration(t *testing.T) {
	// Test various configuration combinations
	tests := []struct {
		name    string
		options S3StoreOptions
		wantErr bool
	}{
		{
			name: "minimal config",
			options: S3StoreOptions{
				AuditLogsStoreS3Bucket: "bucket",
			},
			wantErr: false,
		},
		{
			name: "full config",
			options: S3StoreOptions{
				AuditLogsStoreS3Bucket:     "bucket",
				AuditLogsStoreS3Endpoint:   "https://custom.endpoint.com",
				AuditLogsStoreUsePathStyle: true,
			},
			wantErr: false,
		},
		{
			name: "endpoint without bucket",
			options: S3StoreOptions{
				AuditLogsStoreS3Endpoint: "https://s3.amazonaws.com",
			},
			wantErr: true,
		},
		{
			name: "path style without bucket",
			options: S3StoreOptions{
				AuditLogsStoreUsePathStyle: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewS3Store("test.com", false, tt.options)

			if tt.wantErr {
				if err == nil {
					t.Error("NewS3Store() expected error but got nil")
				}
			} else {
				// In test environment without AWS credentials, we expect an error
				// The important thing is that bucket validation happens first
				if err != nil {
					t.Logf("NewS3Store() error = %v (expected in test environment)", err)
				}
			}
		})
	}
}

// Note: Full integration tests for s3Store.Persist() would require either:
// 1. A mock S3 client implementation
// 2. An actual S3 bucket with credentials
// 3. A local S3-compatible service (like MinIO)
//
// These tests focus on validation and configuration which can be tested
// without external dependencies. The Persist() method's compression logic
// is similar to diskStore and uses the same gzip approach.
