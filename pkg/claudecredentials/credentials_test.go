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

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"gorm.io/gorm"
)

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
