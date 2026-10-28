// Package analyzer checks backend i18n message keys against the source catalogs.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const (
	i18nPath = "github.com/obot-platform/obot/pkg/i18n"
)

var (
	// Analyzer reports missing or empty translations for constant message keys.
	Analyzer  = newAnalyzer()
	reference = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_]*)\}`)
)

type messageKey struct {
	arg ast.Expr
	key string
}

func newAnalyzer() *analysis.Analyzer {
	var root string
	a := &analysis.Analyzer{
		Name: "i18nkeys",
		Doc:  "check backend i18n message keys against every supported locale",
	}
	a.Flags.StringVar(&root, "root", "", "repository root (default: enclosing Go module)")
	a.Run = func(pass *analysis.Pass) (any, error) {
		return run(pass, root)
	}
	return a
}

func run(pass *analysis.Pass, root string) (any, error) {
	importsI18n := pass.Pkg.Path() == i18nPath
	for _, pkg := range pass.Pkg.Imports() {
		if pkg.Path() == i18nPath {
			importsI18n = true
			break
		}
	}
	if !importsI18n {
		return nil, nil
	}
	var keys []messageKey
	for _, file := range pass.Files {
		// Runtime tests deliberately exercise unknown keys and partial catalogs.
		if pass.Pkg.Path() == i18nPath && strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			var ident *ast.Ident
			switch fun := ast.Unparen(call.Fun).(type) {
			case *ast.SelectorExpr:
				ident = fun.Sel
			case *ast.Ident:
				ident = fun
			default:
				return true
			}
			fn, ok := pass.TypesInfo.Uses[ident].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != i18nPath || fn.Type().(*types.Signature).Recv() != nil {
				return true
			}
			index := 1
			switch fn.Name() {
			case "Locales":
				index = 0
			case "Text", "Message", "Resolve":
			default:
				return true
			}
			if len(call.Args) <= index {
				return true
			}
			arg := call.Args[index]
			value := pass.TypesInfo.Types[arg].Value
			if value == nil || value.Kind() != constant.String {
				return true
			}
			check := func(key string) {
				keys = append(keys, messageKey{arg: arg, key: key})
			}
			text := constant.StringVal(value)
			if fn.Name() == "Resolve" {
				for _, match := range reference.FindAllStringSubmatch(text, -1) {
					check(match[1])
				}
			} else {
				check(text)
			}
			return true
		})
	}
	if len(keys) == 0 {
		return nil, nil
	}
	if root == "" {
		var err error
		root, err = moduleRoot(filepath.Dir(pass.Fset.Position(keys[0].arg.Pos()).Filename))
		if err != nil {
			return nil, err
		}
	}
	catalogs, err := readCatalogs(filepath.Join(root, "pkg", "i18n", "messages"))
	if err != nil {
		return nil, err
	}
	locales := make([]string, 0, len(catalogs))
	for locale := range catalogs {
		locales = append(locales, locale)
	}
	slices.Sort(locales)

	for _, ref := range keys {
		for _, locale := range locales {
			if catalogs[locale][ref.key] == "" {
				pass.ReportRangef(ref.arg, "message %q is missing or empty in %s", ref.key, locale)
			}
		}
	}

	return nil, nil
}

func moduleRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find enclosing Go module; specify -root")
		}
		dir = parent
	}
}
