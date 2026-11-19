package adminmcp

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/obot-platform/obot/pkg/openapi"
)

const (
	maxValidationItems       = 200
	maxValidationImports     = 10
	maxValidationSchemaBytes = 8 << 20
	catalogValidationTimeout = time.Minute
)

// validationImporter bounds amplification from small source URLs to downloaded
// snapshots. Its budget belongs to one call, including failed import attempts.
type validationImporter struct {
	importer mcpcatalog.OpenAPIImporter
	imports  int
	bytes    int
	err      error
}

func (i *validationImporter) Import(ctx context.Context, config types.OpenAPIRuntimeConfig) (*openapi.Result, error) {
	if err := ctx.Err(); err != nil {
		i.err = err
		return nil, err
	}
	if i.imports >= maxValidationImports {
		i.err = fmt.Errorf("validate at most %d OpenAPI sources per call; split the files into smaller batches", maxValidationImports)
		return nil, i.err
	}
	i.imports++
	result, err := i.importer.Import(ctx, config)
	if ctx.Err() != nil {
		i.err = ctx.Err()
		return nil, i.err
	}
	if err != nil {
		return nil, err
	}
	if len(result.Schema) > maxValidationSchemaBytes-i.bytes {
		i.err = fmt.Errorf("imported OpenAPI schemas exceed the %d MiB validation budget; split the files into smaller batches", maxValidationSchemaBytes>>20)
		return nil, i.err
	}
	i.bytes += len(result.Schema)
	return result, nil
}
