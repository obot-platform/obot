package events

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/cli/textio"
)

var log = logger.Package()

type Quiet struct {
	Client *apiclient.Client
	Ctx    context.Context
}

func (q *Quiet) Print(events <-chan types.Progress) error {
	var lastContent string
	for event := range events {
		if event.Error != "" {
			return fmt.Errorf("%s", event.Error)
		}
		if event.Prompt != nil {
			fmt.Printf("> %s (use @file.txt syntax to read value from file)\n", event.Prompt.Message)
			if err := handlePrompt(q.Ctx, q.Client, event.Prompt); err != nil {
				return err
			}
		} else if event.Content != "" {
			lastContent = event.Content
			fmt.Print(event.Content)
		}
	}
	if !strings.HasSuffix(lastContent, "\n") {
		fmt.Println()
	}
	return nil
}

type Verbose struct {
	Details bool
	Client  *apiclient.Client
	Ctx     context.Context
}

func (v *Verbose) Print(events <-chan types.Progress) error {
	var (
		out       = textio.NewSpinnerPrinter()
		lastType  string
		lastRunID string
	)
	out.Start()
	defer out.Stop()

outer:
	for {
		select {
		case <-time.After(100 * time.Millisecond):
			out.Tick()
		case event, ok := <-events:
			if !ok {
				break outer
			}

			if event.RunID != "" && event.RunID != lastRunID && v.Details {
				lastType = "run"
				out.EnsureNewline()
				out.Print(fmt.Sprintf("\n> Run ID: %s\n", event.RunID))
				lastRunID = event.RunID
			}

			if event.Step != nil && v.Details {
				lastType = "step"
				out.EnsureNewline()
				out.Print(event.Step.Display())
			} else if event.Input != "" {
				out.EnsureNewline()
				if lastType == "content" {
					out.Print("\n")
				}
				lastType = "input"
				out.Print(fmt.Sprintf("> %s\n", color.GreenString(event.Input)))
			} else if event.WaitingOnModel {
				lastType = "waiting"
				out.EnsureNewline()
				out.Print("> Waiting for model... \n")
			} else if event.Error != "" {
				lastType = "error"
				out.EnsureNewline()
				out.Stop()
				log.Errorf("%s", event.Error)
				out.Start()
			} else if event.ToolInput != nil {
				if lastType != "toolInput" {
					out.Print(fmt.Sprintf("> Generating tool input for (%s)...  ", event.ToolInput.Name))
				}
				lastType = "toolInput"
				out.Print(event.ToolInput.Input)
			} else if event.ToolCall != nil {
				out.EnsureNewline()
				out.Print(fmt.Sprintf("> Running tool (%s): %s\n", color.MagentaString(event.ToolCall.Name), color.MagentaString(event.ToolCall.Input)))
			} else if event.Prompt != nil {
				out.EnsureNewline()
				out.Print(fmt.Sprintf("> %s\n", color.CyanString(event.Prompt.Message+` (use @file.txt syntax to read value from file)`)))
				if err := handlePrompt(v.Ctx, v.Client, event.Prompt); err != nil {
					return err
				}
			} else if event.Content != "" {
				if lastType != "content" {
					out.EnsureNewline()
					out.Print("\n")
				}
				lastType = "content"
				out.Print(color.CyanString(event.Content))
			}
		}
	}

	out.EnsureNewline()
	return nil
}

func handlePrompt(ctx context.Context, c *apiclient.Client, prompt *types.Prompt) error {
	promptResponse := types.PromptResponse{
		ID:        prompt.ID,
		Responses: make(map[string]string),
	}

	for _, field := range prompt.Fields {
		v, err := textio.Ask(field.Name, "")
		if err != nil {
			return err
		}
		if strings.HasPrefix(v, "@") {
			data, err := os.ReadFile(v[1:])
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			} else if err == nil {
				v = string(data)
			}
		}
		promptResponse.Responses[field.Name] = v
	}

	if len(promptResponse.Responses) == 0 {
		return nil
	}

	return c.PromptResponse(ctx, promptResponse)
}
