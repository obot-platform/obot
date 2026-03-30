# MessagePolicy Design Document

## Overview

MessagePolicy is a new global (admin-only) resource that enables natural language content policies at the LLM proxy level. Policies are evaluated against user messages, LLM tool calls, or both, and violations result in the offending content being replaced with a system notification.

## Resource Definition

### MessagePolicy Type

```go
// apiclient/types/messagepolicy.go

type MessagePolicyManifest struct {
    DisplayName string          `json:"displayName"`
    Description string          `json:"description,omitempty"`
    Definition  string          `json:"definition"`
    Direction   PolicyDirection `json:"direction"`
    Subjects    []Subject       `json:"subjects"`
}

type PolicyDirection string

const (
    PolicyDirectionUserMessage PolicyDirection = "user-message"
    PolicyDirectionToolCalls   PolicyDirection = "tool-calls"
    PolicyDirectionBoth        PolicyDirection = "both"
)
```

- **DisplayName**: Human-readable name (e.g., "Economy Travel Only")
- **Description**: Optional longer explanation for admins
- **Definition**: Freeform natural language describing what is allowed or disallowed (e.g., "Do not allow the user to book travel above economy class")
- **Direction**: Whether the policy applies to user messages, tool calls, or both
- **Subjects**: Reuses the existing `Subject` type (`user`, `group`, `selector`/wildcard) from `accesscontrolrule.go`

### Storage Type

```go
// pkg/storage/apis/obot.obot.ai/v1/messagepolicy.go

type MessagePolicy struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              MessagePolicySpec `json:"spec,omitempty"`
}

type MessagePolicySpec struct {
    Manifest types.MessagePolicyManifest `json:"manifest,omitempty"`
}
```

- Storage prefix: `mp1`
- No finalizers needed (no external references to clean up)

### Validation

- At least one subject required
- At least one of: wildcard subject OR specific user/group subjects
- `definition` must be non-empty
- `direction` must be one of the three valid values
- Standard duplicate/wildcard checks matching existing policy types

## API

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/message-policies` | List all message policies |
| GET | `/api/message-policies/{id}` | Get a specific message policy |
| POST | `/api/message-policies` | Create a message policy |
| PUT | `/api/message-policies/{id}` | Update a message policy |
| DELETE | `/api/message-policies/{id}` | Delete a message policy |

All endpoints are admin-only. The handler follows the same pattern as `ModelAccessPolicyHandler` in `pkg/api/handlers/modelaccesspolicy.go`.

### Handler

```go
// pkg/api/handlers/messagepolicy.go

type MessagePolicyHandler struct{}

func (h *MessagePolicyHandler) List(req api.Context) error { ... }
func (h *MessagePolicyHandler) Get(req api.Context) error { ... }
func (h *MessagePolicyHandler) Create(req api.Context) error { ... }
func (h *MessagePolicyHandler) Update(req api.Context) error { ... }
func (h *MessagePolicyHandler) Delete(req api.Context) error { ... }
```

## Policy Evaluation

### Enforcement Point

Policy evaluation is added to the LLM proxy in `pkg/gateway/server/llmproxy.go`. It runs **only when applicable policies exist** for the current user.

### Helper

A new `pkg/messagepolicy/helper.go` provides the policy evaluation engine, following the pattern of `pkg/modelaccesspolicy/helper.go`.

```go
type Helper struct {
    // Kubernetes informer/indexer for MessagePolicy objects
    // Indexes by user ID, group ID, and wildcard selector
}

// GetApplicablePolicies returns all MessagePolicies that apply to a given user and direction.
func (h *Helper) GetApplicablePolicies(user kuser.Info, direction PolicyDirection) ([]types.MessagePolicyManifest, error)

// EvaluateMessage runs all applicable policies against a message.
// Returns a slice of violations (empty if all policies pass).
func (h *Helper) EvaluateMessage(ctx context.Context, policies []types.MessagePolicyManifest, messages []ConversationMessage, targetMessage ConversationMessage) ([]PolicyViolation, error)
```

### Two-Stage LLM Evaluation

Each policy is evaluated independently against the message, with all policies running **in parallel** via goroutines.

**Stage 1: Yes/No Classification**

A single LLM call using the `llm-mini` model alias determines whether the message violates the policy.

```
System prompt:
You are a policy compliance checker. You must determine whether the following
message violates the given policy. Respond with exactly "yes" if the message
is compliant, or "no" if it violates the policy. Do not explain your reasoning.

Policy: {policy.Definition}

---

Conversation context (tool outputs redacted):
{conversation history with tool outputs replaced by "[tool output redacted]"}

---

Message to evaluate:
{target message content}
```

- Model: resolved from `llm-mini` default model alias
- Token usage is **not** counted against the user's token limit or tracked in `RunTokenActivity`
- On LLM call failure (timeout, rate limit, error): **fail closed** (treat as violation)

**Stage 2: Explanation Generation**

Only runs if Stage 1 returns "no" (violation detected). A second LLM call generates a human-readable explanation.

```
System prompt:
A message was blocked for violating a policy. Write a brief explanation of why
the message was blocked. Write the explanation for an AI assistant to relay to the user.

