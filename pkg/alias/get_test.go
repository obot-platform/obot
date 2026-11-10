package alias

import (
	"context"
	"errors"
	"fmt"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestGetAliasLookupFailure(t *testing.T) {
	failure := errors.New("alias storage failed")
	var missing error
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if _, ok := obj.(*v1.Alias); ok {
				return failure
			}
			missing = c.Get(ctx, key, obj, opts...)
			return missing
		},
	}).Build()
	err := Get(t.Context(), c, &v1.Model{}, system.DefaultNamespace, "m1-missing")
	require.True(t, apierrors.IsNotFound(missing))
	require.EqualError(t, err, fmt.Sprintf("%v\n%v", missing, failure), "retain both lookup diagnostics")
	require.ErrorIs(t, err, failure)
	require.False(t, apierrors.IsNotFound(err), "the secondary storage fault must prevent NotFound fallback")
	require.NotErrorIs(t, err, missing)
}

func TestGetLookupPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name          string
		primaryExists bool
		aliasExists   bool
		wrongKind     bool
		wantModel     string
	}{
		{
			name:          "resource precedes alias",
			primaryExists: true,
			aliasExists:   true,
			wantModel:     "m1-primary",
		},
		{
			name:        "missing resource follows alias",
			aliasExists: true,
			wantModel:   "m1-target",
		},
		{
			name: "both absent retain NotFound",
		},
		{
			name:        "wrong alias kind retains NotFound",
			aliasExists: true,
			wrongKind:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			require.NoError(t, c.Create(t.Context(), &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-target"}))
			if tc.primaryExists {
				require.NoError(t, c.Create(t.Context(), &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-primary"}))
			}
			if tc.aliasExists {
				kind := "Model"
				if tc.wrongKind {
					kind = "DefaultModelAlias"
				}
				require.NoError(t, c.Create(t.Context(), &v1.Alias{Name: KeyFromScopeID("Model", "m1-primary"), Spec: v1.AliasSpec{
					TargetKind:      kind,
					TargetNamespace: system.DefaultNamespace,
					TargetName:      "m1-target",
				}}))
			}
			var got v1.Model
			err := Get(t.Context(), c, &got, system.DefaultNamespace, "m1-primary")
			if tc.wantModel == "" {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantModel, got.Name)
			}
		})
	}
}
