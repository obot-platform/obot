package client

import (
	"context"
	"errors"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (c *Client) RunState(ctx context.Context, namespace, name string) (*types.RunState, error) {
	r := new(types.RunState)
	if err := c.db.WithContext(ctx).Where("name = ?", name).Where("namespace = ?", namespace).First(r).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return r, err
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "runstates",
	}, name)
}

func (c *Client) CreateRunState(ctx context.Context, runState *types.RunState) error {
	if err := c.db.WithContext(ctx).Create(runState).Error; !errors.Is(err, gorm.ErrCheckConstraintViolated) {
		return err
	}
	return apierrors.NewAlreadyExists(schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "runstates",
	}, runState.Name)
}

func (c *Client) UpdateRunState(ctx context.Context, runState *types.RunState) error {
	if err := c.db.WithContext(ctx).Save(runState).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return apierrors.NewNotFound(schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "runstates",
	}, runState.Name)
}

func (c *Client) DeleteRunState(ctx context.Context, namespace, name string) error {
	if err := c.db.WithContext(ctx).Delete(&types.RunState{Name: name, Namespace: namespace}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}
