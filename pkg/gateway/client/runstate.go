package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var (
	runStatesGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "runstates",
	}
)

func (c *Client) RunState(ctx context.Context, namespace, name string) (*types.RunState, error) {
	r := new(types.RunState)
	if err := c.db.WithContext(ctx).Where("name = ?", name).Where("namespace = ?", namespace).First(r).Error; err == nil {
		if err := c.decryptRunState(ctx, r); err != nil {
			return nil, err
		}
		return r, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return nil, apierrors.NewNotFound(runStatesGroupResource, name)
}

func (c *Client) CreateRunState(ctx context.Context, runState *types.RunState) error {
	// Copy the run state to avoid modifying the original
	r := *runState

	if err := c.encryptRunState(ctx, &r); err != nil {
		return err
	}

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get the run state. If it exists, return an already exists error, otherwise create it.
		// We do this because trying to catch the gorm.ErrDuplicateKey doesn't work.
		if err := tx.Where("name = ?", runState.Name).Where("namespace = ?", runState.Namespace).First(r).Error; err == nil {
			return apierrors.NewAlreadyExists(runStatesGroupResource, runState.Name)
		}
		return tx.Create(&r).Error
	}); err != nil {
		return err
	}

	runState.CreatedAt = r.CreatedAt
	runState.UpdatedAt = r.UpdatedAt
	return nil
}

func (c *Client) UpdateRunState(ctx context.Context, runState *types.RunState) error {
	// Copy the run state to avoid modifying the original
	r := *runState

	if err := c.encryptRunState(ctx, &r); err != nil {
		return err
	}

	// Explicitly update the done, so that it is always set to the value that is sent by the caller.
	if err := c.db.WithContext(ctx).Updates(&r).Update("done", runState.Done).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return apierrors.NewNotFound(runStatesGroupResource, runState.Name)
	} else if err != nil {
		return err
	}

	runState.CreatedAt = r.CreatedAt
	runState.UpdatedAt = r.UpdatedAt
	return nil
}

func (c *Client) DeleteRunState(ctx context.Context, namespace, name string) error {
	if err := c.db.WithContext(ctx).Delete(&types.RunState{Name: name, Namespace: namespace}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func (c *Client) encryptRunState(ctx context.Context, runState *types.RunState) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[runStatesGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		err  error
		errs []error

		dataCtx = runStateDataCtx(runState.Namespace, runState.Name)
	)
	if runState.Output, err = transformer.TransformToStorage(ctx, runState.Output, dataCtx); err != nil {
		errs = append(errs, err)
	}
	if runState.CallFrame, err = transformer.TransformToStorage(ctx, runState.CallFrame, dataCtx); err != nil {
		errs = append(errs, err)
	}
	if runState.ChatState, err = transformer.TransformToStorage(ctx, runState.ChatState, dataCtx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c *Client) decryptRunState(ctx context.Context, runState *types.RunState) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[runStatesGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		err  error
		errs []error

		dataCtx = runStateDataCtx(runState.Namespace, runState.Name)
	)
	runState.Output, _, err = transformer.TransformFromStorage(ctx, runState.Output, dataCtx)
	if err != nil {
		errs = append(errs, err)
	}
	runState.CallFrame, _, err = transformer.TransformFromStorage(ctx, runState.CallFrame, dataCtx)
	if err != nil {
		errs = append(errs, err)
	}
	runState.ChatState, _, err = transformer.TransformFromStorage(ctx, runState.ChatState, dataCtx)
	if err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func runStateDataCtx(namespace, name string) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", runStatesGroupResource.String(), namespace, name))
}
