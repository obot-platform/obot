package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kballard/go-shellquote"
)

const (
	auditHookIDEnv       = "OBOT_AUDIT_HOOK_ID"
	auditHookID          = "obot-local-agent-audit-v1"
	auditHookTimeoutSecs = 10
)

var lookPath = exec.LookPath // function alias for unit testing

// auditObotBinaryPath returns the absolute path to the currently running obot
// executable for use in hook commands.
func auditObotBinaryPath() (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve obot binary path: %w", err)
	}
	if !filepath.IsAbs(binary) {
		binary, err = filepath.Abs(binary)
		if err != nil {
			return "", fmt.Errorf("resolve obot binary path: %w", err)
		}
	}
	return binary, nil
}

func installAuditHook(ctx context.Context, home, binary, client string) auditHookSetupResult {
	if ctx != nil && ctx.Err() != nil {
		return auditHookSetupResult{Client: client, Error: ctx.Err().Error()}
	}
	command := auditHookCommand(binary, client)
	var result auditHookSetupResult
	switch client {
	case auditClientClaudeCode:
		path := filepath.Join(home, ".claude", "settings.json")
		result = installClaudeLikeAuditHooks(path, client, command, []string{"PostToolUse", "PostToolUseFailure"})
	case auditClientCodex:
		result = installCodexAuditHooks(home, command)
	case auditClientCursor:
		path := filepath.Join(home, ".cursor", "hooks.json")
		result = installFlatAuditHooks(path, client, command, []string{"postToolUse", "postToolUseFailure"}, true)
	case auditClientVSCode:
		path := filepath.Join(home, ".copilot", "hooks", "obot-audit.json")
		result = installFlatAuditHooks(path, client, command, []string{"PostToolUse"}, false)
	default:
		return auditHookSetupResult{Client: client, Error: "unsupported client"}
	}
	return result
}

// installClaudeLikeAuditHooks installs hooks into a Claude/Codex-style JSON
// file whose event values are matcher groups, where each matcher group contains
// a "hooks" array. It preserves unrelated matcher groups and replaces any
// prior Obot-managed command hook for the requested events.
//
// Example input fragment:
//
//	{"hooks":{"PostToolUse":[{"matcher":"Read","hooks":[{"command":"user.sh"}]}]}}
//
// Example output fragment:
//
//	{"hooks":{"PostToolUse":[{"matcher":"Read","hooks":[{"command":"user.sh"}]},{"matcher":".*","hooks":[{"command":"/usr/bin/obot audit submit --format claude-code --input -"}]}]}}
func installClaudeLikeAuditHooks(path, client, command string, events []string) auditHookSetupResult {
	root, err := readJSONObjectFile(path)
	if err != nil {
		return auditHookConfigError(client, path, err)
	}
	hooks, err := objectField(root, "hooks")
	if err != nil {
		return auditHookConfigError(client, path, err)
	}
	for _, event := range events {
		if err := validateMatcherGroups(hooks[event]); err != nil {
			return auditHookConfigError(client, path, fmt.Errorf("hooks.%s: %w", event, err))
		}
		hooks[event] = updateMatcherGroups(hooks[event], command)
	}
	root["hooks"] = hooks
	if err := writeJSONObjectFile(path, root); err != nil {
		return auditHookSetupResult{Client: client, ConfigPath: path, Error: err.Error()}
	}
	return auditHookSetupResult{Client: client, ConfigPath: path, Installed: true, Message: "installed Obot audit hook"}
}

