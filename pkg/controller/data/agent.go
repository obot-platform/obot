package data

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adrg/xdg"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

//go:embed agent.yaml
var agentBytes []byte

func addAgents(ctx context.Context, k kclient.Client, agentDir string) error {
	if err := addAutoAgents(ctx, k, agentDir); err != nil {
		return err
	}
	return addDefaultAgent(ctx, k, agentDir)
}

func addAutoAgents(ctx context.Context, k kclient.Client, agentDir string) error {
	var err error
	if agentDir == "" {
		agentDir, err = xdg.ConfigFile(path.Join("obot", "agents"))
		if err != nil {
			return fmt.Errorf("failed to get agent dir: %w", err)
		}
	}

	files, err := os.ReadDir(agentDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(agentDir, file.Name()))
		if err != nil {
			return fmt.Errorf("failed to read agent file %s: %w", file.Name(), err)
		}

		var manifest types.AgentManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("failed to unmarshal agent file %s: %w", file.Name(), err)
		}

		if manifest.Alias == "" {
			return fmt.Errorf("agent file %s is missing an alias", file.Name())
		}

		var agent v1.Agent
		if err := alias.Get(ctx, k, &agent, system.DefaultNamespace, manifest.Alias); apierrors.IsNotFound(err) {
			if err := k.Create(ctx, &v1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.AgentPrefix,
					Namespace:    system.DefaultNamespace,
				},
				Spec: v1.AgentSpec{
					Manifest: manifest,
				},
			}); err != nil {
				return fmt.Errorf("failed to create agent %s: %w", manifest.Alias, err)
			}
			continue
		} else if err != nil {
			return fmt.Errorf("failed to get agent %s: %w", manifest.Alias, err)
		}

		if !equality.Semantic.DeepEqual(agent.Spec.Manifest, manifest) {
			agent.Spec.Manifest = manifest
			if err := k.Update(ctx, &agent); err != nil {
				return fmt.Errorf("failed to update agent %s: %w", manifest.Alias, err)
			}
		}
	}

	return nil
}

func addDefaultAgent(ctx context.Context, k kclient.Client, agentDir string) error {
	var agent v1.Agent
	if err := yaml.Unmarshal(agentBytes, &agent); err != nil {
		return err
	}

	var existing v1.Agent
	if err := k.Get(ctx, kclient.ObjectKey{Namespace: agent.Namespace, Name: agent.Name}, &existing); apierrors.IsNotFound(err) {
		if agentDir != "" {
			// If the agent dir is set, it's assumed they don't want the default, so only add it if there are zero agents
			var agents v1.AgentList
			if err := k.List(ctx, &agents); err != nil {
				return fmt.Errorf("failed to list agents: %w", err)
			}
			if len(agents.Items) > 0 {
				return nil
			}
		}

		if err := k.Create(ctx, &agent); err != nil {
			return err
		}
		if err := k.Create(ctx, &v1.AgentAuthorization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      handlers.AgentAuthorizationName(agent.Name, "*"),
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.AgentAuthorizationSpec{
				AuthorizationManifest: types.AuthorizationManifest{
					UserID:  "*",
					AgentID: agent.Name,
				},
			},
		}); kclient.IgnoreAlreadyExists(err) != nil {
			return err
		}
		existing = agent
	} else if err != nil {
		return err
	}

	var modified bool
	modified, existing.Spec.Manifest.Tools = addTool(modified, &existing, existing.Spec.Manifest.Tools, agent.Spec.Manifest.Tools)
	modified, existing.Spec.Manifest.DefaultThreadTools = addTool(modified, &existing, existing.Spec.Manifest.DefaultThreadTools, agent.Spec.Manifest.DefaultThreadTools)
	modified, existing.Spec.Manifest.AvailableThreadTools = addTool(modified, &existing, existing.Spec.Manifest.AvailableThreadTools, agent.Spec.Manifest.AvailableThreadTools)

	// migrate from the old icon path
	if existing.Spec.Manifest.Icons != nil && existing.Spec.Manifest.Icons.Icon == "/images/obot-icon-blue.svg" {
		existing.Spec.Manifest.Icons = agent.Spec.Manifest.Icons
		modified = true
	}

	if modified {
		return k.Update(ctx, &existing)
	}

	return nil
}

func addTool(alreadyModified bool, agent *v1.Agent, existing, newTools []string) (modified bool, result []string) {
	if alreadyModified {
		modified = true
	}

	result = existing
	for _, tool := range newTools {
		if agent.Annotations[tool] == "added" {
			continue
		}

		modified = true
		if agent.Annotations == nil {
			agent.Annotations = make(map[string]string)
		}
		agent.Annotations[tool] = "added"
		if !slices.Contains(result, tool) {
			result = append(result, tool)
		}
	}
	return
}
