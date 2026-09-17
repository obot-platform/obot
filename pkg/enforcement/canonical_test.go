package enforcement

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestCanonicalPackageName(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source types.AllowlistServerPackageSource
		in     string
		want   string
	}{
		// npm: lowercase, scope preserved.
		{
			name:   "npm already canonical",
			source: types.AllowlistServerPackageSourceNPM,
			in:     "linear-mcp",
			want:   "linear-mcp",
		},
		{
			name:   "npm uppercase",
			source: types.AllowlistServerPackageSourceNPM,
			in:     "Linear-MCP",
			want:   "linear-mcp",
		},
		{
			name:   "npm scoped",
			source: types.AllowlistServerPackageSourceNPM,
			in:     "@Scope/Pkg",
			want:   "@scope/pkg",
		},
		{
			name:   "npm separators are not collapsed",
			source: types.AllowlistServerPackageSourceNPM,
			in:     "a_b.c-d",
			want:   "a_b.c-d",
		},
		{
			name:   "npm trimmed",
			source: types.AllowlistServerPackageSourceNPM,
			in:     "  @scope/pkg\t",
			want:   "@scope/pkg",
		},
		{
			name:   "npm empty",
			source: types.AllowlistServerPackageSourceNPM,
			in:     "",
			want:   "",
		},

		// PyPI: PEP 503 — lowercase, every run of - _ . collapsed to one -.
		{
			name:   "pypi already canonical",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "mcp-server-git",
			want:   "mcp-server-git",
		},
		{
			name:   "pypi underscores and dots",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "Mcp_Server.Git",
			want:   "mcp-server-git",
		},
		{
			name:   "pypi dotted namespace",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "awslabs.core-mcp-server",
			want:   "awslabs-core-mcp-server",
		},
		{
			name:   "pypi mixed separator run",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "a_-._b",
			want:   "a-b",
		},
		{
			name:   "pypi trimmed",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "  MCP_Server  ",
			want:   "mcp-server",
		},
		{
			name:   "pypi empty",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "",
			want:   "",
		},
		// PEP 503's regex keeps a leading/trailing separator run as a single "-".
		{
			name:   "pypi leading separator",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "__pkg",
			want:   "-pkg",
		},
		{
			name:   "pypi trailing separator",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "pkg..",
			want:   "pkg-",
		},
		{
			name:   "pypi separators only",
			source: types.AllowlistServerPackageSourcePyPI,
			in:     "._-",
			want:   "-",
		},

		// An unmodeled source is left alone (beyond trimming) so validation can
		// reject it on its own terms.
		{
			name:   "unknown source untouched",
			source: types.AllowlistServerPackageSource("cargo"),
			in:     " Some_Crate ",
			want:   "Some_Crate",
		},
		{
			name:   "empty source untouched",
			source: types.AllowlistServerPackageSource(""),
			in:     "Some.Thing",
			want:   "Some.Thing",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalPackageName(tt.source, tt.in); got != tt.want {
				t.Fatalf("CanonicalPackageName(%q, %q) = %q, want %q", tt.source, tt.in, got, tt.want)
			}
		})
	}
}

func TestCanonicalPackageNameIsIdempotent(t *testing.T) {
	for _, source := range []types.AllowlistServerPackageSource{
		types.AllowlistServerPackageSourceNPM,
		types.AllowlistServerPackageSourcePyPI,
	} {
		for _, name := range []string{
			"mcp-server-git", "Mcp_Server.Git", "@Scope/Pkg", "awslabs.core-mcp-server", "a_-._b", "",
		} {
			once := CanonicalPackageName(source, name)
			if twice := CanonicalPackageName(source, once); twice != once {
				t.Fatalf("CanonicalPackageName(%q, %q) is not idempotent: %q then %q", source, name, once, twice)
			}
		}
	}
}

func TestCanonicalPackageNameClosesTheCaseAliasBypass(t *testing.T) {
	entry := &types.AllowlistServerPackage{
		Source: types.AllowlistServerPackageSourcePyPI,
		Name:   CanonicalPackageName(types.AllowlistServerPackageSourcePyPI, "mcp-server-git"),
	}
	for _, alias := range []string{"mcp-server-git", "Mcp_Server.Git", "MCP.SERVER.GIT", "mcp_server_git"} {
		actual := &PackageIdentity{
			Source: types.AllowlistServerPackageSourcePyPI,
			Name:   CanonicalPackageName(types.AllowlistServerPackageSourcePyPI, alias),
		}
		if !packageMatches(entry, actual) {
			t.Fatalf("alias %q canonicalized to %q, which does not match the entry %q", alias, actual.Name, entry.Name)
		}
	}
}
