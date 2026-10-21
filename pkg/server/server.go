package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/obot-platform/obot/pkg/api/router"
	"github.com/obot-platform/obot/pkg/claudecredentials"
	"github.com/obot-platform/obot/pkg/clauderunner"
	"github.com/obot-platform/obot/pkg/controller"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/rs/cors"
)

func Run(ctx context.Context, c services.Config) error {
	servicesCtx, servicesCancel := context.WithCancel(context.Background())
	defer servicesCancel()
	svcs, err := services.New(servicesCtx, c)
	if err != nil {
		return err
	}

	// Router construction migrates the Nanobot session schema. Finish it before
	// controller initialization writes to the same SQLite database to avoid schema locks.
	router, err := router.NewRouter(ctx, svcs)
	if err != nil {
		return err
	}
	// The router is also closed during server shutdown,
	// but we should make sure it's closed on any errors.
	defer router.Close()

	ctrl, err := controller.New(svcs)
	if err != nil {
		return fmt.Errorf("failed to create controller: %v", err)
	}
	if err = ctrl.PreStart(ctx); err != nil {
		return fmt.Errorf("failed to start controller: %v", err)
	}
	if err = ctrl.Start(ctx); err != nil {
		return fmt.Errorf("failed to start controller: %v", err)
	}

	if c.AllowedOrigin == "" {
		c.AllowedOrigin = "*"
	}

	address := fmt.Sprintf("0.0.0.0:%d", c.HTTPListenPort)
	slog.Info("Starting server", "address", address)
	allowEverything := cors.New(cors.Options{
		AllowedOrigins: []string{c.AllowedOrigin},
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowedHeaders: []string{"*"},
		ExposedHeaders: []string{"*"},
	})

	s := &http.Server{
		Addr:    address,
		Handler: allowEverything.Handler(router),
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()

	go svcs.GatewayClient.RunEgressAuditCleanup(ctx, c.EgressAuditLogRetentionDays)
	stopOrchestrator := func() {}
	stopCredentials := func() {}
	if svcs.Substrate != nil {
		stopCredentials, err = claudecredentials.StartCredentialProvider(ctx, ":8444", "/run/substrate/credential.pem", "/run/substrate/pod-identity-ca.pem", &claudecredentials.CredentialProvider{Storage: svcs.StorageClient, Gateway: svcs.GatewayClient, ObotURL: svcs.Substrate.Config.ObotURL})
		if err != nil {
			return fmt.Errorf("start Claude egress credentials: %w", err)
		}
		defer func() {
			stopCredentials()
			if err := svcs.GatewayClient.FlushEgressAuditLogs(); err != nil {
				slog.Error("Failed to flush egress audit logs", "error", err)
			}
		}()

		stopOrchestrator = clauderunner.ManageOrchestrators(ctx, svcs.GatewayClient, fmt.Sprintf("http://127.0.0.1:%d", c.HTTPListenPort))
		defer stopOrchestrator()
	}

	shutdown := make(chan struct{})
	context.AfterFunc(ctx, func() {
		defer close(shutdown)
		// Shutdown services after controller and web server are done.
		defer servicesCancel()

		stopOrchestrator()

		// Wait for controller to release the lease.
		<-svcs.Router.Stopped()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		slog.Info("Shutting down OTel SDK")
		err := svcs.Otel.Shutdown(ctx)
		if err != nil {
			slog.Error("Failed to shutdown OTel SDK", "error", err)
		}

		slog.Info("Shutting down server")
		if err := s.Shutdown(ctx); err != nil {
			slog.Error("Failed to gracefully shutdown server", "error", err)
		}

		slog.Info("Shutting down HTTP router")
		if err := router.Close(); err != nil {
			slog.Error("Failed to gracefully shutdown HTTP router", "error", err)
		}

		// Stop and drain egress handlers before closing the audit persister/database.
		stopCredentials()

		// Ensure that the audit logs are persisted.
		svcs.AuditLogger.Close()

		slog.Info("Shutting down MCP servers")
		// Shutdown all MCP servers
		svcs.MCPSessionManager.Close()

		if err := svcs.GatewayClient.Close(); err != nil {
			slog.Error("Failed to close gateway client", "error", err)
		}
		svcs.ProviderDispatcher.Close()
	})

	if err = s.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-shutdown

	return nil
}