// installFlatAuditHooks installs hooks into a flat hook JSON file whose event
// values are arrays of command hook objects. Cursor uses this shape; the VS Code
// user hook file also uses this shape.
//
// Example input fragment:
//
//	{"hooks":{"postToolUse":[{"command":"user.sh"}]}}
//
// Example output fragment:
//
//	{"hooks":{"postToolUse":[{"command":"user.sh"},{"command":"/usr/bin/obot audit submit --format cursor --input -","failClosed":false}]}}
func installFlatAuditHooks(path, client, command string, events []string, cursor bool) auditHookSetupResult {
	root, err := readJSONObjectFile(path)
	if err != nil {
		return auditHookConfigError(client, path, err)
	}
	if cursor {
		if _, ok := root["version"]; !ok {
			root["version"] = float64(1)
		}
	}
	hooks, err := objectField(root, "hooks")
	if err != nil {
		return auditHookConfigError(client, path, err)
	}
	for _, event := range events {
		if err := validateFlatHooks(hooks[event]); err != nil {
			return auditHookConfigError(client, path, fmt.Errorf("hooks.%s: %w", event, err))
		}
		hooks[event] = updateFlatHooks(hooks[event], command, cursor)
	}
	root["hooks"] = hooks
	if err := writeJSONObjectFile(path, root); err != nil {
		return auditHookSetupResult{Client: client, ConfigPath: path, Error: err.Error()}
	}
	return auditHookSetupResult{Client: client, ConfigPath: path, Installed: true, Message: "installed Obot audit hook"}
}

// installCodexAuditHooks installs Codex audit hooks in the user's Codex config
// layer. New installs prefer ~/.codex/hooks.json; if ~/.codex/config.toml
// already contains inline hooks, this updates that TOML file instead to avoid
// creating two hook representations in the same layer.
func installCodexAuditHooks(home, command string) auditHookSetupResult {
	configPath := filepath.Join(home, ".codex", "config.toml")
	hooksPath := filepath.Join(home, ".codex", "hooks.json")
	inline, err := codexConfigHasInlineHooks(configPath)
	if err != nil {
		return auditHookConfigError(auditClientCodex, configPath, err)
	}
	if inline {
		if err := writeCodexInlineAuditHooks(configPath, command); err != nil {
			return auditHookSetupResult{Client: auditClientCodex, ConfigPath: configPath, Error: err.Error()}
		}
		return auditHookSetupResult{Client: auditClientCodex, ConfigPath: configPath, Installed: true, Message: "installed Obot audit hook"}
	}
	return installClaudeLikeAuditHooks(hooksPath, auditClientCodex, command, []string{"PostToolUse"})
}

func auditHookConfigError(client, path string, err error) auditHookSetupResult {
	return auditHookSetupResult{
		Client:     client,
		ConfigPath: path,
		Malformed:  true,
		Message:    fmt.Sprintf("skipped malformed config %s", path),
		Error:      err.Error(),
	}
}

// readJSONObjectFile reads a JSON object from path. Missing and empty files are
// treated as empty objects, while malformed JSON, non-object JSON, and trailing
// JSON values return errors so setup can skip the file safely.
//
// Example input:
//
//	{"hooks":{}}
//
// Example output:
//
//	map[string]any{"hooks": map[string]any{}}
func readJSONObjectFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// writeJSONObjectFile writes a JSON object with stable indentation and 0600
// permissions, creating parent directories as needed.
func writeJSONObjectFile(path string, value map[string]any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// objectField returns root[key] as a JSON object, creating an empty object when
// the field is absent. If the field exists but is not an object, it returns an
// error so setup does not overwrite user-managed malformed config.
//
// Example input:
//
//	root={"hooks":{"PostToolUse":[]}}, key="hooks"
//
// Example output:
//
//	map[string]any{"PostToolUse": []any{}}
func objectField(root map[string]any, key string) (map[string]any, error) {
	raw, ok := root[key]
	if !ok || raw == nil {
		return map[string]any{}, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q must be an object", key)
	}
	return object, nil
}

// validateMatcherGroups checks the Claude/Codex hook shape before mutation.
// A nil value is valid because setup may create a missing event array.
//
// Valid example:
//
//	[]any{map[string]any{"matcher": ".*", "hooks": []any{map[string]any{"command": "x"}}}}
//
// Invalid example:
//
//	map[string]any{"command": "x"}
func validateMatcherGroups(raw any) error {
	if raw == nil {
		return nil
	}
	groups, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("must be an array")
	}
	for i, entry := range groups {
		group, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("entry %d must be an object", i)
		}
		rawHooks, ok := group["hooks"]
		if !ok {
			return fmt.Errorf("entry %d missing hooks array", i)
		}
		if err := validateFlatHooks(rawHooks); err != nil {
			return fmt.Errorf("entry %d hooks: %w", i, err)
		}
	}
	return nil
}

