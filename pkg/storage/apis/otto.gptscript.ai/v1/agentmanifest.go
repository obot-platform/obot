package v1

import (
	"maps"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gptscript-ai/go-gptscript"
)

const (
	JavascriptHeader = "!javascript\n"
	PythonHeader     = "!python\n"
)

type AgentManifest struct {
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Temperature      *float32          `json:"temperature"`
	Cache            *bool             `json:"cache"`
	Slug             string            `json:"slug"`
	Prompt           Body              `json:"prompt"`
	Agents           []string          `json:"agents"`
	Workflows        []string          `json:"workflows,omitempty"`
	Tools            []string          `json:"tools"`
	Params           map[string]string `json:"params,omitempty"`
	CodeDependencies string            `json:"codeDependencies"`
}

type Body string

func (a Body) IsInline() bool {
	return strings.HasPrefix(string(a), "!")
}

func (a Body) Instructions() string {
	if a.IsJSON() {
		return "#!node\n" + strings.TrimPrefix(string(a), JavascriptHeader)
	}
	if a.IsPython() {
		return "#!python\n" + strings.TrimPrefix(string(a), PythonHeader)
	}
	return string(a)
}

func (a Body) Metadata(codeDeps string) map[string]string {
	if codeDeps == "" {
		return nil
	}
	if a.IsJSON() {
		return map[string]string{"package.json": codeDeps}
	} else if a.IsPython() {
		return map[string]string{"requirements.txt": codeDeps}
	}
	return nil
}

func (a Body) IsJSON() bool {
	return strings.HasPrefix(string(a), JavascriptHeader)
}

func (a Body) IsPython() bool {
	return strings.HasPrefix(string(a), PythonHeader)
}

func (m AgentManifest) GetParams() *openapi3.Schema {
	var args []string
	for _, k := range slices.Sorted(maps.Keys(m.Params)) {
		args = append(args, k)
		args = append(args, m.Params[k])
	}

	return gptscript.ObjectSchema(args...)
}
