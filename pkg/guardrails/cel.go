package guardrails

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

var (
	envOnce sync.Once
	envs    map[string]*cel.Env
	envErr  error

	programLock sync.Mutex
	programs    = map[string]cel.Program{}
)

func celEnv(target string) (*cel.Env, error) {
	envOnce.Do(func() {
		common := []cel.EnvOption{
			cel.Variable("user", cel.MapType(cel.StringType, cel.DynType)),
			cel.Variable("headers", cel.MapType(cel.StringType, cel.StringType)),
		}
		llm, err := cel.NewEnv(append(common,
			cel.Variable("model", cel.StringType),
			cel.Variable("provider", cel.StringType),
			cel.Variable("params", cel.MapType(cel.StringType, cel.DynType)),
		)...)
		if err != nil {
			envErr = err
			return
		}
		mcp, err := cel.NewEnv(append(common,
			cel.Variable("mcp_server", cel.StringType),
			cel.Variable("mcp_method", cel.StringType),
			cel.Variable("mcp_tool", cel.StringType),
			cel.Variable("mcp_arguments", cel.MapType(cel.StringType, cel.DynType)),
		)...)
		if err != nil {
			envErr = err
			return
		}
		envs = map[string]*cel.Env{types.GuardrailTargetLLM: llm, types.GuardrailTargetMCP: mcp}
	})
	if envErr != nil {
		return nil, envErr
	}
	env, ok := envs[target]
	if !ok {
		return nil, fmt.Errorf("unknown guardrail target %q", target)
	}
	return env, nil
}

// CompileCondition validates a policy condition for a target.
func CompileCondition(target, expr string) (cel.Program, error) {
	key := target + "\x00" + expr
	programLock.Lock()
	defer programLock.Unlock()
	if p, ok := programs[key]; ok {
		return p, nil
	}
	env, err := celEnv(target)
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("condition must evaluate to a boolean, not %s", ast.OutputType())
	}
	p, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	programs[key] = p
	return p, nil
}

// conditionVars fills in every variable the target's environment declares, so conditions
// never fail on a missing top-level variable. A missing map key (headers["x"] when the header
// is absent) still fails at runtime; write has(headers.x) or "x" in headers to test for one.
func conditionVars(target string, identity Identity, vars map[string]any) map[string]any {
	groups := make([]any, 0, len(identity.Groups))
	for _, g := range identity.Groups {
		groups = append(groups, g)
	}
	out := map[string]any{
		"user":    map[string]any{"id": identity.UserID, "groups": groups},
		"headers": map[string]string{},
	}
	if target == types.GuardrailTargetMCP {
		out["mcp_server"] = ""
		out["mcp_method"] = ""
		out["mcp_tool"] = ""
		out["mcp_arguments"] = map[string]any{}
	} else {
		out["model"] = ""
		out["provider"] = ""
		out["params"] = map[string]any{}
	}
	for k, v := range vars {
		if _, declared := out[k]; declared && v != nil {
			out[k] = v
		}
	}
	return out
}

func evalCondition(target, expr string, identity Identity, vars map[string]any) (bool, error) {
	if expr == "" {
		return true, nil
	}
	p, err := CompileCondition(target, expr)
	if err != nil {
		return false, err
	}
	out, _, err := p.Eval(conditionVars(target, identity, vars))
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	return ok && b, nil
}
