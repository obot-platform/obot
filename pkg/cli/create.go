package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/acorn-io/acorn/pkg/cli/edit"
	"github.com/acorn-io/acorn/pkg/cli/templates"
	"github.com/acorn-io/namegenerator"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	yamlv3 "gopkg.in/yaml.v3"
)

type Create struct {
	Quiet bool `usage:"Only print ID after successful creation." short:"q"`
	root  *Acorn
}

func (l *Create) Customize(cmd *cobra.Command) {
	cmd.Use = "create [flags] FILE"
}

type manifest struct {
	Type string
	Data []byte
}

func parseManifests(data []byte) (result []manifest, _ error) {
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

		var typeName string
		for k, v := range parsed {
			if strings.EqualFold(k, "type") {
				typeName, _ = v.(string)
				if typeName != "" {
					break
				}
			}
		}

		if typeName == "" {
			return nil, fmt.Errorf("missing type field in manifest")
		}

		jsonData, err := json.Marshal(parsed)
		if err != nil {
			return nil, err
		}

		result = append(result, manifest{
			Type: typeName,
			Data: jsonData,
		})
	}

	return
}

func readInput(file string) (io.ReadCloser, error) {
	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		resp, err := http.Get(file)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}

	return os.Open(file)
}

func (l *Create) create(ctx context.Context, input io.ReadCloser) error {
	defer input.Close()

	data, err := io.ReadAll(input)
	if err != nil {
		return err
	}

	manifests, err := parseManifests(data)
	if err != nil {
		return fmt.Errorf("failed to parse manifest: %v", err)
	}

	for _, manifest := range manifests {
		id, err := l.root.Client.Create(ctx, manifest.Type, manifest.Data)
		if err != nil {
			return err
		}
		if l.Quiet {
			fmt.Println(id)
		} else {
			fmt.Printf("Created %s: %s\n", manifest.Type, id)
		}
	}

	return nil
}

func newName() string {
	caser := cases.Title(language.English)
	return caser.String(strings.ReplaceAll(namegenerator.NewNameGenerator(time.Now().UnixNano()).Generate(), "-", " "))
}

func (l *Create) fromTemplate() (string, error) {
	sel, err := pterm.DefaultInteractiveSelect.
		WithDefaultText("Select a type to create").
		WithOptions([]string{
			"Agent",
			"Workflow",
			"Webhook",
			"Email Receiver",
		}).Show()
	if err != nil {
		return "", err
	}

	template, err := templates.FS.ReadFile(strings.ToLower(strings.ReplaceAll(sel, " ", "") + ".yaml"))
	if err != nil {
		return "", err
	}

	template = bytes.ReplaceAll(template, []byte("%NAME%"), []byte(newName()))

	err = edit.Edit(template, ".yaml", func(data []byte) error {
		template = data
		return nil
	})
	if errors.Is(err, edit.ErrEditAborted) {
		return string(template), nil
	}

	return string(template), err
}

func (l *Create) Run(cmd *cobra.Command, args []string) error {
	var input io.ReadCloser

	if len(args) == 0 {
		template, err := l.fromTemplate()
		if err != nil {
			return err
		}
		input = io.NopCloser(strings.NewReader(template))
	} else {
		var err error
		input, err = readInput(args[0])
		if err != nil {
			return err
		}
	}

	return l.create(cmd.Context(), input)
}
