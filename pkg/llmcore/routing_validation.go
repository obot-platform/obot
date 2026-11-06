package llmcore

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/maximhq/bifrost/framework/routing"
)

// ruleValidator mirrors the typed environment in the pinned Bifrost routing
// package's createCELEnvironment, which is private. Keep these declarations in
// sync when upgrading Bifrost. GetProgram exposes no AST/output type, so a checked
// AST here is needed to reject non-boolean results before changing live policy.
type ruleValidator struct {
	env *cel.Env
}

func newRuleValidator() (*ruleValidator, error) {
	options := []cel.EnvOption{
		cel.Variable("headers", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("params", cel.MapType(cel.StringType, cel.StringType)),
	}
	for _, name := range []string{
		"model", "provider", "request_type",
		"virtual_key_id", "virtual_key_name", "user_id",
		"team_id", "team_name", "customer_id", "customer_name", "complexity_tier",
	} {
		options = append(options, cel.Variable(name, cel.StringType))
	}
	for _, name := range []string{"tokens_used", "request", "budget_used"} {
		options = append(options, cel.Variable(name, cel.DoubleType))
	}
	env, err := cel.NewEnv(options...)
	if err != nil {
		return nil, err
	}
	return &ruleValidator{env: env}, nil
}

func (v *ruleValidator) validate(expression string) error {
	// Use the same default and key normalization as Bifrost's GetProgram.
	if expression == "" {
		expression = "true"
	}
	expression = routing.NormalizeMapKeysInCEL(expression)
	if err := routing.ValidateCELExpression(expression); err != nil {
		return fmt.Errorf("invalid CEL expression: %w", err)
	}
	ast, issues := v.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("CEL compile error: %w", issues.Err())
	}
	// Require a statically boolean result. A dynamic type (for example, a
	// conditional mixing bool and int) could otherwise fail only on some requests.
	if !ast.OutputType().IsExactType(cel.BoolType) {
		return fmt.Errorf("CEL expression must return boolean, got %s", ast.OutputType())
	}
	return nil
}
