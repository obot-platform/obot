package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/otto8-ai/otto8/apiclient"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/spf13/cobra"
	yamlv3 "gopkg.in/yaml.v3"
)

type Create struct {
	Quiet bool `usage:"Only print ID after successful creation." short:"q"`
	root  *Otto8
}

func (l *Create) Customize(cmd *cobra.Command) {
	cmd.Use = "create [flags] FILE"
	cmd.Args = cobra.ExactArgs(1)
}

func parseManifests(data []byte) (result []types.WorkflowManifest, _ error) {
	var (
		dec = yamlv3.NewDecoder(bytes.NewReader(data))
	)
	for {
		parsed := map[string]any{}
		if err := dec.Decode(&parsed); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		jsonData, err := json.Marshal(parsed)
		if err != nil {
			return nil, err
		}

		var manifest types.WorkflowManifest
		if err := json.Unmarshal(jsonData, &manifest); err != nil {
			return nil, err
		}
		result = append(result, manifest)
	}

	return
}

func (l *Create) loadFromFile(ctx context.Context, file string) error {
	var (
		data []byte
		err  error
	)

	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		resp, err := http.Get(file)
		if err != nil {
			return err
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
	} else {
		data, err = os.ReadFile(file)
		if err != nil {
			return err
		}
	}

	manifests, err := parseManifests(data)
	if err != nil {
		return err
	}

	for _, newManifest := range manifests {
		if len(newManifest.Steps) > 0 || newManifest.Output != "" {
			if newManifest.RefName != "" {
				workflows, err := l.root.Client.ListWorkflows(ctx, apiclient.ListWorkflowsOptions{
					RefName: newManifest.RefName,
				})
				if err != nil {
					return err
				}
				if len(workflows.Items) > 0 {
					_, err = l.root.Client.UpdateWorkflow(ctx, workflows.Items[0].ID, newManifest)
					if err != nil {
						return err
					}
					if l.Quiet {
						fmt.Println(workflows.Items[0].ID)
					} else {
						fmt.Printf("Workflow updated: %s\n", workflows.Items[0].ID)
					}
					return nil
				}
			}

			workflow, err := l.root.Client.CreateWorkflow(ctx, newManifest)
			if err != nil {
				return err
			}

			if l.Quiet {
				fmt.Println(workflow.ID)
			} else {
				fmt.Printf("Workflow created: %s\n", workflow.ID)
			}
		} else {
			if newManifest.RefName != "" {
				agents, err := l.root.Client.ListAgents(ctx, apiclient.ListAgentsOptions{
					RefName: newManifest.RefName,
				})
				if err != nil {
					return err
				}
				if len(agents.Items) > 0 {
					_, err = l.root.Client.UpdateAgent(ctx, agents.Items[0].ID, newManifest.AgentManifest)
					if err != nil {
						return err
					}
					if l.Quiet {
						fmt.Println(agents.Items[0].ID)
					} else {
						fmt.Printf("Agent update: %s\n", agents.Items[0].ID)
					}
					return nil
				}
			}

			agent, err := l.root.Client.CreateAgent(ctx, newManifest.AgentManifest)
			if err != nil {
				return err
			}

			if l.Quiet {
				fmt.Println(agent.ID)
			} else {
				fmt.Printf("Agent created: %s\n", agent.ID)
			}
		}
	}

	return nil
}

func (l *Create) Run(cmd *cobra.Command, args []string) error {
	return l.loadFromFile(cmd.Context(), args[0])
}
