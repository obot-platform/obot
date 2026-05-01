package mcpsecretbinding

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const obotNS = "obot-ns"

func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		Build()
}

// newSecret builds a Secret in obotNS.
func newSecret(name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: obotNS},
		Data:       make(map[string][]byte, len(data)),
	}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

// newServer builds an MCPServer in system.DefaultNamespace with one env
// secretBinding pointing at secretName/secretKey.
func newServer(name, secretName, secretKey string) *v1.MCPServer {
	return &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				Env: []types.MCPEnv{{
					MCPHeader: types.MCPHeader{
						Key: "MY_KEY",
						SecretBinding: &types.MCPSecretBinding{
							Name: secretName,
							Key:  secretKey,
						},
					},
				}},
			},
		},
	}
}

func request(ctx context.Context, obj kclient.Object, ns, name string) router.Request {
	return router.Request{Ctx: ctx, Object: obj, Namespace: ns, Name: name}
}

func getServer(t *testing.T, client kclient.Client, name string) v1.MCPServer {
	t.Helper()
	var s v1.MCPServer
	require.NoError(t, client.Get(context.Background(),
		kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, &s))
	return s
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestSecretChanged_EmptyObotNamespaceIsNoop(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, "", nil)

	secret := newSecret("my-secret", map[string]string{"tok": "v1"})
	err := h.SecretChanged(request(context.Background(), secret, obotNS, "my-secret"), nil)
	require.NoError(t, err)

	// Annotation must not have been set.
	s := getServer(t, storage, "srv")
	assert.Empty(t, s.Annotations[rotationAnnotation])
}

func TestSecretChanged_WrongNamespaceIsNoop(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	secret := newSecret("my-secret", map[string]string{"tok": "v1"})
	secret.Namespace = "other-ns"

	err := h.SecretChanged(request(context.Background(), secret, "other-ns", "my-secret"), nil)
	require.NoError(t, err)

	s := getServer(t, storage, "srv")
	assert.Empty(t, s.Annotations[rotationAnnotation])
}

func TestSecretChanged_UnreferencedSecretIsNoop(t *testing.T) {
	// Server binds "my-secret" but the event is for "other-secret".
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	other := newSecret("other-secret", map[string]string{"tok": "v1"})
	err := h.SecretChanged(request(context.Background(), other, obotNS, "other-secret"), nil)
	require.NoError(t, err)

	s := getServer(t, storage, "srv")
	assert.Empty(t, s.Annotations[rotationAnnotation])
}

func TestSecretChanged_BumpsAnnotationOnFirstChange(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	secret := newSecret("my-secret", map[string]string{"tok": "hello"})
	err := h.SecretChanged(request(context.Background(), secret, obotNS, "my-secret"), nil)
	require.NoError(t, err)

	s := getServer(t, storage, "srv")
	assert.NotEmpty(t, s.Annotations[rotationAnnotation])
}

func TestSecretChanged_IdempotentWhenReferencedValueUnchanged(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	secret := newSecret("my-secret", map[string]string{"tok": "hello"})

	// First call — sets the annotation.
	require.NoError(t, h.SecretChanged(request(context.Background(), secret, obotNS, "my-secret"), nil))
	s1 := getServer(t, storage, "srv")
	hash1 := s1.Annotations[rotationAnnotation]
	require.NotEmpty(t, hash1)

	// Second call with identical secret data — annotation must not change and
	// no additional Update should be issued (same hash → skip).
	require.NoError(t, h.SecretChanged(request(context.Background(), secret, obotNS, "my-secret"), nil))
	s2 := getServer(t, storage, "srv")
	assert.Equal(t, hash1, s2.Annotations[rotationAnnotation])
}

func TestSecretChanged_HashChangesWhenReferencedKeyChanges(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	v1Secret := newSecret("my-secret", map[string]string{"tok": "value-v1"})
	require.NoError(t, h.SecretChanged(request(context.Background(), v1Secret, obotNS, "my-secret"), nil))
	hash1 := getServer(t, storage, "srv").Annotations[rotationAnnotation]

	v2Secret := newSecret("my-secret", map[string]string{"tok": "value-v2"})
	require.NoError(t, h.SecretChanged(request(context.Background(), v2Secret, obotNS, "my-secret"), nil))
	hash2 := getServer(t, storage, "srv").Annotations[rotationAnnotation]

	assert.NotEqual(t, hash1, hash2)
}

