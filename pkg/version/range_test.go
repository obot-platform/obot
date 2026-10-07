package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInRange(t *testing.T) {
	tests := []struct {
		name       string
		tag        string
		minVersion string
		maxVersion string
		want       bool
	}{
		{
			name: "unrestricted",
			tag:  "v0.20.0",
			want: true,
		},
		{
			name:       "dev build ignores range",
			tag:        "v0.0.0-dev",
			minVersion: "v99.0.0",
			want:       true,
		},
		{
			name:       "dev build with commit suffix ignores range",
			tag:        "v0.0.0-20260101-abcdef",
			maxVersion: "v0.1.0",
			want:       true,
		},
		{
			name:       "equal to min",
			tag:        "v0.20.0",
			minVersion: "v0.20.0",
			want:       true,
		},
		{
			name:       "below min",
			tag:        "v0.19.3",
			minVersion: "v0.20.0",
			want:       false,
		},
		{
			name:       "equal to max",
			tag:        "v0.20.0",
			maxVersion: "v0.20.0",
			want:       true,
		},
		{
			name:       "above max",
			tag:        "v0.20.1",
			maxVersion: "v0.20.0",
			want:       false,
		},
		{
			name:       "within min and max",
			tag:        "v0.21.0",
			minVersion: "v0.20.0",
			maxVersion: "v0.22.0",
			want:       true,
		},
		{
			name:       "release candidate counts as its release",
			tag:        "v0.20.0-rc1",
			minVersion: "v0.20.0",
			want:       true,
		},
		{
			name:       "invalid bound is ignored",
			tag:        "v0.19.0",
			minVersion: "not-a-version",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, InRange(tt.tag, tt.minVersion, tt.maxVersion))
		})
	}
}

func TestValidateRange(t *testing.T) {
	tests := []struct {
		name       string
		minVersion string
		maxVersion string
		wantErr    string
	}{
		{
			name: "empty",
		},
		{
			name:       "valid range",
			minVersion: "v0.20.0",
			maxVersion: "v0.21.0",
		},
		{
			name:       "missing v prefix",
			minVersion: "0.20.0",
			wantErr:    "must be a full release version",
		},
		{
			name:       "invalid min",
			minVersion: "latest",
			wantErr:    "invalid minObotVersion",
		},
		{
			name:       "invalid max",
			maxVersion: ">=0.20.0",
			wantErr:    "invalid maxObotVersion",
		},
		{
			name:       "shorthand version",
			maxVersion: "v0.21",
			wantErr:    "must be a full release version",
		},
		{
			name:       "pre-release bound",
			minVersion: "v0.21.0-rc2",
			wantErr:    "must be a full release version",
		},
		{
			name:       "build metadata bound",
			minVersion: "v0.21.0+abc",
			wantErr:    "must be a full release version",
		},
		{
			name:       "min greater than max",
			minVersion: "v0.21.0",
			maxVersion: "v0.20.0",
			wantErr:    "must not be greater than",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRange(tt.minVersion, tt.maxVersion)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