// validateFlatHooks checks the Cursor/VS Code hook shape before mutation. A nil
// value is valid because setup may create a missing event array.
//
// Valid example:
//
//	[]any{map[string]any{"command": "x"}}
//
// Invalid example:
//
//	map[string]any{"command": "x"}
func validateFlatHooks(raw any) error {
	if raw == nil {
		return nil
	}
	hooks, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("must be an array")
	}
	for i, entry := range hooks {
		if _, ok := entry.(map[string]any); !ok {
			return fmt.Errorf("entry %d must be an object", i)
		}
	}
	return nil
}

// updateMatcherGroups returns a matcher-group event array with exactly one
// Obot-managed matcher group appended. Existing Obot-managed hook objects are
// removed first, making repeated setup idempotent while preserving unrelated
// hooks.
//
// Example output:
//
//	[{"matcher":".*","hooks":[{"env":{"OBOT_AUDIT_HOOK_ID":"obot-local-agent-audit-v1"}}]}]
func updateMatcherGroups(raw any, command string) []any {
	existing, _ := raw.([]any)
	out := make([]any, 0, len(existing)+1)
	for _, entry := range existing {
		group, ok := entry.(map[string]any)
		if !ok {
			out = append(out, entry)
			continue
		}
		hooks, _ := group["hooks"].([]any)
		filtered := removeManagedHookObjects(hooks)
		if len(filtered) == 0 && groupOwnedByObot(group) {
			continue
		}
		if len(filtered) != len(hooks) {
			group = maps.Clone(group)
			group["hooks"] = filtered
		}
		out = append(out, group)
	}
	out = append(out, map[string]any{
		"matcher": ".*",
		"hooks":   []any{managedHookObject(command)},
	})
	return out
}

// updateFlatHooks returns a flat event hook array with exactly one Obot-managed
// hook appended. For Cursor, it also sets failClosed=false so audit transport
// cannot block local agent behavior.
//
// Example output for Cursor:
//
//	[{"command":"/usr/bin/obot audit submit --format cursor --input -","failClosed":false}]
func updateFlatHooks(raw any, command string, cursor bool) []any {
	existing, _ := raw.([]any)
	out := make([]any, 0, len(existing)+1)
	for _, entry := range existing {
		hook, ok := entry.(map[string]any)
		if ok && managedHookObjectPresent(hook) {
			continue
		}
		out = append(out, entry)
	}
	hook := managedHookObject(command)
	if cursor {
		hook["failClosed"] = false
	}
	out = append(out, hook)
	return out
}