func TestSecretChanged_UnreferencedKeyChangeDoesNotBumpAnnotation(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	// First call establishes the annotation hash.
	s1 := newSecret("my-secret", map[string]string{"tok": "stable", "noise": "v1"})
	require.NoError(t, h.SecretChanged(request(context.Background(), s1, obotNS, "my-secret"), nil))
	hash1 := getServer(t, storage, "srv").Annotations[rotationAnnotation]

	// Only "noise" (not referenced) changes — hash must be identical.
	s2 := newSecret("my-secret", map[string]string{"tok": "stable", "noise": "v2"})
	require.NoError(t, h.SecretChanged(request(context.Background(), s2, obotNS, "my-secret"), nil))
	hash2 := getServer(t, storage, "srv").Annotations[rotationAnnotation]

	assert.Equal(t, hash1, hash2, "unreferenced key change must not bump annotation")
}

func TestSecretChanged_DeleteSetsEmptyHash(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	// First establish a non-empty annotation.
	live := newSecret("my-secret", map[string]string{"tok": "v1"})
	require.NoError(t, h.SecretChanged(request(context.Background(), live, obotNS, "my-secret"), nil))
	require.NotEmpty(t, getServer(t, storage, "srv").Annotations[rotationAnnotation])

	// Delete event: req.Object is nil, namespace/name come from the request fields.
	require.NoError(t, h.SecretChanged(request(context.Background(), nil, obotNS, "my-secret"), nil))
	s := getServer(t, storage, "srv")
	assert.Empty(t, s.Annotations[rotationAnnotation], "deleted secret should set empty hash")
}

func TestSecretChanged_DeleteFansOutEvenWhenNeverAnnotated(t *testing.T) {
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	// Precondition: annotation is absent before the delete event.
	require.Empty(t, getServer(t, storage, "srv").Annotations[rotationAnnotation])

	// Delete event with no prior annotation on the server.
	require.NoError(t, h.SecretChanged(request(context.Background(), nil, obotNS, "my-secret"), nil))

	// The Annotations map must have been explicitly written (Update was called),
	// proving the reconcile fan-out happened.
	s := getServer(t, storage, "srv")
	require.NotNil(t, s.Annotations, "Update must have been called to initialise Annotations")
	_, wasSet := s.Annotations[rotationAnnotation]
	assert.True(t, wasSet, "rotationAnnotation must be explicitly set even when its value is empty")
}

func TestSecretChanged_FansOutToMultipleServers(t *testing.T) {
	srv1 := newServer("srv1", "shared-secret", "key")
	srv2 := newServer("srv2", "shared-secret", "key")
	// srv3 references a different secret — must not be touched.
	srv3 := newServer("srv3", "other-secret", "key")

	storage := newFakeClient(t, srv1, srv2, srv3)
	h := New(storage, obotNS, nil)

	secret := newSecret("shared-secret", map[string]string{"key": "val"})
	require.NoError(t, h.SecretChanged(request(context.Background(), secret, obotNS, "shared-secret"), nil))

	assert.NotEmpty(t, getServer(t, storage, "srv1").Annotations[rotationAnnotation])
	assert.NotEmpty(t, getServer(t, storage, "srv2").Annotations[rotationAnnotation])
	assert.Empty(t, getServer(t, storage, "srv3").Annotations[rotationAnnotation])
}

func TestSecretChanged_ObjectNilFallsBackToRequestNameNamespace(t *testing.T) {
	// req.Object is nil (e.g. tombstone on delete); req.Namespace/Name carry identity.
	server := newServer("srv", "my-secret", "tok")
	storage := newFakeClient(t, server)
	h := New(storage, obotNS, nil)

	// No Object — handler must still identify the secret by req.Namespace/Name.
	req := router.Request{Ctx: context.Background(), Object: nil, Namespace: obotNS, Name: "my-secret"}
	require.NoError(t, h.SecretChanged(req, nil))

	// Annotation set to empty hash (nil secret → HashReferencedKeys returns "").
	s := getServer(t, storage, "srv")
	assert.Empty(t, s.Annotations[rotationAnnotation])
}
