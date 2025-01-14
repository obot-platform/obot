package creds

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	gtypes "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/obot/pkg/system"
)

func DetermineCredsAndCredNames(prg *gptscript.Program, tool gptscript.Tool, name string) ([]string, []string, error) {
	seen := make(map[string]struct{})
	// The available tool references from this tool are the tool itself and any tool this tool exports.
	toolRefs := make([]toolRef, 0, len(tool.Export)+len(tool.Tools)+1)
	toolRefs = append(toolRefs, toolRef{
		ToolReference: gptscript.ToolReference{
			Reference: name,
			ToolID:    prg.EntryToolID,
		},
		name: name,
	})
	toolRefs = append(toolRefs, toolRefsFromTools(tool, toolRefs[0], tool.Tools, seen)...)

	credentials := make([]string, 0, len(tool.Credentials)+len(tool.Export)+len(tool.Tools))
	credentialNames := make([]string, 0, len(tool.Credentials)+len(tool.Export)+len(tool.Tools))
	for len(toolRefs) > 0 {
		ref := toolRefs[0]
		toolRefs = toolRefs[1:]

		if _, ok := seen[ref.ToolID]; ok {
			continue
		}
		seen[ref.ToolID] = struct{}{}

		t := prg.ToolSet[ref.ToolID]

		// Add the tools that this tool exports if we haven't already seen them.
		toolRefs = append(toolRefs, toolRefsFromTools(t, ref, t.Export, seen)...)

		for _, cred := range append(t.Credentials, t.ExportCredentials...) {
			if parsedCred := fullToolPathName(ref, cred); parsedCred != "" && !slices.Contains(credentials, parsedCred) {
				credentials = append(credentials, parsedCred)
			}

			credNames, err := determineCredentialNames(prg, prg.ToolSet[ref.ToolID], cred)
			if err != nil {
				return credentials, credentialNames, err
			}

			for _, n := range credNames {
				if !slices.Contains(credentialNames, n) {
					credentialNames = append(credentialNames, n)
				}
			}
		}
	}

	return credentials, credentialNames, nil
}

func determineCredentialNames(prg *gptscript.Program, tool gptscript.Tool, toolName string) ([]string, error) {
	if toolName == system.ModelProviderCredential {
		return []string{system.ModelProviderCredential}, nil
	}

	var subTool string
	parsedToolName, alias, args, err := gtypes.ParseCredentialArgs(toolName, "")
	if err != nil {
		parsedToolName, subTool = gtypes.SplitToolRef(toolName)
		parsedToolName, alias, args, err = gtypes.ParseCredentialArgs(parsedToolName, "")
		if err != nil {
			return nil, err
		}
	}

	if alias != "" {
		return []string{alias}, nil
	}

	if args == nil {
		// This is a tool and not the credential format. Parse the tool from the program to determine the alias
		toolNames := make([]string, 0, len(tool.Credentials))
		if subTool == "" {
			toolName = parsedToolName
		}
		for _, cred := range tool.Credentials {
			if cred == toolName {
				if len(tool.ToolMapping[cred]) == 0 {
					return nil, fmt.Errorf("cannot find credential name for tool %q", toolName)
				}

				for _, ref := range tool.ToolMapping[cred] {
					for _, c := range prg.ToolSet[ref.ToolID].ExportCredentials {
						names, err := determineCredentialNames(prg, prg.ToolSet[ref.ToolID], c)
						if err != nil {
							return nil, err
						}

						toolNames = append(toolNames, names...)
					}
				}
			}
		}

		if len(toolNames) > 0 {
			return toolNames, nil
		}

		return nil, fmt.Errorf("tool %q not found in program", toolName)
	}

	return []string{toolName}, nil
}

type toolRef struct {
	gptscript.ToolReference
	name string
}

func toolRefsFromTools(parentTool gptscript.Tool, parentRef toolRef, tools []string, seen map[string]struct{}) []toolRef {
	var toolRefs []toolRef
	for _, e := range tools {
		name := e
		if _, ok := parentTool.LocalTools[strings.ToLower(e)]; ok {
			name, _ = gtypes.SplitToolRef(parentRef.name)
			name = fmt.Sprintf("%s from %s", e, name)
		}
		name = fullToolPathName(parentRef, name)
		if name == "" {
			continue
		}

		for _, r := range parentTool.ToolMapping[e] {
			if _, ok := seen[r.ToolID]; !ok {
				toolRefs = append(toolRefs, toolRef{
					ToolReference: r,
					name:          name,
				})
			}
		}
	}

	return toolRefs
}

func fullToolPathName(parentRef toolRef, name string) string {
	toolName, subTool := gtypes.SplitToolRef(name)

	// If this tool's path is relative to its parent.
	if strings.HasPrefix(toolName, ".") {
		parentToolName, _ := gtypes.SplitToolRef(parentRef.name)
		if rel, err := filepath.Rel(parentToolName, toolName); err == nil && strings.HasPrefix(toolName, parentToolName) {
			toolName = rel
		}
		refURL, err := url.Parse(parentToolName)
		if err != nil {
			return ""
		}

		if strings.HasSuffix(refURL.Path, ".gpt") {
			refURL.Path = path.Dir(refURL.Path)
		}

		refURL.Path = path.Join(refURL.Path, toolName)
		name = refURL.String()
		if refURL.Host == "" {
			// This is only a path, so url unescape it.
			// No need to check the error here, we would have errored when parsing.
			name, _ = url.PathUnescape(name)
		}

		if subTool != "" {
			name = fmt.Sprintf("%s from %s", subTool, name)
		}
	}

	return name
}
