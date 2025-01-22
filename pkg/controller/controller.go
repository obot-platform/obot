package controller

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/data"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	"github.com/obot-platform/obot/pkg/services"

	// Enable logrus logging in nah
	_ "github.com/obot-platform/nah/pkg/logrus"
)

type Controller struct {
	router         *router.Router
	services       *services.Services
	toolRefHandler *toolreference.Handler
}

func New(services *services.Services) (*Controller, error) {
	c := &Controller{
		router:   services.Router,
		services: services,
	}

	err := c.setupRoutes()
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Controller) PreStart(ctx context.Context) error {
	if err := data.Data(ctx, c.services.StorageClient, c.services.AgentsDir); err != nil {
		return fmt.Errorf("failed to apply data: %w", err)
	}
	return nil
}

func (c *Controller) PostStart(ctx context.Context) error {
	go c.toolRefHandler.PollRegistries(ctx, c.services.Router.Backend())
	return c.toolRefHandler.EnsureOpenAIEnvCredentialAndDefaults(ctx, c.services.Router.Backend())
}

func (c *Controller) Start(ctx context.Context) error {
	if err := c.router.Start(ctx); err != nil {
		return fmt.Errorf("failed to start router: %w", err)
	}
	return nil
}
