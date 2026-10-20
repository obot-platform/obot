package claudecredentials

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/clauderunner"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/substrate"
	pb "github.com/obot-platform/obot/pkg/substrate/credproviderpb"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClaudeEgressCredentials(t *testing.T) {
	gateway, db := newCredentialTestGateway(t)

	ctx := t.Context()
	env := &gt.ClaudeEnvironment{ID: "environment", EnvironmentID: "ccpool_test", OrganizationID: "org_test", ClaudeTagEnabled: true}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	require.NoError(t, gateway.EnsureClaudeTagUser(ctx, env))
	env.Profiles = []gt.ClaudeEnvironmentProfile{{Name: "service", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: env.ClaudeTagUserID}}, MCPServerIDs: []string{"vmcp_one"}}}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	inst := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{UserID: env.ClaudeTagUserID, ClaudeEnvironment: env.ID, ClaudeAccount: clauderunner.ClaudeTagAccount, ClaudeOrder: "order", ClaudeSession: "session"}}
	inst.Name, inst.Namespace = "acr-test", system.DefaultNamespace
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(inst).Build()
	provider := &CredentialProvider{Gateway: gateway, Storage: storage}
	ref := substrate.Ref(inst)
	input := &pb.FetchSecretRequest{Uri: "ate-secret://obot/claude/" + inst.Name, ActorSpiffeId: "spiffe://substrate-actor.local/actor/" + ref.Atespace + "/" + ref.Name}

	_, err := provider.FetchSecret(ctx, input)
	require.Equal(t, codes.PermissionDenied, status.Code(err), "an actor cannot use the provider directly")
	uri, err := url.Parse("spiffe://cluster.local/ns/ate-system/sa/atenet-egress")
	require.NoError(t, err)
	cert := &x509.Certificate{URIs: []*url.URL{uri}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}}})
	started := time.Now()
	first, err := provider.fetchSecret(ctx, input)
	finished := time.Now()
	require.NoError(t, err)
	key, err := gateway.ValidateAPIKey(ctx, string(first.OpaqueBytes))
	require.NoError(t, err)
	require.Equal(t, []string{"vmcp_one"}, key.MCPServerIDs)
	require.True(t, key.ClaudeEnvironment)
	require.WithinRange(t, *key.ExpiresAt, started.Add(5*time.Minute), finished.Add(5*time.Minute))
	second, err := provider.fetchSecret(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.OpaqueBytes, second.OpaqueBytes)

	require.NoError(t, db.Model(&gt.APIKey{}).Where("id = ?", key.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	refreshed, err := provider.fetchSecret(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, first.OpaqueBytes, refreshed.OpaqueBytes)
	require.ErrorIs(t, db.First(&gt.APIKey{}, key.ID).Error, gorm.ErrRecordNotFound)

	wrong := &pb.FetchSecretRequest{Uri: input.Uri, ActorSpiffeId: "spiffe://substrate-actor.local/actor/other/" + inst.Name}
	_, err = provider.fetchSecret(ctx, wrong)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	wrong = &pb.FetchSecretRequest{Uri: input.Uri + "/../another", ActorSpiffeId: input.ActorSpiffeId}
	_, err = provider.fetchSecret(ctx, wrong)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	env.Runtime.EgressHosts = []string{"*.example.com"}
	env.EgressSecrets = []gt.ClaudeEgressSecret{{ID: "ces-env", Hostname: "api.example.com", Header: "authorization", Value: "env-secret"}}
	env.Profiles[0].EgressSecrets = []gt.ClaudeEgressSecret{{ID: "ces-profile", Hostname: "api.example.com", Header: "authorization", Value: "profile-secret"}}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	secretInput := &pb.FetchSecretRequest{Uri: input.Uri + "/ces-profile", ActorSpiffeId: input.ActorSpiffeId}
	injected, err := provider.fetchSecret(ctx, secretInput)
	require.NoError(t, err)
	require.Equal(t, "profile-secret", string(injected.OpaqueBytes))

	// A mapped human loses both third-party secrets and MCP credentials immediately.
	require.NoError(t, db.Create(&gt.User{ID: 123, Username: "mapped", HashedUsername: "mapped", Email: "mapped@example.com"}).Error)
	require.NoError(t, gateway.SaveClaudeOrganizationAccounts(ctx, env.OrganizationID, []gt.ClaudeOrganizationAccount{{Account: "user_mapped", UserID: "123"}}))
	env.Profiles[0].Subjects = append(env.Profiles[0].Subjects, types.Subject{Type: types.SubjectTypeUser, ID: "123"})
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	inst.Spec.UserID, inst.Spec.ClaudeAccount = "123", "user_mapped"
	require.NoError(t, storage.Update(ctx, inst))
	mappedRef := substrate.Ref(inst)
	mapped := &pb.FetchSecretRequest{Uri: secretInput.Uri, ActorSpiffeId: "spiffe://substrate-actor.local/actor/" + mappedRef.Atespace + "/" + mappedRef.Name}
	_, err = provider.fetchSecret(ctx, mapped)
	require.NoError(t, err)
	require.NoError(t, db.Model(&gt.User{}).Where("id = ?", 123).Update("disabled_at", time.Now()).Error)
	_, err = provider.fetchSecret(ctx, mapped)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	mapped.Uri = input.Uri
	_, err = provider.fetchSecret(ctx, mapped)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	inst.Spec.UserID, inst.Spec.ClaudeAccount = env.ClaudeTagUserID, clauderunner.ClaudeTagAccount
	require.NoError(t, storage.Update(ctx, inst))

	processor := &AuditProcessor{Provider: provider}
	entry := &gt.EgressAuditLog{Scheme: "https", Host: "api.example.com:443", EnvironmentID: env.ID, ClaudeAccount: inst.Spec.ClaudeAccount, UserID: inst.Spec.UserID}
	mutation, err := processor.secretPlaceholders(ctx, entry)
	require.NoError(t, err)
	require.Len(t, mutation.SetHeaders, 1)
	require.Equal(t, "authorization", mutation.SetHeaders[0].Header.Key)
	require.Equal(t, "obot-egress-placeholder", string(mutation.SetHeaders[0].Header.RawValue))
	require.Equal(t, core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD, mutation.SetHeaders[0].AppendAction)

	entry.Host = "other.example.com"
	mutation, err = processor.secretPlaceholders(ctx, entry)
	require.NoError(t, err)
	require.Empty(t, mutation.SetHeaders)
	entry.Host = "api.example.com:8443"
	mutation, err = processor.secretPlaceholders(ctx, entry)
	require.NoError(t, err)
	require.Nil(t, mutation)
	entry.Host, entry.Scheme = "api.example.com", "http"
	mutation, err = processor.secretPlaceholders(ctx, entry)
	require.NoError(t, err)
	require.Nil(t, mutation)

	secretInput.Uri = input.Uri + "/ces-env"
	_, err = provider.fetchSecret(ctx, secretInput)
	require.Equal(t, codes.PermissionDenied, status.Code(err), "overridden secrets must not be retrievable")
	secretInput.Uri = input.Uri + "/ces-profile"
	env.Runtime.EgressHosts = nil
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	_, err = provider.fetchSecret(ctx, secretInput)
	require.Equal(t, codes.PermissionDenied, status.Code(err), "removing egress permission also removes secret access")
	env.Runtime.AllowAllEgress = true
	env.Profiles[0].EgressSecrets[0].Value = "rotated"
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	injected, err = provider.fetchSecret(ctx, secretInput)
	require.NoError(t, err)
	require.Equal(t, "rotated", string(injected.OpaqueBytes))
	secretInput.ActorSpiffeId = "spiffe://substrate-actor.local/actor/other/" + inst.Name
	_, err = provider.fetchSecret(ctx, secretInput)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	for _, field := range []string{"suspended", "done"} {
		inst.Spec.Suspended = field == "suspended"
		inst.Spec.ClaudeDone = field == "done"
		require.NoError(t, storage.Update(ctx, inst))
		_, err = provider.fetchSecret(ctx, input)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	}
	inst.Spec.Suspended, inst.Spec.ClaudeDone = false, false
	require.NoError(t, storage.Update(ctx, inst))
	env.ClaudeTagEnabled = false
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	_, err = provider.fetchSecret(ctx, input)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	env.ClaudeTagEnabled = true
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	key, err = gateway.ValidateAPIKey(ctx, string(refreshed.OpaqueBytes))
	require.NoError(t, err)
	require.NoError(t, gateway.RevokeAPIKeyByID(ctx, key.ID))
	_, err = provider.fetchSecret(ctx, input)
	require.Equal(t, codes.PermissionDenied, status.Code(err), "refresh must not resurrect a revoked key")

	cert.URIs[0], _ = url.Parse("spiffe://cluster.local/ns/ate-system/sa/other")
	_, err = provider.FetchSecret(ctx, input)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestEgressPeerRequiresVerifiedCurrentCertificate(t *testing.T) {
	uri, err := url.Parse(egressIdentity)
	require.NoError(t, err)
	cert := &x509.Certificate{URIs: []*url.URL{uri}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	info := credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}}
	require.Error(t, verifyEgressPeer(peer.NewContext(t.Context(), &peer.Peer{AuthInfo: info})))
	info.State.VerifiedChains = [][]*x509.Certificate{{cert}}
	require.NoError(t, verifyEgressPeer(peer.NewContext(t.Context(), &peer.Peer{AuthInfo: info})))
	cert.NotAfter = time.Now().Add(-time.Second)
	require.Error(t, verifyEgressPeer(peer.NewContext(t.Context(), &peer.Peer{AuthInfo: info})))
}

func newCredentialTestGateway(t *testing.T) (*gclient.Client, *gorm.DB) {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway, services.DB.DB
}

func TestProviderTLSHandshake(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	identity, err := url.Parse(egressIdentity)
	require.NoError(t, err)
	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:              []string{"localhost"},
		URIs:                  []*url.URL{identity},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	private, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	bundle := append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})...)
	file := filepath.Join(t.TempDir(), "bundle.pem")
	require.NoError(t, os.WriteFile(file, bundle, 0600))
	config, err := providerTLSConfig(file, file)
	require.NoError(t, err)
	clientCert, err := tls.X509KeyPair(bundle, bundle)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(certPEM))
	serverPipe, clientPipe := net.Pipe()
	defer serverPipe.Close()
	defer clientPipe.Close()
	require.NoError(t, serverPipe.SetDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, clientPipe.SetDeadline(time.Now().Add(5*time.Second)))
	server := tls.Server(serverPipe, config)
	client := tls.Client(clientPipe, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "localhost", Certificates: []tls.Certificate{clientCert}, NextProtos: []string{"h2"}})
	done := make(chan error, 1)
	go func() { done <- server.Handshake() }()
	require.NoError(t, client.Handshake())
	require.NoError(t, <-done)
	require.Equal(t, "h2", client.ConnectionState().NegotiatedProtocol)
	require.NoError(t, verifyEgressPeer(peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: server.ConnectionState()}})))
	require.NoError(t, os.WriteFile(file, []byte("invalid rotated bundle"), 0600))
	_, err = config.GetConfigForClient(nil)
	require.Error(t, err, "a bad rotation must fail closed, not reuse stale certificates")
}
