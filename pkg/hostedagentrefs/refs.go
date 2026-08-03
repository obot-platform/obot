// Package hostedagentrefs resolves the portable references a hosted agent
// template uses to name MCP servers and skills.
//
// A template is written once and synced into many installations, so it cannot
// name things by the IDs those installations generate. It names them by
// something stable instead -- the source they came from, and their key within
// it -- and this resolves that to whatever the local ID happens to be.
//
// References are resolved where they are used, not rewritten into the stored
// template at sync time. A template naming a server that is not installed
// therefore syncs, appears, and is reported unavailable with the reason;
// installing the server later makes it work with no resync and no edit. Sync
// rewriting would instead fail the whole source over one reference and freeze
// the answer.
package hostedagentrefs

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Separator divides the source from the key within it. It matches what the MCP
// catalog already reserves and validates entry keys against, so a reference
// written here is the same string that catalog builds internally.
const Separator = "::"

// IsReference reports whether a value is a portable reference rather than an
// ID. Anything without the separator is passed through untouched, so templates
// and agents that name IDs directly keep working.
func IsReference(value string) bool {
	return strings.Contains(value, Separator)
}

// Split divides a reference into its source and key.
func Split(ref string) (source, key string, ok bool) {
	source, key, ok = strings.Cut(ref, Separator)
	source, key = strings.TrimSpace(source), strings.TrimSpace(key)
	return source, key, ok && source != "" && key != ""
}

// ResolveMCP turns "<source>::<entryKey>" into the local catalog entry ID.
//
// The source is a catalog's URL without its scheme, which is how the MCP
// catalog itself identifies a source, and the key is the entry key that
// catalog supplies. An entry key is unique only within its source, so both
// halves are required.
func ResolveMCP(ctx context.Context, client kclient.Client, namespace, ref string) (string, error) {
	if !IsReference(ref) {
		return ref, nil
	}
	source, key, ok := Split(ref)
	if !ok {
		return "", fmt.Errorf("malformed MCP server reference %q, expected <source>%s<entryKey>", ref, Separator)
	}

	var entries v1.MCPServerCatalogEntryList
	if err := client.List(ctx, &entries, &kclient.ListOptions{Namespace: namespace}); err != nil {
		return "", fmt.Errorf("list catalog entries: %w", err)
	}
	for _, entry := range entries.Items {
		if entry.Spec.Manifest.EntryKey != key {
			continue
		}
		if mcp.SourceIDForURL(entry.Spec.SourceURL) == source {
			return entry.Name, nil
		}
	}
	return "", fmt.Errorf("no MCP server %q from %s is installed", key, source)
}

// ResolveSkill turns "<repoURL>::<relativePath>" into the local skill ID.
//
// The path rather than the skill's name: a path is unique within a repository
// by construction, whereas two skills in different directories may share a
// name.
func ResolveSkill(ctx context.Context, client kclient.Client, namespace, ref string) (string, error) {
	if !IsReference(ref) {
		return ref, nil
	}
	source, path, ok := Split(ref)
	if !ok {
		return "", fmt.Errorf("malformed skill reference %q, expected <repoURL>%s<path>", ref, Separator)
	}

	var skills v1.SkillList
	// Indexed on the path, so this reads the few skills sharing it rather than
	// every skill the installation has.
	if err := client.List(ctx, &skills, &kclient.ListOptions{Namespace: namespace},
		kclient.MatchingFields{"spec.relativePath": path}); err != nil {
		return "", fmt.Errorf("list skills: %w", err)
	}
	for _, skill := range skills.Items {
		if mcp.SourceIDForURL(skill.Spec.RepoURL) == mcp.SourceIDForURL(source) {
			return skill.Name, nil
		}
	}
	return "", fmt.Errorf("no skill %q from %s is installed", path, source)
}

// Resolver resolves many references, remembering what failed.
//
// Callers want both halves: the IDs that did resolve, so an agent gets what it
// can have, and the references that did not, so it can be said why the agent is
// incomplete.
type Resolver struct {
	client    kclient.Client
	namespace string
}

func New(client kclient.Client, namespace string) *Resolver {
	return &Resolver{client: client, namespace: namespace}
}

// MCPServers resolves each reference, returning the resolved IDs and the
// references that named nothing installed here.
func (r *Resolver) MCPServers(ctx context.Context, refs []string) (resolved []string, unresolved []string) {
	return r.each(ctx, refs, ResolveMCP)
}

// Skills resolves each reference the same way.
func (r *Resolver) Skills(ctx context.Context, refs []string) (resolved []string, unresolved []string) {
	return r.each(ctx, refs, ResolveSkill)
}

func (r *Resolver) each(ctx context.Context, refs []string,
	resolve func(context.Context, kclient.Client, string, string) (string, error),
) (resolved []string, unresolved []string) {
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		id, err := resolve(ctx, r.client, r.namespace, ref)
		if err != nil {
			unresolved = append(unresolved, ref)
			continue
		}
		resolved = append(resolved, id)
	}
	return resolved, unresolved
}
