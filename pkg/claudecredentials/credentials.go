package claudecredentials

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	als "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	ext "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	"github.com/obot-platform/obot/pkg/clauderunner"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/substrate"
	pb "github.com/obot-platform/obot/pkg/substrate/credproviderpb"
	"github.com/obot-platform/obot/pkg/system"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	egressIdentity = "spiffe://cluster.local/ns/ate-system/sa/atenet-egress"
)

var (
	// errKeyIssueNeeded reports that a session's key must be issued or rotated, which
	// fetchSecret does only when FetchSecret holds the provisioning lock.
	errKeyIssueNeeded = errors.New("the Claude session key must be issued")
)

// CredentialProvider is reachable only by the egress gateway, never by actors.
type CredentialProvider struct {
	pb.UnimplementedCredentialProviderServer
	Storage kclient.Reader
	Gateway *client.Client
	// ObotURL is the Obot URL actors reach Obot at.
	ObotURL string
}

func (p *CredentialProvider) FetchSecret(ctx context.Context, input *pb.FetchSecretRequest) (*pb.FetchSecretResponse, error) {
	if err := verifyEgressPeer(ctx); err != nil {
		return nil, err
	}

	// Most fetches return a session's current key or a stored secret, which needs no lock.
	// Only issuing or rotating a key takes the provisioning lock, so that no key is issued
	// for a session while it is revoked. The egress gateway fetches on every request, so
	// taking the lock each time would serialize them all.
	result, err := p.fetchSecret(ctx, input, false)
	if errors.Is(err, errKeyIssueNeeded) {
		err = p.Gateway.WithClaudeProvisionLock(ctx, func() error {
			var err error
			result, err = p.fetchSecret(ctx, input, true)
			return err
		})
	}
	if err != nil {
		if status.Code(err) == codes.PermissionDenied {
			return nil, err
		}
		slog.ErrorContext(ctx, "Claude egress credential unavailable", "error", err)
		return nil, status.Error(codes.Unavailable, "Claude egress credential unavailable")
	}
	return result, nil
}

// fetchSecret checks the session and its access on every fetch. It issues or rotates a
// session key only when issue is true; otherwise it returns errKeyIssueNeeded.
func (p *CredentialProvider) fetchSecret(ctx context.Context, input *pb.FetchSecretRequest, issue bool) (*pb.FetchSecretResponse, error) {
	name, ok := strings.CutPrefix(input.GetUri(), "ate-secret://obot/claude/")
	name, secretID, hasSecret := strings.Cut(name, "/")
	llm := hasSecret && secretID == "llm"
	if !ok || (hasSecret && !llm && (!strings.HasPrefix(secretID, "ces-") || strings.ContainsAny(secretID, "/%?#"))) || !strings.HasPrefix(name, "acr-") || strings.ContainsAny(name, "/%?#") {
		return nil, status.Error(codes.PermissionDenied, "Invalid credential reference")
	}

	var inst v1.AgentInstance
	if err := p.Storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, &inst); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, status.Error(codes.PermissionDenied, "Claude session not found")
		}
		return nil, err
	}
	ref := substrate.Ref(&inst)
	expected := "spiffe://substrate-actor.local/actor/" + ref.Atespace + "/" + ref.Name
	if input.GetActorSpiffeId() != expected || inst.Spec.ClaudeOrder == "" || inst.DeletionTimestamp != nil || inst.Spec.ClaudeDone || inst.Spec.Suspended {
		return nil, status.Error(codes.PermissionDenied, "Claude session is not active for this actor")
	}

	if llm {
		// The session's runtime records whether its environment enabled the LLM proxy.
		var runtime gt.ClaudeRuntime
		if err := json.Unmarshal([]byte(inst.Spec.ClaudeRuntime), &runtime); err != nil || !runtime.LLMProxy {
			return nil, status.Error(codes.PermissionDenied, "The LLM proxy is not enabled for this Claude session")
		}
	}

	environment, err := clauderunner.AuthorizedEnvironment(ctx, p.Gateway, inst.Spec.ClaudeEnvironment, inst.Spec.ClaudeAccount, inst.Spec.UserID)
	if clauderunner.AccessDenied(err) {
		return nil, status.Error(codes.PermissionDenied, "Claude environment access has been removed")
	}
	if err != nil {
		return nil, err
	}
	userID, err := strconv.ParseUint(inst.Spec.UserID, 10, 64)
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(5 * time.Minute)
	if llm {
		return sessionKey(issue, func() (gt.Credential, bool, error) {
			return p.Gateway.CurrentClaudeLLMCredential(ctx, uint(userID), inst.Name, expires)
		}, func() (gt.Credential, error) {
			return p.Gateway.EnsureClaudeLLMCredential(ctx, uint(userID), inst.Name, expires)
		})
	}
	if hasSecret {
		for _, secret := range environment.EgressSecrets {
			if secret.ID == secretID && clauderunner.EgressHostAllowed(environment.Runtime, secret.Hostname) {
				return &pb.FetchSecretResponse{OpaqueBytes: []byte(secret.Value)}, nil
			}
		}
		return nil, status.Error(codes.PermissionDenied, "Egress secret access has been removed")
	}
	if len(environment.MCPServerIDs) == 0 {
		return nil, status.Error(codes.PermissionDenied, "No MCP access granted")
	}

	return sessionKey(issue, func() (gt.Credential, bool, error) {
		return p.Gateway.CurrentClaudeCredential(ctx, uint(userID), inst.Name, environment.MCPServerIDs, expires)
	}, func() (gt.Credential, error) {
		return p.Gateway.EnsureClaudeCredential(ctx, uint(userID), inst.Name, environment.MCPServerIDs, expires)
	})
}

