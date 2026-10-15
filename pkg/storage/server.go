package storage

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/obot-platform/kinm/pkg/db"
	mserver "github.com/obot-platform/kinm/pkg/server"
	"github.com/obot-platform/obot/pkg/storage/openapi/generated"
	"github.com/obot-platform/obot/pkg/storage/registry/apigroups/agent"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/version"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	k8sversion "k8s.io/apimachinery/pkg/version"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// schemaLockID is the Postgres advisory lock that serializes the storage tables' schema
	// setup across Obot replicas.
	schemaLockID int64 = 0x6f626f74746273 // "obottbs"
)

type Client kclient.WithWatch

func Start(ctx context.Context, config sservices.Config) (Client, *rest.Config, *db.Factory, *sservices.Services, error) {
	services, err := sservices.New(config)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	c, cfg, err := startMinkServer(ctx, config, services)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return c, cfg, services.DB, services, nil
}

// buildAPIGroups builds the storage API groups. Building one creates or migrates its
// table: kinm runs CREATE TABLE IF NOT EXISTS, adds missing columns and recreates the
// fields index for every type. Postgres does not make those safe to run concurrently, so
// replicas that start together would fail with a duplicate key error on the catalog (for
// example pg_type_typname_nsp_index) whenever a table is new. On Postgres, a session
// advisory lock makes replicas take turns.
func buildAPIGroups(ctx context.Context, services *sservices.Services) ([]*genericapiserver.APIGroupInfo, error) {
	if services.DB.DB.Name() == "postgres" {
		conn, err := services.DB.SQLDB.Conn(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get a connection to lock the storage schema: %w", err)
		}
		defer conn.Close()
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", schemaLockID); err != nil {
			return nil, fmt.Errorf("failed to lock the storage schema: %w", err)
		}
		defer func() {
			// The connection goes back to the pool, and the lock would stay with it. If it
			// can't be released, drop the connection instead, which releases it.
			if _, unlockErr := conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", schemaLockID); unlockErr != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}()
	}

	return mserver.BuildAPIGroups(services, agent.APIGroup, agent.LeasesAPIGroup)
}

func startMinkServer(ctx context.Context, config sservices.Config, services *sservices.Services) (Client, *rest.Config, error) {
	apiGroups, err := buildAPIGroups(ctx, services)
	if err != nil {
		return nil, nil, err
	}

	var l net.Listener
	if config.StorageListenPort == 0 {
		l, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, nil, err
		}
	}

	minkConfig := &mserver.Config{
		Name:              "Storage Server",
		Version:           version.Get().String(),
		Authenticator:     services.Authn,
		Authorization:     services.Authz,
		HTTPSListenPort:   config.StorageListenPort,
		Listener:          l,
		OpenAPIConfig:     generated.GetOpenAPIDefinitions,
		Scheme:            scheme.Scheme,
		APIGroups:         apiGroups,
		ReadinessCheckers: []healthz.HealthChecker{services.DB},
	}

	//if cfg.AuditLogPolicyFile != "" && cfg.AuditLogPath != "" {
	//	minkConfig.AuditConfig = mserver.NewAuditOptions(cfg.AuditLogPolicyFile, cfg.AuditLogPath)
	//}

	minkConfig.Middleware = []func(http.Handler) http.Handler{
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				if req.URL.Path == "/version" {
					_ = json.NewEncoder(rw).Encode(k8sversion.Info{
						GitVersion: version.Get().String(),
						GitCommit:  version.Get().Commit,
					})
				} else {
					next.ServeHTTP(rw, req)
				}
			})
		},
	}

	minkServer, err := mserver.New(minkConfig)
	if err != nil {
		return nil, nil, err
	}

	_ = minkServer.Handler(ctx)

	cfg := minkServer.Loopback
	// Create an HTTP client so we can add the otel transport for metrics
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, nil, err
	}

	httpClient.Transport = otelhttp.NewTransport(httpClient.Transport)

	c, err := kclient.NewWithWatch(cfg, kclient.Options{
		Scheme:     scheme.Scheme,
		HTTPClient: httpClient,
	})
	return c, cfg, err
}
