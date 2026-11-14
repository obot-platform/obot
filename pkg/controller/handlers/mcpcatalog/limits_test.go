package mcpcatalog

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/obot-platform/nah/pkg/apply"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// applier embeds apply.Apply under a name that doesn't collide with its Apply method.
type applier = apply.Apply

// limitedApply applies objects like storage with a compute limit: an apply that includes a vMCP in refuse fails as a
// whole, and failWith fails every apply.
type limitedApply struct {
	applier
	refuse   []string
	failWith error
	applied  []string
}

func (a *limitedApply) Apply(_ context.Context, _ kclient.Object, objs ...kclient.Object) error {
	if a.failWith != nil {
		return a.failWith
	}
	for _, obj := range objs {
		if _, ok := obj.(*v1.VMCP); ok && slices.Contains(a.refuse, obj.GetName()) {
			return &apierrors.StatusError{ErrStatus: metav1.Status{
				Code:    http.StatusPaymentRequired,
				Message: "There isn't room in the plan for this.",
			}}
		}
	}
	for _, obj := range objs {
		a.applied = append(a.applied, obj.GetName())
	}
	return nil
}

func TestApplyWithinLimits(t *testing.T) {
	vmcp := func(name, sourceURL string) *v1.VMCP {
		return &v1.VMCP{
			Name: name,
			Spec: v1.VMCPSpec{
				SourceURL: sourceURL,
			},
		}
	}
	objs := []kclient.Object{
		&v1.MCPServerCatalogEntry{Name: "entry"},
		vmcp("fits", "https://example.com/a"),
		vmcp("too-big", "https://example.com/a"),
		vmcp("also-too-big", "https://example.com/b"),
	}
	failure := errors.New("storage is down")

	for _, tt := range []struct {
		name        string
		refuse      []string
		failWith    error
		wantApplied []string
		wantRefused map[string]string
		wantErr     error
	}{
		{
			name:        "everything fits",
			wantApplied: []string{"entry", "fits", "too-big", "also-too-big"},
		},
		{
			name:        "vMCPs that don't fit are left out and reported by source",
			refuse:      []string{"too-big", "also-too-big"},
			wantApplied: []string{"entry", "fits"},
			wantRefused: map[string]string{
				"https://example.com/a": "vMCP too-big: There isn't room in the plan for this.",
				"https://example.com/b": "vMCP also-too-big: There isn't room in the plan for this.",
			},
		},
		{
			name:     "any other failure fails the apply",
			failWith: failure,
			wantErr:  failure,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := &limitedApply{
				refuse:   tt.refuse,
				failWith: tt.failWith,
			}

			refused, err := applyWithinLimits(t.Context(), app, &v1.MCPCatalog{}, objs)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantApplied, app.applied)
			if len(tt.wantRefused) == 0 {
				require.Empty(t, refused)
			} else {
				require.Equal(t, tt.wantRefused, refused)
			}
		})
	}
}