// sessionKey returns a session key: the current one, or, when issue is true, one ensure
// reuses or issues.
func sessionKey(issue bool, current func() (gt.Credential, bool, error), ensure func() (gt.Credential, error)) (*pb.FetchSecretResponse, error) {
	var (
		credential gt.Credential
		err        error
	)
	if issue {
		credential, err = ensure()
	} else {
		var ok bool
		credential, ok, err = current()
		if err == nil && !ok {
			return nil, errKeyIssueNeeded
		}
	}
	if errors.Is(err, client.ErrClaudeCredentialRevoked) {
		return nil, status.Error(codes.PermissionDenied, "Claude session credential has been revoked")
	}
	if err != nil {
		return nil, err
	}
	return &pb.FetchSecretResponse{OpaqueBytes: []byte(credential.Secrets["key"])}, nil
}

func verifyEgressPeer(ctx context.Context) error {
	p, ok := peer.FromContext(ctx)
	if ok {
		info, ok := p.AuthInfo.(credentials.TLSInfo)
		if ok && len(info.State.VerifiedChains) > 0 && len(info.State.PeerCertificates) > 0 {
			cert := info.State.PeerCertificates[0]
			now := time.Now()
			if now.Before(cert.NotAfter) && !now.Before(cert.NotBefore) && len(cert.URIs) == 1 && cert.URIs[0].String() == egressIdentity {
				return nil
			}
		}
	}
	return status.Error(codes.PermissionDenied, "Only the Substrate egress gateway may fetch credentials")
}

// StartCredentialProvider reloads projected certificates on every handshake.
func StartCredentialProvider(ctx context.Context, address, bundleFile, caFile string, provider *CredentialProvider) (func(), error) {
	tlsConfig, err := providerTLSConfig(bundleFile, caFile)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	server := grpc.NewServer(
		grpc.WaitForHandlers(true),
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		// Rotate connections for fresh certificates without terminating active audit streams.
		grpc.KeepaliveParams(keepalive.ServerParameters{MaxConnectionAge: 5 * time.Minute}),
	)
	pb.RegisterCredentialProviderServer(server, provider)
	ext.RegisterExternalProcessorServer(server, &AuditProcessor{Provider: provider})
	als.RegisterAccessLogServiceServer(server, &AccessLogger{Provider: provider})
	closeServer := sync.OnceFunc(server.Stop)
	stop := context.AfterFunc(ctx, closeServer)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			slog.ErrorContext(ctx, "Claude credential provider stopped", "error", err)
		}
	}()
	return func() { stop(); closeServer() }, nil
}

func providerTLSConfig(bundleFile, caFile string) (*tls.Config, error) {
	load := func(*tls.ClientHelloInfo) (*tls.Config, error) {
		bundle, err := os.ReadFile(bundleFile)
		if err != nil {
			return nil, err
		}
		cert, err := tls.X509KeyPair(bundle, bundle)
		if err != nil {
			return nil, err
		}
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("empty Substrate pod identity trust bundle")
		}
		return &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: []string{"h2"}, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}, nil
	}
	if _, err := load(nil); err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, GetConfigForClient: load}, nil
}
