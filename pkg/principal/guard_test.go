package principal

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	principalImportPath = "github.com/obot-platform/obot/pkg/principal"

	// ownerExpression matches arguments that look like a stored owner or a caller's UID.
	ownerExpression = regexp.MustCompile(`(?i)userid|owner|getuid\(\)`)

	// allowedOwnerParses are the user-only parses that predate this guard, keyed by file and parsed expression. Do
	// not add to this list: decode owners with Parse, and use Principal.UserID where only a user is acceptable.
	allowedOwnerParses = map[string]string{
		"pkg/api/request.go: r.User.GetUID()":                    "api.Context.UserID returns 0 for any caller that is not a user",
		"pkg/api/handlers/claudeaccounts.go: account.UserID":     "imported Claude account mappings name Obot users",
		"pkg/claudecredentials/credentials.go: inst.Spec.UserID": "hosted Claude sessions belong to users",
		"pkg/clauderunner/access.go: owner":                      "hosted Claude sessions belong to users",
		"pkg/gateway/client/claude_tag.go: userID":               "Claude Tag identities are users",
	}

	// allowedOnBehalfOfRecorders are the functions outside this package, keyed by file and function name, that may
	// record the user a caller acts for (RecordOnBehalfOf or OnBehalfOfExtra). Each must be an authenticator that opted
	// in to tokens with which an agent acts on behalf of a user (persistent.TokenService.DecodeDelegatedToken), and must
	// record it on an agent principal only, after its kind.
	allowedOnBehalfOfRecorders = map[string]string{
		"pkg/gateway/server/delegated_auth.go: AuthenticateRequest": "the authenticator of delegated tokens, which records the user on the agent's principal after AgentUserInfo records its kind",
	}
)

// sourceFile is a parsed non-test Go file of the repository, outside this package.
type sourceFile struct {
	rel  string
	src  []byte
	fset *token.FileSet
	file *ast.File
}

func (f sourceFile) text(n ast.Node) string {
	return string(f.src[f.fset.Position(n.Pos()).Offset:f.fset.Position(n.End()).Offset])
}

// principalName returns the name the file imports this package as, or "" if it does not import it.
func (f sourceFile) principalName() string {
	for _, spec := range f.file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err != nil || path != principalImportPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return "principal"
	}
	return ""
}

// walkSourceFiles calls fn for every non-test Go file of the repository outside this package.
func walkSourceFiles(t *testing.T, fn func(sourceFile)) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root not found at %s: %v", root, err)
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch {
			case rel == "pkg/principal", rel == "ui", rel == "docs", rel == "bin",
				d.Name() == "node_modules", d.Name() == "vendor", d.Name() == "testdata",
				rel != "." && strings.HasPrefix(d.Name(), "."):
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		fn(sourceFile{rel: rel, src: src, fset: fset, file: file})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOwnersAreNotParsedAsUserIDs fails on a new strconv.ParseUint of a stored owner or a caller's UID outside this
// package. Such a parse reads an agent owner ("agent:<id>") as malformed rather than as an agent.
func TestOwnersAreNotParsedAsUserIDs(t *testing.T) {
	seen := map[string]bool{}
	walkSourceFiles(t, func(f sourceFile) {
		ast.Inspect(f.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "ParseUint" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "strconv" {
				return true
			}
			arg := f.text(call.Args[0])
			if !ownerExpression.MatchString(arg) {
				return true
			}
			key := f.rel + ": " + arg
			if _, ok := allowedOwnerParses[key]; ok {
				seen[key] = true
				return true
			}
			t.Errorf("%s: strconv.ParseUint(%s) parses an owner as a user ID; use principal.Parse and Principal.UserID", f.fset.Position(call.Pos()), arg)
			return true
		})
	})

	for key := range allowedOwnerParses {
		if !seen[key] {
			t.Errorf("allowed owner parse %q no longer exists; remove it from allowedOwnerParses", key)
		}
	}
}

// TestUserStatusIsRecordedWithAKind fails when a function outside this package records a user's status without
// recording the principal's kind. FromUserInfo requires the kind, so such a principal would not be read as a user.
// RecordPrincipal records both.
func TestUserStatusIsRecordedWithAKind(t *testing.T) {
	recorders := 0
	walkSourceFiles(t, func(f sourceFile) {
		name := f.principalName()
		if name == "" {
			return
		}
		for _, decl := range f.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var recordsStatus, recordsKind bool
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != name {
					return true
				}
				switch sel.Sel.Name {
				case "RecordUserStatus", "UserStatusExtra":
					recordsStatus = true
				case "RecordKind", "KindExtra":
					recordsKind = true
				case "RecordPrincipal":
					recordsStatus, recordsKind = true, true
				}
				return true
			})
			if !recordsStatus {
				continue
			}
			recorders++
			if !recordsKind {
				t.Errorf("%s: %s records a user status without recording the principal kind; call %s.RecordPrincipal", f.fset.Position(fn.Pos()), fn.Name.Name, name)
			}
		}
	})
	// The guard is only meaningful while it finds the authenticators that record a status.
	if recorders == 0 {
		t.Fatal("found no function that records a user status")
	}
}

// TestOnBehalfOfIsRecordedOnlyByAllowedAuthenticators fails when a function outside this package that is not in
// allowedOnBehalfOfRecorders records the user a caller acts for. Whoever records it decides that an agent acts for a
// user, so only an authenticator that verified a delegated token may.
func TestOnBehalfOfIsRecordedOnlyByAllowedAuthenticators(t *testing.T) {
	seen := map[string]bool{}
	walkSourceFiles(t, func(f sourceFile) {
		name := f.principalName()
		if name == "" {
			return
		}
		for _, decl := range f.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != name {
					return true
				}
				if sel.Sel.Name != "RecordOnBehalfOf" && sel.Sel.Name != "OnBehalfOfExtra" {
					return true
				}
				key := f.rel + ": " + fn.Name.Name
				if _, ok := allowedOnBehalfOfRecorders[key]; ok {
					seen[key] = true
					return true
				}
				t.Errorf("%s: %s records the user a caller acts for; only an authenticator of delegated tokens may (allowedOnBehalfOfRecorders)", f.fset.Position(sel.Pos()), fn.Name.Name)
				return true
			})
		}
	})

	for key := range allowedOnBehalfOfRecorders {
		if !seen[key] {
			t.Errorf("allowed on-behalf-of recorder %q no longer records it; remove it from allowedOnBehalfOfRecorders", key)
		}
	}
}
