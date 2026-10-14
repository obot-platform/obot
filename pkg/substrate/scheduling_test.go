package substrate

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestValidateWorkerScheduling(t *testing.T) {
	for _, tc := range []struct {
		name        string
		selector    map[string]string
		tolerations []corev1.Toleration
		wantError   string
	}{
		{
			name:     "valid",
			selector: map[string]string{"kubernetes.io/os": "linux"},
			tolerations: []corev1.Toleration{
				{
					Key:    "dedicated",
					Value:  "agents",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:               "maintenance",
					Operator:          corev1.TolerationOpExists,
					Effect:            corev1.TaintEffectNoExecute,
					TolerationSeconds: new(int64(60)),
				},
			},
		},
		{
			name:      "invalid selector key",
			selector:  map[string]string{"bad key": "agents"},
			wantError: "valid Kubernetes labels",
		},
		{
			name:      "invalid selector value",
			selector:  map[string]string{"dedicated": "bad value"},
			wantError: "valid Kubernetes labels",
		},
		{
			name: "invalid key",
			tolerations: []corev1.Toleration{{
				Key: "bad key",
			}},
			wantError: "toleration key",
		},
		{
			name:        "empty key requires Exists",
			tolerations: []corev1.Toleration{{}},
			wantError:   "empty key",
		},
		{
			name: "invalid value",
			tolerations: []corev1.Toleration{{
				Key:   "dedicated",
				Value: "bad value",
			}},
			wantError: "toleration value",
		},
		{
			name: "Exists cannot have value",
			tolerations: []corev1.Toleration{{
				Key:      "dedicated",
				Operator: corev1.TolerationOpExists,
				Value:    "agents",
			}},
			wantError: "must not specify a value",
		},
		{
			name: "invalid operator",
			tolerations: []corev1.Toleration{{
				Key:      "dedicated",
				Operator: "Invalid",
			}},
			wantError: "operator",
		},
		{
			name: "invalid effect",
			tolerations: []corev1.Toleration{{
				Key:    "dedicated",
				Effect: "Invalid",
			}},
			wantError: "effect",
		},
		{
			name: "seconds require NoExecute",
			tolerations: []corev1.Toleration{{
				Key:               "dedicated",
				TolerationSeconds: new(int64(60)),
			}},
			wantError: "NoExecute",
		},
		{
			name:        "too many",
			tolerations: make([]corev1.Toleration, 17),
			wantError:   "at most 16",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWorkerScheduling(tc.selector, tc.tolerations)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
