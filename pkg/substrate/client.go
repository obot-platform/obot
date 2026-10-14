// Package substrate connects Obot's instance controller to Substrate v0.3.0.
package substrate

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Finalizer = "obot.obot.ai/substrate-agent"
)

type Config struct {
	WorkerNodeSelector map[string]string
	WorkerTolerations  []corev1.Toleration
	WorkerImage        string
	RunnerImage        string
	Namespace          string
	Address            string
	CAFile             string
	TokenFile          string
	SnapshotLocation   string
	ObotURL            string
}

type Client struct {
	Kube    kclient.Client
	Control pb.ControlClient
	Config  Config
}

type tokenFile string

func (f tokenFile) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	b, err := os.ReadFile(string(f))
	if err != nil {
		return nil, err
	}

	return map[string]string{"authorization": "Bearer " + strings.TrimSpace(string(b))}, nil
}

func (tokenFile) RequireTransportSecurity() bool { return true }

func New(ctx context.Context, config Config) (*Client, error) {
	if config.Address == "" {
		return nil, nil
	}
	if err := ValidateWorkerScheduling(config.WorkerNodeSelector, config.WorkerTolerations); err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(config.Address)
	if err != nil || host == "" {
		return nil, fmt.Errorf("substrate address must be host:port")
	}

	obot, err := url.Parse(config.ObotURL)
	if err != nil || obot.Host == "" || obot.Scheme != "http" || (obot.Path != "" && obot.Path != "/") || obot.User != nil || obot.RawQuery != "" || obot.Fragment != "" {
		return nil, fmt.Errorf("substrate Obot URL must be an internal http:// URL without a path for egress credential injection")
	}
	if net.ParseIP(obot.Hostname()) != nil || obot.Hostname() != strings.ToLower(obot.Hostname()) || strings.HasSuffix(obot.Hostname(), ".") {
		return nil, fmt.Errorf("substrate Obot URL must use a lowercase DNS hostname for egress policy matching")
	}
	if obot.Port() != "" {
		port, err := strconv.Atoi(obot.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("substrate Obot URL must be an internal http:// URL without a path for egress credential injection port")
		}
	}
	if config.CAFile == "" || config.TokenFile == "" {
		return nil, fmt.Errorf("substrate CA and token files are required")
	}
	snapshot, err := url.Parse(config.SnapshotLocation)
	if err != nil || (snapshot.Scheme != "s3" && snapshot.Scheme != "gs") || snapshot.Host == "" {
		return nil, fmt.Errorf("substrate snapshot location must be an s3:// or gs:// bucket URL")
	}
	config.ObotURL = strings.TrimRight(config.ObotURL, "/")

	// Reload the projected trust bundle on each handshake, including CA rotations.
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, InsecureSkipVerify: true} // Verification is performed below against the current bundle.
	tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
		pem, err := os.ReadFile(config.CAFile)
		if err != nil {
			return err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return fmt.Errorf("substrate CA bundle contains no certificates")
		}
		intermediates := x509.NewCertPool()
		for _, cert := range cs.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		_, err = cs.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: intermediates})
		return err
	}
	conn, err := grpc.NewClient(config.Address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithPerRPCCredentials(tokenFile(config.TokenFile)))
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	return &Client{Control: pb.NewControlClient(conn), Config: config}, nil
}

func Ref(instance *v1.AgentInstance) *pb.ObjectRef {
	owner := sha256.Sum256([]byte(instance.Spec.UserID))
	return &pb.ObjectRef{Atespace: fmt.Sprintf("obot-%x", owner[:12]), Name: instance.Name}
}
