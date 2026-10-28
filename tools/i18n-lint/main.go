// Command i18n-lint checks backend message keys against every supported locale.
package main

import (
	"github.com/obot-platform/obot/tools/i18n-lint/analyzer"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(analyzer.Analyzer)
}
