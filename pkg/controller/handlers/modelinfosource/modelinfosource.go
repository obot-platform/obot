// Package modelinfosource syncs ModelInfo records from an external source.
package modelinfosource

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nanobot/pkg/safehttp"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const (
	syncInterval             = time.Hour
	modelInfoOwnerSubContext = "model-info"
)

type Handler struct {
	httpClient       *http.Client
	defaultSourceURL string
}

func New(defaultSourceURL string, urlValidation mcp.RemoteMCPURLValidationConfig) *Handler {
	return &Handler{
		// Reuse the deployment's remote-URL egress policy so the model info fetch
		// honors the same loopback/private-IP/link-local rules as other configured-URL fetches.
		httpClient: safehttp.NewClientWithTimeout(
			!urlValidation.AllowLocalhostMCP,
			!urlValidation.AllowPrivateIPMCP,
			!urlValidation.AllowLinkLocalMCP,
			time.Minute,
		),
		defaultSourceURL: defaultSourceURL,
	}
}

// Sync refreshes ModelInfo records when forced or stale.
func (h *Handler) Sync(req router.Request, resp router.Response) error {
	source := req.Object.(*v1.ModelInfoSource)

	forceSync := source.Annotations[v1.ModelInfoSourceSyncAnnotation] == "true"
	if !forceSync && !source.Status.LastSyncTime.IsZero() {
		if elapsed := time.Since(source.Status.LastSyncTime.Time); elapsed < syncInterval {
			resp.RetryAfter(syncInterval - elapsed)
			return nil
		}
	}

	infos, err := h.fetchModelInfos(req.Ctx, source)
	if err == nil {
		err = apply.New(req.Client).WithOwnerSubContext(modelInfoOwnerSubContext).WithPruneTypes(&v1.ModelInfo{}).Apply(req.Ctx, source, infos...)
	}
	if err != nil {
		log.Errorf("failed to sync model info source %s: %v", source.Name, err)
		source.Status.SyncError = err.Error()
	} else {
		source.Status.SyncError = ""
		source.Status.ModelCount = len(infos)
	}
	source.Status.LastSyncTime = metav1.Now()
	resp.RetryAfter(syncInterval)

	if err := req.Client.Status().Update(req.Ctx, source); err != nil {
		return err
	}
	if forceSync {
		delete(source.Annotations, v1.ModelInfoSourceSyncAnnotation)
		return req.Client.Update(req.Ctx, source)
	}
	return nil
}

// SetUpDefaultModelInfoSource reconciles the default ModelInfoSource from config.
func (h *Handler) SetUpDefaultModelInfoSource(ctx context.Context, c kclient.Client) error {
	source := &v1.ModelInfoSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultModelInfoSource,
			Namespace: system.DefaultNamespace,
		},
	}
	if h.defaultSourceURL == "" {
		if err := c.Delete(ctx, source); apierrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to delete disabled model info source: %w", err)
		}
		log.Infof("Deleted default model info source (disabled by empty URL)")

		return nil
	}

	var existing v1.ModelInfoSource
	if err := c.Get(ctx, router.Key(system.DefaultNamespace, system.DefaultModelInfoSource), &existing); apierrors.IsNotFound(err) {
		source.Spec.Manifest.URL = h.defaultSourceURL
		if err := c.Create(ctx, source); err != nil {
			return fmt.Errorf("failed to create default model info source: %w", err)
		}
		log.Infof("Created default model info source: %s (%s)", system.DefaultModelInfoSource, h.defaultSourceURL)
		return nil
	} else if err != nil {
		return err
	}

	if existing.Spec.Manifest.URL == h.defaultSourceURL {
		return nil
	}

	existing.Spec.Manifest.URL = h.defaultSourceURL
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	existing.Annotations[v1.ModelInfoSourceSyncAnnotation] = "true"
	if err := c.Update(ctx, &existing); err != nil {
		return fmt.Errorf("failed to update default model info source URL: %w", err)
	}
	log.Infof("Updated default model info source URL: %s (%s)", system.DefaultModelInfoSource, h.defaultSourceURL)
	return nil
}
