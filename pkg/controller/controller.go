package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/data"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	"github.com/obot-platform/obot/pkg/services"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	// Enable logrus logging in nah
	_ "github.com/obot-platform/nah/pkg/logrus"
)

type Controller struct {
	router             *router.Router
	services           *services.Services
	toolRefHandler     *toolreference.Handler
	catalogRefreshChan chan struct{}
}

func New(services *services.Services) (*Controller, error) {
	c := &Controller{
		router:             services.Router,
		services:           services,
		catalogRefreshChan: services.MCPCatalogRefreshChan,
	}

	err := c.setupRoutes()
	if err != nil {
		return nil, err
	}

	services.Router.PosStart(c.PostStart)

	return c, nil
}

func (c *Controller) PreStart(ctx context.Context) error {
	if err := data.Data(ctx, c.services.StorageClient, c.services.AgentsDir); err != nil {
		return fmt.Errorf("failed to apply data: %w", err)
	}
	return nil
}

func (c *Controller) PostStart(ctx context.Context, client kclient.Client) {
	go c.toolRefHandler.PollRegistriesAndCatalogs(ctx, client, c.catalogRefreshChan)
	var err error
	for range 3 {
		err = c.toolRefHandler.EnsureOpenAIEnvCredentialAndDefaults(ctx, client)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond) // wait a bit before retrying
	}
	if err != nil {
		panic(fmt.Errorf("failed to ensure openai env credential and defaults: %w", err))
	}
}

func (c *Controller) Start(ctx context.Context) error {
	if err := c.router.Start(ctx); err != nil {
		return fmt.Errorf("failed to start router: %w", err)
	}
	return nil
}
