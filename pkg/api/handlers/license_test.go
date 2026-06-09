package handlers

import "testing"

func TestDisplayLicenseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		licenseKey     string
		canViewPartial bool
		want           string
	}{
		{
			name:           "empty key",
			licenseKey:     "",
			canViewPartial: true,
			want:           "",
		},
		{
			name:           "non admin masks key",
			licenseKey:     "keygen/abc123invalid",
			canViewPartial: false,
			want:           licenseKeyMask,
		},
		{
			name:           "admin shows suffix",
			licenseKey:     "keygen/abc123j13lasds",
			canViewPartial: true,
			want:           "****j13lasds",
		},
		{
			name:           "admin short key is fully masked",
			licenseKey:     "shortkey",
			canViewPartial: true,
			want:           licenseKeyMask,
		},
		{
			name:           "trims whitespace",
			licenseKey:     "  keygen/license-key  ",
			canViewPartial: false,
			want:           licenseKeyMask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := displayLicenseKey(tt.licenseKey, tt.canViewPartial); got != tt.want {
				t.Fatalf("displayLicenseKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
