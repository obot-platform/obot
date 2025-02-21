package projects

import (
	"context"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func ThreadIDs(ctx context.Context, c kclient.Client, thread *v1.Thread) ([]string, error) {
	var parentIDs []string

	if thread.Spec.ParentThreadName != "" {
		var parent v1.Thread
		if err := c.Get(ctx, router.Key(thread.Namespace, thread.Spec.ParentThreadName), &parent); err != nil {
			return nil, err
		}

		var err error
		parentIDs, err = ThreadIDs(ctx, c, &parent)
		if err != nil {
			return nil, err
		}
	}

	return append([]string{thread.Name}, parentIDs...), nil
}

func Recurse(ctx context.Context, c kclient.Client, thread *v1.Thread, check func(*v1.Thread) (bool, error)) (*v1.Thread, error) {
	if thread == nil || thread.Spec.ParentThreadName == "" {
		return thread, nil
	}

	parentThread := new(v1.Thread)
	if err := c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Spec.ParentThreadName}, parentThread); err != nil {
		return nil, err
	}

	if ok, err := check(parentThread); ok || err != nil {
		return parentThread, err
	}

	return Recurse(ctx, c, parentThread, check)
}
