# Local Agent Audit Hook Fixtures

These fixtures capture the hook schema for local agent audit logs.
They are representative, schema-shaped samples derived from current
vendor documentation rather than user transcripts.

## Sources Checked

- Claude Code hooks reference: `https://docs.anthropic.com/en/docs/claude-code/hooks`
- Codex hooks reference: `https://developers.openai.com/codex/hooks`
- Cursor hooks reference: `https://cursor.com/docs/hooks`

## Fixture Matrix

| Client | Success | Failure | Timeout/cancel | Oversized payload |
| --- | --- | --- | --- | --- |
| Claude Code | `claude-code/post-tool-use-success.json` | `claude-code/post-tool-use-failure.json` | `claude-code/post-tool-use-timeout.json` | `claude-code/post-tool-use-oversized.json` |
| Codex CLI | `codex-cli/post-tool-use-success.json` | `codex-cli/post-tool-use-failure.json` | `codex-cli/post-tool-use-timeout.json` | `codex-cli/post-tool-use-oversized.json` |
| Cursor | `cursor/after-shell-execution-success.json` | `cursor/post-tool-use-failure.json` | `cursor/after-shell-execution-timeout.json` | `cursor/after-mcp-execution-oversized.json` |

## Normalized Mapping

| Normalized field | Claude Code | Codex CLI | Cursor |
| --- | --- | --- | --- |
| Client name | setup flag `claude-code` | setup flag `codex-cli` | setup flag `cursor` |
| Event name | `hook_event_name` | `hook_event_name` | `hookEventName` |
| Session ID | `session_id` | `session_id` | `session_id` |
| Conversation/thread ID | `transcript_path` basename or encrypted raw payload only | `turn_id`; `transcript_path` basename or encrypted raw payload only | `conversationId` / `requestId` when present |
| Workspace identity | hash `cwd`; basename from `cwd` | hash `cwd`; basename from `cwd` | hash `cwd`; basename from `workspace.name` or `cwd` |
| Tool name | `tool_name` | `tool_name` | `toolName` or shell/MCP event family |
| Tool type/category | infer from `tool_name`: shell for `Bash`, file edit for `Write`/`Edit`/`apply_patch`, MCP for `mcp__*` | infer from `tool_name`: shell for `Bash`, file edit for `apply_patch`, MCP for `mcp__*` | `toolType` when present; otherwise infer from `hookEventName` |
| Tool input | `tool_input` | `tool_input` | `args`, `input`, or `command` |
| Tool output/result | `tool_response` | `tool_response` | `result`, `output`, or event-specific stdout/stderr fields |
| Raw error details | `error` from `PostToolUseFailure` or error-like `tool_response` fields | error-like `tool_response` fields; no separate failure hook currently documented | `error` from `postToolUseFailure`; `stderr`/nonzero exit from shell events |
| Status/exit code | derive from event and `tool_response.exit_code` when present | derive from `tool_response.exit_code` or error status | `status`, `exitCode`, or event-specific result fields |
| Duration | not documented for tool hooks; absent unless present in raw payload | not documented for tool hooks; absent unless present in raw payload | `duration_ms` when present |
| Payload truncated | `payload_truncated` fixture marker, later `payloadTruncated` normalized field | `payload_truncated` fixture marker, later `payloadTruncated` normalized field | `payloadTruncated` when present |

## Setup Hook Event Names

Install these first-slice event paths:

- Claude Code: `PostToolUse` and `PostToolUseFailure`
- Codex CLI: `PostToolUse`
- Cursor: `postToolUse`, `postToolUseFailure`, `afterShellExecution`, and `afterMCPExecution`

## Unsupported Or Lossy Fields

- Claude Code has a separate `PostToolUseFailure` path, but tool duration is not
  documented in the hook input.
- Codex CLI documents `PostToolUse` for successful supported tools and Bash
  nonzero exits. The current public docs do not document a separate post-tool
  failure event, timeout/cancel status field, or duration field.
- Cursor has the richest event list, including shell/MCP after-events and
  `postToolUseFailure`, but not every event has one canonical tool result shape.
  Normalization must accept `command`, `args`, `input`, `result`, `output`,
  `stderr`, `status`, `exitCode`, and `duration_ms` variants.
- Oversized fixtures intentionally use short placeholder payloads plus
  `payload_truncated`/`payloadTruncated` markers.
