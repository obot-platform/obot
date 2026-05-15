package imagepullsecret

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/imagepullsecrets"
	obotv1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPopulateECRComputedStatusUsesObotServiceAccount(t *testing.T) {
	handler := New(nil, nil, "kubernetes", "obot-mcp", "obot", "obot", nil, "https://issuer.example.com")
	secret := &obotv1.ImagePullSecret{
		Spec: obotv1.ImagePullSecretSpec{
			ECR: &types.ECRImagePullSecretConfig{},
		},
	}
	var status obotv1.ImagePullSecretStatus

	handler.populateECRComputedStatus(secret, &status)

	if status.Subject != "system:serviceaccount:obot:obot" {
		t.Fatalf("unexpected ECR subject: %q", status.Subject)
	}
}

func TestShouldRefreshECRHonorsManualRequest(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	lastSuccess := metav1.NewTime(now.Add(-time.Hour))
	lastReconciled := metav1.NewTime(now.Add(-time.Minute))
	secret := &obotv1.ImagePullSecret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				imagepullsecrets.AnnotationECRRefreshRequestedAt: now.Format(time.RFC3339Nano),
			},
		},
		Spec: obotv1.ImagePullSecretSpec{
			ECR: &types.ECRImagePullSecretConfig{
				RefreshSchedule: "0 0 * * *",
			},
		},
		Status: obotv1.ImagePullSecretStatus{
			LastSuccessTime:    &lastSuccess,
			LastReconciledTime: &lastReconciled,
		},
	}
	handler := &Handler{now: func() time.Time { return now }}

	if !handler.shouldRefreshECR(secret, false) {
		t.Fatal("expected manual refresh request to force refresh")
	}

	reconciledAfterRequest := metav1.NewTime(now.Add(time.Minute))
	secret.Status.LastReconciledTime = &reconciledAfterRequest
	if handler.shouldRefreshECR(secret, false) {
		t.Fatal("did not expect already-observed manual refresh request to force refresh")
	}
}
