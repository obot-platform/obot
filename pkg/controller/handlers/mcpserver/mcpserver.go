package mcpserver

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	baseURL string
}

func New(baseURL string) *Handler {
	return &Handler{
		baseURL: baseURL,
	}
}

func (h *Handler) DetectDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.MCPServerCatalogEntryName == "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
		return err
	}

	var (
		drifted bool
		err     error
	)
	if entry.Spec.CommandManifest.Name != "" {
		drifted, err = configurationHasDrifted(server.Spec.Manifest, entry.Spec.CommandManifest)
	} else {
		drifted, err = configurationHasDrifted(server.Spec.Manifest, entry.Spec.URLManifest)
	}
	if err != nil {
		return err
	}

	if server.Status.NeedsUpdate != drifted {
		server.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

func configurationHasDrifted(serverManifest types.MCPServerManifest, entryManifest types.MCPServerCatalogEntryManifest) (bool, error) {
	// First, check on the URL or hostname.
	if entryManifest.FixedURL != "" && serverManifest.URL != entryManifest.FixedURL {
		return true, nil
	}

	if entryManifest.Hostname != "" {
		u, err := url.Parse(serverManifest.URL)
		if err != nil {
			// Shouldn't ever happen.
			return true, err
		}

		if u.Hostname() != entryManifest.Hostname {
			return true, nil
		}
	}

	// Check the rest of the fields to see if anything has changed.
	drifted := serverManifest.Command != entryManifest.Command ||
		!slices.Equal(serverManifest.Args, entryManifest.Args) ||
		!utils.SlicesEqualIgnoreOrder(serverManifest.Env, entryManifest.Env) ||
		!utils.SlicesEqualIgnoreOrder(serverManifest.Headers, entryManifest.Headers)

	return drifted, nil
}

func (h *Handler) MigrateProjectMCPServers(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	mcpID, ok := strings.CutPrefix(server.Spec.Manifest.URL, fmt.Sprintf("%s/mcp-connect/", h.baseURL))
	if !ok || server.Spec.ThreadName == "" {
		return nil
	}

	var projectMCPServers v1.ProjectMCPServerList
	if err := req.List(untriggered.UncachedList(&projectMCPServers), &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": server.Spec.ThreadName,
		}),
	}); err != nil {
		return err
	}

	var found bool
	for _, projectMCPServer := range projectMCPServers.Items {
		if projectMCPServer.Spec.Manifest.MCPID == mcpID {
			found = true
			break
		}
	}

	if found {
		return kclient.IgnoreNotFound(req.Delete(server))
	}

	if err := req.Client.Create(req.Ctx, &v1.ProjectMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ProjectMCPServerPrefix,
			Namespace:    req.Namespace,
			Finalizers:   []string{v1.ProjectMCPServerFinalizer},
		},
		Spec: v1.ProjectMCPServerSpec{
			Manifest: types.ProjectMCPServerManifest{
				MCPID: mcpID,
			},
			ThreadName: server.Spec.ThreadName,
			UserID:     server.Spec.UserID,
		},
	}); err != nil {
		return err
	}

	return kclient.IgnoreNotFound(req.Delete(server))
}
