package server

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/otto8-ai/otto8/logger"
	"github.com/otto8-ai/otto8/pkg/api/router"
	"github.com/otto8-ai/otto8/pkg/controller"
	"github.com/otto8-ai/otto8/pkg/services"
	"github.com/rs/cors"
)

var log = logger.Package()

func Run(ctx context.Context, c services.Config) error {
	if os.Getenv("OTTO8_PPROF_ENABLED") == "true" {
		go func() {
			log.Infof("Starting pprof on :6060")
			log.Errorf(http.ListenAndServe("localhost:6060", nil).Error())
		}()
	}

	svcs, err := services.New(ctx, c)
	if err != nil {
		return err
	}

	go func() {
		c, err := controller.New(svcs)
		if err != nil {
			log.Fatalf("Failed to start controller: %v", err)
		}
		if err := c.Start(ctx); err != nil {
			log.Fatalf("Failed to start controller: %v", err)
		}
		if err := c.PostStart(ctx); err != nil {
			log.Fatalf("Failed to post start controller: %v", err)
		}
	}()

	handler, err := router.Router(svcs)
	if err != nil {
		return err
	}

	context.AfterFunc(ctx, func() {
		log.Fatalf("Interrupted, exiting")
	})

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
	return http.ListenAndServe(address, allowEverything.Handler(handler))
}