Policy: {policy.Definition}

Message that was blocked:
{target message content}
```

The audience instruction is always LLM-facing ("Write the explanation for an AI assistant to relay to the user") regardless of direction, since both blocked user messages and blocked tool calls are surfaced to the user via the LLM.

### Conversation Context for the Judge

The policy judge receives the full conversation history with the following treatment:

| Message Type | Treatment |
|---|---|
| User messages | Included as-is |
| Assistant text messages | Included as-is |
| Assistant tool calls | Included (tool name + arguments) |
| Tool results/outputs | Replaced with `[tool output redacted]` |
| System messages | Excluded |

This provides enough context to evaluate policies that depend on conversation flow (e.g., "don't allow booking above economy") while guarding against prompt injection via tool outputs.

### Parallel Evaluation

```go
func (h *Helper) EvaluateMessage(ctx context.Context, ...) ([]PolicyViolation, error) {
    var (
        mu         sync.Mutex
        violations []PolicyViolation
        wg         sync.WaitGroup
    )

    for _, policy := range policies {
        wg.Add(1)
        go func(p types.MessagePolicyManifest) {
            defer wg.Done()

            // Stage 1: yes/no check
            compliant, err := h.checkCompliance(ctx, p, messages, targetMessage)
            if err != nil || !compliant {
                // Stage 2: generate explanation (or use generic message on error)
                explanation := h.generateExplanation(ctx, p, targetMessage, direction)

                mu.Lock()
                violations = append(violations, PolicyViolation{
                    PolicyName:  p.DisplayName,
                    Explanation: explanation,
                })
                mu.Unlock()
            }
        }(policy)
    }

    wg.Wait()
    return violations, nil
}
```

## Integration with the LLM Proxy

### User Message Evaluation (Input)

**Where**: In `dispatchLLMProxy`, after token decoding and body parsing, before model resolution and proxying.

**Flow**:
1. Extract user identity from decoded token (`UserID`, `UserGroups`, auth provider groups)
2. Call `helper.GetApplicablePolicies(user, PolicyDirectionUserMessage)` — if no policies apply, skip evaluation entirely
3. Parse `messages` array from request body
4. Identify the last user message as the target
5. Call `helper.EvaluateMessage(...)` with the conversation history and target message
6. **If violation detected**: Replace the last user message content in the request body with:

```
<system_notification>This message was removed by the system for violating policies. Inform the user that you cannot complete their requested action due to a policy violation. The following explanation was generated for you to relay to the user: {stage 2 explanation}</system_notification>
```

7. Re-serialize the modified body and continue the proxy flow (the LLM receives the modified message and responds acknowledging the policy violation)

### Tool Call Evaluation (Output)

**Where**: In the response modifier (`responseModifier`), using a pipe-based streaming design with a background goroutine.

**Key design principle**: Only tool calls are evaluated against output policies. Text-only responses stream through untouched with zero latency impact. Text content in mixed responses (text + tool calls) is also streamed through immediately — only the tool call portions are held back for evaluation.

**Flow (streaming responses)**:
1. When tool-call policies exist for the user, `modifyResponse` creates an `io.Pipe` and launches a goroutine (`streamAndEvaluateToolCalls`)
2. The goroutine reads upstream SSE chunks line by line:
   - **Text delta chunks** (has `delta.content`, no `delta.tool_calls`): forwarded to the client immediately
   - **Tool call delta chunks** (has `delta.tool_calls`): buffered internally, tool call names and arguments accumulated by index
   - **Finish line** (non-null `finish_reason`): held until evaluation completes
   - **Non-data lines** (empty lines, `event:` lines): forwarded immediately
3. When the upstream response is complete:
   - **No tool calls detected**: forward the finish line, close the pipe — no policy evaluation needed
   - **Tool calls detected**: evaluate all applicable policies against the tool calls (with full conversation history)
     - **No violation**: flush all buffered tool call chunks + finish line to the client
     - **Violation**: discard tool call chunks, emit a synthetic text delta with the `<system_notification>`, then a stop finish chunk and `[DONE]`

**Flow (non-streaming responses)**:
1. Read the complete JSON response
2. Parse `choices[0].message.tool_calls`
3. If no tool calls: pass through unchanged
4. If tool calls exist: evaluate policies
   - No violation: pass through unchanged
   - Violation: remove `tool_calls` from the response, set `content` to the `<system_notification>`, set `finish_reason` to `"stop"`

### Streaming Architecture

```
                    ┌──────────────────────────────────────────┐
                    │          streamAndEvaluateToolCalls       │
                    │              (goroutine)                  │
                    │                                          │
   upstream ───────►│  text chunks ──────► io.PipeWriter ──────┼──► client
   (LLM provider)  │                                          │
                    │  tool call chunks ──► buffer             │
                    │                         │                │
                    │  on finish:             ▼                │
                    │    no tool calls? ──► forward finish     │
                    │    tool calls? ──► evaluate policies     │
                    │      pass? ──► flush buffer + finish     │
                    │      fail? ──► emit notification + stop  │
                    └──────────────────────────────────────────┘
