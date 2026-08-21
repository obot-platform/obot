package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/obot-platform/obot/pkg/api/router"
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

	shutdown := make(chan struct{})
	context.AfterFunc(ctx, func() {
		defer close(shutdown)
		// Shutdown services after controller and web server are done.
		defer servicesCancel()

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

		// Ensure that the audit logs are persisted.
		svcs.AuditLogger.Close()

		slog.Info("Shutting down MCP servers")
		// Shutdown all MCP servers
		svcs.MCPSessionManager.Close()

		svcs.GatewayClient.Close()
		svcs.ProviderDispatcher.Close()
	})

	if err = s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-shutdown

	return nil
}
