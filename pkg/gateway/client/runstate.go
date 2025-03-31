package client

import (
	"bytes"
	"context"
	"errors"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var (
	encryptionSuffix = []byte(":OBOTENCRYPTED")
	gr               = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "runstates",
	}
)

func (c *Client) RunState(ctx context.Context, namespace, name string) (*types.RunState, error) {
	r := new(types.RunState)
	if err := c.db.WithContext(ctx).Where("name = ?", name).Where("namespace = ?", namespace).First(r).Error; err == nil {
		if transformer, ok := c.encryptionConfig.Transformers[gr]; ok {
			if err := decryptRunState(ctx, r, transformer); err != nil {
				return nil, err
			}
		}
		return r, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return nil, apierrors.NewNotFound(gr, name)
}

func (c *Client) CreateRunState(ctx context.Context, runState *types.RunState) error {
	// Copy the run state to avoid modifying the original
	r := runState
	if transformer, ok := c.encryptionConfig.Transformers[gr]; ok {
		if err := encryptRunState(ctx, r, transformer); err != nil {
			return err
		}
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get the run state. If it exists, return an already exists error, otherwise create it.
		// We do this because trying to catch the gorm.ErrDuplicateKey doesn't work.
		if err := tx.Where("name = ?", runState.Name).Where("namespace = ?", runState.Namespace).First(r).Error; err == nil {
			return apierrors.NewAlreadyExists(gr, runState.Name)
		}
		return tx.Create(r).Error
	})
}

func (c *Client) UpdateRunState(ctx context.Context, runState *types.RunState) error {
	// Copy the run state to avoid modifying the original
	r := runState
	if transformer, ok := c.encryptionConfig.Transformers[gr]; ok {
		if err := encryptRunState(ctx, r, transformer); err != nil {
			return err
		}
	}
	if err := c.db.WithContext(ctx).Save(r).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return apierrors.NewNotFound(gr, runState.Name)
}

func (c *Client) DeleteRunState(ctx context.Context, namespace, name string) error {
	if err := c.db.WithContext(ctx).Delete(&types.RunState{Name: name, Namespace: namespace}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func encryptRunState(ctx context.Context, runState *types.RunState, transformer value.Transformer) error {
	var (
		b    []byte
		err  error
		errs []error
	)
	if transformer != nil {
		if b, err = transformer.TransformToStorage(ctx, runState.Output, nil); err != nil {
			errs = append(errs, err)
		} else {
			runState.Output = append(b, encryptionSuffix...)
		}
		if b, err = transformer.TransformToStorage(ctx, runState.CallFrame, nil); err != nil {
			errs = append(errs, err)
		} else {
			runState.CallFrame = append(b, encryptionSuffix...)
		}
		if b, err = transformer.TransformToStorage(ctx, runState.ChatState, nil); err != nil {
			errs = append(errs, err)
		} else {
			runState.ChatState = append(b, encryptionSuffix...)
		}
	}
	return errors.Join(errs...)
}

func decryptRunState(ctx context.Context, runState *types.RunState, transformer value.Transformer) error {
	var (
		b    []byte
		ok   bool
		errs []error
		err  error
	)
	if transformer != nil {
		if b, ok = bytes.CutSuffix(runState.Output, encryptionSuffix); ok {
			runState.Output, _, err = transformer.TransformFromStorage(ctx, b, nil)
			if err != nil {
				errs = append(errs, err)
			}
		}
		if b, ok = bytes.CutSuffix(runState.CallFrame, encryptionSuffix); ok {
			runState.CallFrame, _, err = transformer.TransformFromStorage(ctx, b, nil)
			if err != nil {
				errs = append(errs, err)
			}
		}
		if b, ok = bytes.CutSuffix(runState.ChatState, encryptionSuffix); ok {
			runState.ChatState, _, err = transformer.TransformFromStorage(ctx, b, nil)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}
