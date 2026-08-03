package types

import (
	"math"
	"strings"
	"testing"
)

func TestHostedAgentResourceQuantityValidate(t *testing.T) {
	valid := HostedAgentResourceQuantity{
		CPUVCPUs:     4,
		MemoryBytes:  8 << 30,
		StorageBytes: 100 << 30,
	}

	tests := []struct {
		name    string
		value   HostedAgentResourceQuantity
		wantErr string
	}{
		{name: "valid", value: valid},
		{name: "zero cpu", value: HostedAgentResourceQuantity{MemoryBytes: valid.MemoryBytes, StorageBytes: valid.StorageBytes}, wantErr: "cpuVcpus"},
		{name: "negative cpu", value: HostedAgentResourceQuantity{CPUVCPUs: -1, MemoryBytes: valid.MemoryBytes, StorageBytes: valid.StorageBytes}, wantErr: "cpuVcpus"},
		{name: "nan cpu", value: HostedAgentResourceQuantity{CPUVCPUs: math.NaN(), MemoryBytes: valid.MemoryBytes, StorageBytes: valid.StorageBytes}, wantErr: "cpuVcpus"},
		{name: "infinite cpu", value: HostedAgentResourceQuantity{CPUVCPUs: math.Inf(1), MemoryBytes: valid.MemoryBytes, StorageBytes: valid.StorageBytes}, wantErr: "cpuVcpus"},
		{name: "zero memory", value: HostedAgentResourceQuantity{CPUVCPUs: valid.CPUVCPUs, StorageBytes: valid.StorageBytes}, wantErr: "memoryBytes"},
		{name: "zero storage", value: HostedAgentResourceQuantity{CPUVCPUs: valid.CPUVCPUs, MemoryBytes: valid.MemoryBytes}, wantErr: "storageBytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.value.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestHostedAgentPoolAssignmentManifestValidate(t *testing.T) {
	tests := []struct {
		name    string
		value   HostedAgentPoolAssignmentManifest
		wantErr string
	}{
		{name: "valid", value: HostedAgentPoolAssignmentManifest{UserID: "user-1", PoolID: "pool-1"}},
		{name: "user required", value: HostedAgentPoolAssignmentManifest{PoolID: "pool-1"}, wantErr: "userID"},
		{name: "pool required", value: HostedAgentPoolAssignmentManifest{UserID: "user-1"}, wantErr: "poolID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.value.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
