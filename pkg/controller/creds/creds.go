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

			credNames, err := determineCredentialNames(prg, prg.ToolSet[ref.ToolID], cred, map[string]struct{}{system.ModelProviderCredential: {}})
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

func determineCredentialNames(prg *gptscript.Program, tool gptscript.Tool, credToolName string, noAuth map[string]struct{}) ([]string, error) {
	var subTool string
	parsedToolName, alias, args, err := gtypes.ParseCredentialArgs(credToolName, "")
	if err != nil {
		parsedToolName, subTool = gtypes.SplitToolRef(credToolName)
		parsedToolName, alias, args, err = gtypes.ParseCredentialArgs(parsedToolName, "")
		if err != nil {
			return nil, err
		}
	}

	for _, n := range strings.Split(tool.MetaData["noUserAuth"], ",") {
		if n != "" {
			noAuth[n] = struct{}{}
		}
	}

	if alias != "" {
		if _, ok := noAuth[alias]; !ok {
			return []string{alias}, nil
		}
		return nil, nil
	}

	if args != nil {
		if _, ok := noAuth[alias]; !ok {
			return []string{credToolName}, nil
		}
		return nil, nil
	}

	// This is a tool and not the credential format. Parse the tool from the program to determine the alias
	toolNames := make([]string, 0, len(tool.Credentials))
	if subTool == "" {
		credToolName = parsedToolName
	}
	for _, cred := range tool.Credentials {
		if cred == credToolName {
			if len(tool.ToolMapping[cred]) == 0 {
				return nil, fmt.Errorf("cannot find credential name for tool %q", credToolName)
			}

			for _, ref := range tool.ToolMapping[cred] {
				for _, c := range prg.ToolSet[ref.ToolID].ExportCredentials {
					names, err := determineCredentialNames(prg, prg.ToolSet[ref.ToolID], c, noAuth)
					if err != nil {
						return nil, err
					}

					for _, n := range names {
						if _, ok := noAuth[n]; !ok && !slices.Contains(toolNames, n) {
							toolNames = append(toolNames, n)
						}
					}
				}
			}
		}
	}

	return toolNames, nil
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