// removeManagedHookObjects filters Obot-managed hook objects out of an event's
// hook list. Hooks are identified only by the stable marker environment
// variable, not by command text, so a binary path change updates cleanly.
//
// Example input:
//
//	[userHook, obotHook]
//
// Example output:
//
//	[userHook]
func removeManagedHookObjects(hooks []any) []any {
	out := make([]any, 0, len(hooks))
	for _, entry := range hooks {
		hook, ok := entry.(map[string]any)
		if ok && managedHookObjectPresent(hook) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// groupOwnedByObot reports whether a matcher group contains only Obot-managed
// hooks. Such groups can be dropped entirely during update instead of being
// preserved as empty matcher groups.
//
// Example output:
//
//	group with only OBOT_AUDIT_HOOK_ID hook -> true
func groupOwnedByObot(group map[string]any) bool {
	hooks, _ := group["hooks"].([]any)
	return len(hooks) > 0 && len(removeManagedHookObjects(hooks)) == 0
}

// managedHookObject builds the command hook object that setup writes into JSON
// hook config files.
//
// Example output:
//
//	{"type":"command","command":"/usr/bin/obot audit submit --format vscode --input -","timeout":10,"env":{"OBOT_AUDIT_HOOK_ID":"obot-local-agent-audit-v1"}}
func managedHookObject(command string) map[string]any {
	return map[string]any{
		"type":          "command",
		"command":       command,
		"timeout":       float64(auditHookTimeoutSecs),
		"statusMessage": "Sending audit event to Obot",
		"env": map[string]any{
			auditHookIDEnv: auditHookID,
		},
	}
}

// managedHookObjectPresent reports whether hook is one of this feature's
// managed entries. It intentionally checks only env.OBOT_AUDIT_HOOK_ID so
// setup can replace older Obot command paths without duplicating hooks.
//
// Example output:
//
//	{"env":{"OBOT_AUDIT_HOOK_ID":"obot-local-agent-audit-v1"}} -> true
func managedHookObjectPresent(hook map[string]any) bool {
	env, ok := hook["env"].(map[string]any)
	if !ok {
		return false
	}
	value, _ := env[auditHookIDEnv].(string)
	return value == auditHookID
}

func auditHookCommand(binary, client string) string {
	return shellquote.Join(binary, "audit", "submit", "--format", client, "--input", "-")
}

// codexConfigHasInlineHooks parses config.toml and reports whether that file
// already contains an inline [hooks] table. Parse errors are returned so setup
// can skip malformed TOML without overwriting it.
func codexConfigHasInlineHooks(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var parsed any
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		return false, err
	}
	return strings.Contains(string(data), "[hooks.") || strings.Contains(string(data), "[[hooks.") || strings.Contains(string(data), "[hooks]"), nil
}

const (
	codexInlineBegin = "# BEGIN OBOT_AUDIT_HOOK_ID=obot-local-agent-audit-v1"
	codexInlineEnd   = "# END OBOT_AUDIT_HOOK_ID=obot-local-agent-audit-v1"
)

// writeCodexInlineAuditHooks appends or replaces the Obot-managed inline Codex
// TOML block. The block is fenced with comments so repeated setup updates the
// existing block without disturbing unrelated inline hooks.
//
// Example output fragment:
//
//	# BEGIN OBOT_AUDIT_HOOK_ID=obot-local-agent-audit-v1
//	[[hooks.PostToolUse]]
//	matcher = ".*"
//	# END OBOT_AUDIT_HOOK_ID=obot-local-agent-audit-v1
func writeCodexInlineAuditHooks(path, command string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	body := removeCodexInlineManagedBlock(string(data))
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += "\n" + codexInlineBegin + "\n"
	body += "[[hooks.PostToolUse]]\n"
	body += "matcher = \".*\"\n\n"
	body += "[[hooks.PostToolUse.hooks]]\n"
	body += "type = \"command\"\n"
	body += fmt.Sprintf("command = %q\n", command)
	body += fmt.Sprintf("timeout = %d\n", auditHookTimeoutSecs)
	body += "statusMessage = \"Sending audit event to Obot\"\n\n"
	body += "[hooks.PostToolUse.hooks.env]\n"
	body += fmt.Sprintf("%s = %q\n", auditHookIDEnv, auditHookID)
	body += codexInlineEnd + "\n"
	return os.WriteFile(path, []byte(body), 0600)
}

// removeCodexInlineManagedBlock removes the fenced Obot-managed TOML block from
// a Codex config string. If no complete fenced block exists, it returns the
// input unchanged.
func removeCodexInlineManagedBlock(data string) string {
	start := strings.Index(data, codexInlineBegin)
	if start < 0 {
		return data
	}
	end := strings.Index(data[start:], codexInlineEnd)
	if end < 0 {
		return data
	}
	end += start + len(codexInlineEnd)
	for end < len(data) && (data[end] == '\n' || data[end] == '\r') {
		end++
	}
	return data[:start] + data[end:]
}

// inspectAuditHook returns the current hook status for one supported client.
// This is used by "obot audit status" to combine hook installation state with
// auth and spool state.
func inspectAuditHook(home, client string) auditHookStatus {
	switch client {
	case auditClientClaudeCode:
		path := filepath.Join(home, ".claude", "settings.json")
		return inspectClaudeLikeAuditHook(client, path, []string{"PostToolUse", "PostToolUseFailure"})
	case auditClientCodex:
		return inspectCodexAuditHook(home)
	case auditClientCursor:
		path := filepath.Join(home, ".cursor", "hooks.json")
		return inspectFlatAuditHook(client, path, []string{"postToolUse", "postToolUseFailure"})
	case auditClientVSCode:
		path := filepath.Join(home, ".copilot", "hooks", "obot-audit.json")
		return inspectFlatAuditHook(client, path, []string{"PostToolUse"})
	default:
		return auditHookStatus{Client: client, Error: "unsupported client"}
	}
}

// inspectClaudeLikeAuditHook checks a Claude/Codex-style JSON hook file and
// marks it installed only when every requested event contains an Obot-managed
// hook.
//
// Example input:
//
//	events={"PostToolUse","PostToolUseFailure"}
//
// Example output:
//
//	Installed=true only if both events contain OBOT_AUDIT_HOOK_ID.
func inspectClaudeLikeAuditHook(client, path string, events []string) auditHookStatus {
	root, err := readJSONObjectFile(path)
	status := auditHookStatus{Client: client, DisplayName: auditClientDisplayName(client), ConfigPath: path}
	if err != nil {
		status.Malformed = true
		status.Error = err.Error()
		return status
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return status
	}
	for _, event := range events {
		if !matcherGroupsHaveManagedHook(hooks[event]) {
			return status
		}
	}
	status.Installed = true
	return status
}

// inspectFlatAuditHook checks a flat Cursor/VS Code hook file and marks it
// installed only when every requested event contains an Obot-managed hook.
func inspectFlatAuditHook(client, path string, events []string) auditHookStatus {
	root, err := readJSONObjectFile(path)
	status := auditHookStatus{Client: client, DisplayName: auditClientDisplayName(client), ConfigPath: path}
	if err != nil {
		status.Malformed = true
		status.Error = err.Error()
		return status
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return status
	}
	for _, event := range events {
		if !flatHooksHaveManagedHook(hooks[event]) {
			return status
		}
	}
	status.Installed = true
	return status
}

// inspectCodexAuditHook checks Codex's active user-layer representation. If
// config.toml has inline hooks, status is based on the TOML marker; otherwise it
// checks ~/.codex/hooks.json.
func inspectCodexAuditHook(home string) auditHookStatus {
	configPath := filepath.Join(home, ".codex", "config.toml")
	inline, err := codexConfigHasInlineHooks(configPath)
	if err != nil {
		return auditHookStatus{Client: auditClientCodex, DisplayName: auditClientDisplayName(auditClientCodex), ConfigPath: configPath, Malformed: true, Error: err.Error()}
	}
	if inline {
		data, _ := os.ReadFile(configPath)
		return auditHookStatus{
			Client:      auditClientCodex,
			DisplayName: auditClientDisplayName(auditClientCodex),
			ConfigPath:  configPath,
			Installed:   strings.Contains(string(data), auditHookID),
		}
	}
	return inspectClaudeLikeAuditHook(auditClientCodex, filepath.Join(home, ".codex", "hooks.json"), []string{"PostToolUse"})
}

// matcherGroupsHaveManagedHook reports whether any matcher group in a
// Claude/Codex-style event array contains an Obot-managed hook.
//
// Example output:
//
//	[{"hooks":[{"env":{"OBOT_AUDIT_HOOK_ID":"obot-local-agent-audit-v1"}}]}] -> true
func matcherGroupsHaveManagedHook(raw any) bool {
	groups, _ := raw.([]any)
	for _, entry := range groups {
		group, _ := entry.(map[string]any)
		if flatHooksHaveManagedHook(group["hooks"]) {
			return true
		}
	}
	return false
}

// flatHooksHaveManagedHook reports whether a flat event hook array contains an
// Obot-managed hook.
//
// Example output:
//
//	[{"env":{"OBOT_AUDIT_HOOK_ID":"obot-local-agent-audit-v1"}}] -> true
func flatHooksHaveManagedHook(raw any) bool {
	hooks, _ := raw.([]any)
	for _, entry := range hooks {
		hook, ok := entry.(map[string]any)
		if ok && managedHookObjectPresent(hook) {
			return true
		}
	}
	return false
}

func auditClientDisplayName(client string) string {
	switch client {
	case auditClientClaudeCode:
		return "Claude Code"
	case auditClientCodex:
		return "Codex"
	case auditClientCursor:
		return "Cursor"
	case auditClientVSCode:
		return "VS Code"
	default:
		return client
	}
}
