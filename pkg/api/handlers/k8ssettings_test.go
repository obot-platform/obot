package handlers

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseResourceMaximumField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    *resource.Quantity
		wantErr bool
	}{
		{name: "empty"},
		{name: "CPU", value: "500m", want: new(resource.MustParse("500m"))},
		{name: "memory", value: "2Gi", want: new(resource.MustParse("2Gi"))},
		{name: "invalid", value: "invalid", wantErr: true},
		{name: "negative", value: "-1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseResourceMaximumField("maximum", tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseResourceMaximumField() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("parseResourceMaximumField() = %s, want nil", got)
				}
				return
			}
			if got == nil || got.Cmp(*tt.want) != 0 {
				t.Fatalf("parseResourceMaximumField() = %v, want %s", got, tt.want)
			}
		})
	}
}
