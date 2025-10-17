package mcp

import (
	"context"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sSettingsGetter provides access to K8s settings with caching
type K8sSettingsGetter interface {
	GetK8sSettings(ctx context.Context) (*v1.K8sSettingsSpec, error)
}

type k8sSettingsGetter struct {
	client    storage.Client
	mu        sync.RWMutex
	cached    *v1.K8sSettingsSpec
	lastFetch time.Time
	cacheTTL  time.Duration
}

func newK8sSettingsGetter(client storage.Client) K8sSettingsGetter {
	return &k8sSettingsGetter{
		client:   client,
		cacheTTL: 30 * time.Second, // Cache for 30 seconds
	}
}

func (g *k8sSettingsGetter) GetK8sSettings(ctx context.Context) (*v1.K8sSettingsSpec, error) {
	g.mu.RLock()
	if g.cached != nil && time.Since(g.lastFetch) < g.cacheTTL {
		defer g.mu.RUnlock()
		return g.cached, nil
	}
	g.mu.RUnlock()

	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check after acquiring write lock
	if g.cached != nil && time.Since(g.lastFetch) < g.cacheTTL {
		return g.cached, nil
	}

	var settings v1.K8sSettings
	if err := g.client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &settings); err != nil {
		return nil, err
	}

	g.cached = &settings.Spec
	g.lastFetch = time.Now()

	return g.cached, nil
}