```

**No output policies for this user**: The pipe/goroutine is not created. Response streams through the normal `Read()` path with zero overhead.

**Output policies exist, text-only response**: Text streams through immediately. No policy evaluation. No added latency.

**Output policies exist, response has tool calls**: Text streams through immediately. Tool call chunks are held back. After the response completes, policy evaluation adds latency bounded by the `llm-mini` model response time. On pass, tool calls are flushed. On violation, tool calls are suppressed and a notification is emitted.

## Replacement Message Format

### Blocked User Message (sent to LLM)

The user's original message content is replaced in the `messages` array:

```json
{
  "role": "user",
  "content": "<system_notification>This message was removed by the system for violating policies. Inform the user that you cannot complete their requested action due to a policy violation. The following explanation was generated for you to relay to the user: Company policy restricts all travel bookings to economy class. The user's request to upgrade to first class cannot be fulfilled.</system_notification>"
}
```

The LLM then responds naturally, informing the user about the policy violation.

### Blocked Tool Calls (sent to LLM)

The LLM's tool calls are stripped and replaced with a text message containing an LLM-facing notification. The agentic loop ends because there are no tool calls to execute.

```json
{
  "role": "assistant",
  "content": "<system_notification>Your tool call(s) were blocked by the system for violating policies. Inform the user that you cannot complete their requested action due to a policy violation. The following explanation was generated for you to relay to the user: The assistant attempted to book a first-class flight, which violates the economy-only travel policy.</system_notification>"
}
```

## UI Treatment (Nanobot Components)

### Detection

The nanobot message rendering components detect policy violation messages by checking for the `<system_notification>` tag in message content. A utility function is added:

```typescript
// ui/user/src/lib/services/nanobot/utils.ts

export function isPolicyViolation(content: string): boolean {
  return content.includes('<system_notification>') && content.includes('violating');
}

export function extractPolicyExplanation(content: string): string {
  // Extract human-readable explanation from the system_notification
}
```

### Rendering

A new component `MessageItemPolicyViolation.svelte` renders blocked messages with a distinct visual treatment:

- **Icon**: `ShieldAlert` from Lucide Svelte (consistent with the security/policy concept)
- **Styling**: `alert alert-warning` (DaisyUI) — yellow/amber to distinguish from errors (red) and cancellations (gray)
- **Content**: The extracted explanation text, without the raw `<system_notification>` tags
- **Label**: "Policy Violation" badge

**For blocked user messages**: The UI shows only the LLM's response, which will naturally explain the violation.

**For blocked tool calls**: The policy violation alert replaces the normal assistant message bubble. The user sees the explanation directly.

**Visual mockup**:
```
┌─────────────────────────────────────────┐
│ ⚠ Policy Violation                      │
│                                         │
│ The assistant attempted to book a       │
│ first-class flight, which violates the  │
│ economy-only travel policy.             │
└─────────────────────────────────────────┘
```

### Integration Point

In `MessageItem.svelte`, before routing to the standard text renderer, check if the content is a policy violation and route to `MessageItemPolicyViolation.svelte` instead.

## File Inventory

### New Files

| File | Purpose |
|------|---------|
| `apiclient/types/messagepolicy.go` | MessagePolicyManifest type, validation, PolicyDirection |
| `pkg/storage/apis/obot.obot.ai/v1/messagepolicy.go` | CRD storage type |
| `pkg/api/handlers/messagepolicy.go` | CRUD API handler |
| `pkg/messagepolicy/helper.go` | Policy evaluation engine (indexers, LLM judge, parallel eval) |
| `ui/user/src/lib/components/nanobot/MessageItemPolicyViolation.svelte` | Policy violation UI component |

### Modified Files

| File | Change |
|------|--------|
| `pkg/gateway/server/llmproxy.go` | Add input policy evaluation hook; pipe-based streaming tool call evaluation |
| `pkg/gateway/server/router.go` | Register message-policy API routes |
| `pkg/services/config.go` | Wire up MessagePolicy handler and helper |
| `pkg/api/router.go` | Register CRUD routes for `/api/message-policies` |
| `ui/user/src/lib/services/nanobot/utils.ts` | Add `isPolicyViolation` / `extractPolicyExplanation` |
| `ui/user/src/lib/components/nanobot/MessageItem.svelte` | Route policy violations to new component |

## Open Questions / Future Work

1. **Audit logging**: A `MessagePolicyViolation` resource or event log could be added later for admin visibility into how often policies trigger and for which users.
2. **Policy testing**: An admin "test" endpoint that evaluates a sample message against a policy without affecting real traffic.
3. **Caching**: For high-traffic deployments, caching Stage 1 results for identical messages could reduce LLM calls.
4. **Policy priority/ordering**: If multiple policies are violated, the current design reports all violations. A priority system could surface the most relevant one.
5. **Granular tool call policies**: Policies that target specific tool names or MCP servers rather than general content.
