package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api/router"
	"github.com/obot-platform/obot/pkg/api/static"
	"github.com/obot-platform/obot/pkg/controller"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/rs/cors"
)

var log = logger.Package()

func Run(ctx context.Context, c services.Config) error {
	servicesCtx, servicesCancel := context.WithCancel(context.Background())
	defer servicesCancel()
	svcs, err := services.New(servicesCtx, c)
	if err != nil {
		return err
	}

	go func() {
		ctrl, err := controller.New(svcs)
		if err != nil {
			log.Fatalf("Failed to start controller: %v", err)
		}
		if err = ctrl.PreStart(ctx); err != nil {
			log.Fatalf("Failed to start controller: %v", err)
		}
		if err = ctrl.Start(ctx); err != nil {
			log.Fatalf("Failed to start controller: %v", err)
		}
	}()

	handler, err := router.Router(svcs)
	if err != nil {
		return err
	}

	if c.StaticDir != "" {
		handler, err = static.Wrap(handler, c.StaticDir)
		if err != nil {
			return err
		}
	}

	if c.DevMode && c.AllowedOrigin == "" {
		c.AllowedOrigin = "*"
	}

	address := fmt.Sprintf("0.0.0.0:%d", c.HTTPListenPort)
	log.Infof("Starting server on %s", address)
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
		Handler: allowEverything.Handler(handler),
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
		log.Infof("Shutting down OTel SDK")
		err := svcs.Otel.Shutdown(ctx)
		if err != nil {
			log.Errorf("Failed to shutdown OTel SDK: %v", err)
		}

		log.Infof("Shutting down server")
		if err := s.Shutdown(ctx); err != nil {
			log.Errorf("Failed to gracefully shutdown server: %v", err)
		}

		// Ensure that the audit logs are persisted.
		svcs.AuditLogger.Close()

		log.Infof("Shutting down MCP servers")
		// Shutdown all MCP servers
		svcs.MCPLoader.Close()
	})

	if err = s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-shutdown

	return nil
}
