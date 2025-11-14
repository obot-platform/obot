package systemmcpserversources

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Sync(req router.Request, resp router.Response) error {
	sources := req.Object.(*v1.SystemMCPServerSources)

	// Mark as syncing
	sources.Status.IsSyncing = true
	if err := req.Client.Status().Update(req.Ctx, sources); err != nil {
		return err
	}

	defer func() {
		// Fetch again to ensure we have the latest version
		var updatedSources v1.SystemMCPServerSources
		if err := req.Client.Get(req.Ctx, router.Key(req.Namespace, sources.Name), &updatedSources); err != nil {
			log.Errorf("failed to get sources: %v", err)
			return
		}

		updatedSources.Status.IsSyncing = false
		updatedSources.Status.LastSyncTime = metav1.Now()
		if err := req.Client.Status().Update(req.Ctx, &updatedSources); err != nil {
			log.Errorf("failed to update sources status: %v", err)
		}
	}()

	var allObjects []kclient.Object
	syncErrors := make(map[string]string)

	// Process each source URL
	for _, sourceURL := range sources.Spec.SourceURLs {
		objects, err := h.readSystemMCPServers(sources.Name, sourceURL)
		if err != nil {
			syncErrors[sourceURL] = err.Error()
			continue
		}
		allObjects = append(allObjects, objects...)
	}

	// Update sync errors
	sources.Status.SyncErrors = syncErrors
	if err := req.Client.Status().Update(req.Ctx, sources); err != nil {
		return err
	}

	// Apply all system servers using prune
	app := apply.New(req.Client).WithOwnerSubContext(fmt.Sprintf("system-mcp-sources-%s", sources.Name))

	// Don't prune if there are sync errors
	if len(syncErrors) > 0 {
		app = app.WithNoPrune()
	} else {
		app = app.WithPruneTypes(&v1.SystemMCPServer{})
	}

	// Refresh every hour
	resp.RetryAfter(time.Hour)

	return app.Apply(req.Ctx, sources, allObjects...)
}

func (h *Handler) readSystemMCPServers(sourcesName, sourceURL string) ([]kclient.Object, error) {
	var systemServerSpecs []SystemMCPServerSpec

	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		if mcpcatalog.IsGitHubURL(sourceURL) {
			var err error
			systemServerSpecs, err = readGitHubSystemServers(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read GitHub system servers %s: %w", sourceURL, err)
			}
		} else {
			// Treat as raw file URL
			resp, err := http.Get(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch system servers from %s: %w", sourceURL, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("failed to fetch system servers from %s: status code %d", sourceURL, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body from %s: %w", sourceURL, err)
			}

			if err := yaml.UnmarshalStrict(body, &systemServerSpecs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal system servers from %s: %w", sourceURL, err)
			}
		}
	} else if info, err := os.Stat(sourceURL); err == nil {
		if info.IsDir() {
			systemServerSpecs, err = readSystemServerDirectory(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read system server directory %s: %w", sourceURL, err)
			}
		} else {
			systemServerSpecs, err = readSystemServerFile(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read system server file %s: %w", sourceURL, err)
			}
		}
	} else {
		return nil, fmt.Errorf("invalid source URL %s: %w", sourceURL, err)
	}

	// Convert specs to SystemMCPServer objects
	var objects []kclient.Object
	for _, spec := range systemServerSpecs {
		// Validate system type
		if spec.SystemServerSettings.SystemType != types.SystemTypeHook {
			return nil, fmt.Errorf("invalid system type: %s", spec.SystemServerSettings.SystemType)
		}

		systemServer := &v1.SystemMCPServer{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.SystemMCPServerPrefix,
				Labels: map[string]string{
					"obot.ai/synced-from": sourcesName,
				},
				Finalizers: []string{v1.SystemMCPServerFinalizer},
			},
			Spec: v1.SystemMCPServerSpec{
				Manifest:             spec.Manifest,
				SystemServerSettings: spec.SystemServerSettings,
				SourceURL:            sourceURL,
				Editable:             false, // Git-synced servers cannot have their manifest edited
			},
		}

		// Note: Git-synced system servers can still be configured via the /configure endpoint.
		// The Editable:false flag only prevents editing the manifest, not setting credentials.
		// Admins can configure git-synced servers by providing required env vars/headers through credentials.

		objects = append(objects, systemServer)
	}

	return objects, nil
}

// Helper types for git source parsing
type SystemMCPServerSpec struct {
	Manifest             types.MCPServerManifest    `json:"manifest"`
	SystemServerSettings types.SystemServerSettings `json:"systemServerSettings"`
}

func readGitHubSystemServers(sourceURL string) ([]SystemMCPServerSpec, error) {
	// Clone the repository
	tempDir, err := mcpcatalog.CloneGitHubRepo(sourceURL)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	return readSystemServerDirectory(tempDir)
}

func readSystemServerDirectory(dir string) ([]SystemMCPServerSpec, error) {
	var specs []SystemMCPServerSpec

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".json")) {
			return nil
		}

		fileSpecs, err := readSystemServerFile(path)
		if err != nil {
			log.Warnf("Skipping invalid system server file %s: %v", path, err)
			return nil // Continue walking, don't fail entire sync
		}

		specs = append(specs, fileSpecs...)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return specs, nil
}

func readSystemServerFile(path string) ([]SystemMCPServerSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var specs []SystemMCPServerSpec
	if err := yaml.UnmarshalStrict(data, &specs); err != nil {
		// Try as single spec
		var spec SystemMCPServerSpec
		if err := yaml.UnmarshalStrict(data, &spec); err != nil {
			return nil, err
		}
		specs = []SystemMCPServerSpec{spec}
	}

	return specs, nil
}
