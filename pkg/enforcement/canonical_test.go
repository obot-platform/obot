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
		{"npm already canonical", types.AllowlistServerPackageSourceNPM, "linear-mcp", "linear-mcp"},
		{"npm uppercase", types.AllowlistServerPackageSourceNPM, "Linear-MCP", "linear-mcp"},
		{"npm scoped", types.AllowlistServerPackageSourceNPM, "@Scope/Pkg", "@scope/pkg"},
		{"npm separators are not collapsed", types.AllowlistServerPackageSourceNPM, "a_b.c-d", "a_b.c-d"},
		{"npm trimmed", types.AllowlistServerPackageSourceNPM, "  @scope/pkg\t", "@scope/pkg"},
		{"npm empty", types.AllowlistServerPackageSourceNPM, "", ""},

		// PyPI: PEP 503 — lowercase, every run of - _ . collapsed to one -.
		{"pypi already canonical", types.AllowlistServerPackageSourcePyPI, "mcp-server-git", "mcp-server-git"},
		{"pypi underscores and dots", types.AllowlistServerPackageSourcePyPI, "Mcp_Server.Git", "mcp-server-git"},
		{"pypi dotted namespace", types.AllowlistServerPackageSourcePyPI, "awslabs.core-mcp-server", "awslabs-core-mcp-server"},
		{"pypi mixed separator run", types.AllowlistServerPackageSourcePyPI, "a_-._b", "a-b"},
		{"pypi trimmed", types.AllowlistServerPackageSourcePyPI, "  MCP_Server  ", "mcp-server"},
		{"pypi empty", types.AllowlistServerPackageSourcePyPI, "", ""},
		// PEP 503's regex keeps a leading/trailing separator run as a single "-".
		{"pypi leading separator", types.AllowlistServerPackageSourcePyPI, "__pkg", "-pkg"},
		{"pypi trailing separator", types.AllowlistServerPackageSourcePyPI, "pkg..", "pkg-"},
		{"pypi separators only", types.AllowlistServerPackageSourcePyPI, "._-", "-"},

		// An unmodeled source is left alone (beyond trimming) so validation can
		// reject it on its own terms.
		{"unknown source untouched", types.AllowlistServerPackageSource("cargo"), " Some_Crate ", "Some_Crate"},
		{"empty source untouched", types.AllowlistServerPackageSource(""), "Some.Thing", "Some.Thing"},
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
