# User Message Replacement Plan

## Problem

When a user message policy violation is detected in the LLM proxy, the message content is replaced in the request body sent to the LLM, but the original user message remains visible in the nanobot chat UI. The original text is persisted in nanobot's session state (`Execution.PopulatedRequest.Input`) before the proxy evaluates policies, and the proxy has no way to tell nanobot that the message was blocked.

## Scope

Only the `llmProviderProxy.proxy` path needs fixing (`/api/llm-proxy/openai/...` and `/api/llm-proxy/anthropic/...`). The legacy `dispatchLLMProxy` path is out of scope.

## Approach: Response Header + Nanobot Session Update

When the proxy detects a user message policy violation, it sets a custom response header on the HTTP response going back to nanobot. Nanobot's LLM clients read the header and propagate a flag through the response chain. The agent runner then replaces the last user message in the session's `Execution.PopulatedRequest.Input` before persisting.

## Changes

### Obot (this repo)

#### `pkg/gateway/server/llmproxy.go`

**1. Add field to `responseModifier`:**

```go
type responseModifier struct {
    // ... existing fields ...

    // Input policy violation: replacement text to send back via response header.
    inputPolicyReplacement string
}
```

**2. Set the field in `llmProviderProxy.proxy` when a violation is detected:**

In the `proxy` function (~line 998-1010), after building the `replacement` string and setting `bodyModified = true`, also store the replacement:

```go
if msgMap, ok := rawMessages[lastUserIdx].(map[string]any); ok {
    msgMap["content"] = replacement
    bodyModified = true
    inputPolicyReplacement = replacement  // capture for response header
}
```

Then pass it to the `responseModifier` when constructing it (~line 1058-1068):

```go
&responseModifier{
    // ... existing fields ...
    inputPolicyReplacement: inputPolicyReplacement,
}
```

**3. Set the response header in `modifyResponse`:**

At the top of `modifyResponse`, before the early return check or right after it:

```go
func (r *responseModifier) modifyResponse(resp *http.Response) error {
    if r.inputPolicyReplacement != "" {
        resp.Header.Set("X-Obot-Message-Policy-Replacement", r.inputPolicyReplacement)
    }
    // ... rest of existing logic ...
}
```

The header is set unconditionally (regardless of status code or path) because it relates to the input, not the response content. The header name `X-Obot-Message-Policy-Replacement` is descriptive and namespaced to avoid collisions.

### Nanobot

#### `pkg/types/completer.go`

**1. Add a field to `CompletionResponse`:**

```go
type CompletionResponse struct {
    // ... existing fields ...

    // InputReplacement, if set, indicates the last user message was replaced
    // by the LLM proxy due to a policy violation. The value is the replacement text.
    InputReplacement string `json:"inputReplacement,omitempty"`
}
```

#### `pkg/llm/completions/client.go`

**2. Read the header in `complete()` and propagate it:**

After `httpResp, err := http.DefaultClient.Do(httpReq)` (~line 89), capture the header value. The cleanest way is to return it alongside the `Response`:

```go
httpResp, err := http.DefaultClient.Do(httpReq)
if err != nil {
    return nil, "", err
}
defer httpResp.Body.Close()

inputReplacement := httpResp.Header.Get("X-Obot-Message-Policy-Replacement")
```

Update the `complete()` return signature to `(*Response, string, error)`. In the `Complete()` method, propagate to the `CompletionResponse`:

```go
func (c *Client) Complete(...) (*types.CompletionResponse, error) {
    // ...
    resp, inputReplacement, err := c.complete(ctx, completionRequest.Agent, req, opts...)
    if err != nil {
        return nil, err
    }
    cr, err := toResponse(resp, ts)
    if err != nil {
        return nil, err
    }
    cr.InputReplacement = inputReplacement
    return cr, nil
}
```

#### `pkg/llm/anthropic/client.go`

**3. Same change as completions client** - read `X-Obot-Message-Policy-Replacement` header from `httpResp` and propagate through `CompletionResponse.InputReplacement`.

#### `pkg/llm/responses/client.go`

**4. Same change as completions client** - read header from `httpResp` and propagate.

#### `pkg/agents/run.go`

**5. Replace the last user message in the Execution before saving to session:**

In the `Complete()` method, after `a.run()` returns and before `session.Set()` (~line 449-454):

```go
if err := a.run(ctx, config, currentRun, previousRun, opts); err != nil {
    return nil, err
}

// If the LLM proxy replaced the user message due to a policy violation,
// update the stored input to reflect the replacement.
if currentRun.Response != nil && currentRun.Response.InputReplacement != "" && currentRun.PopulatedRequest != nil {
    replaceLastUserMessage(currentRun.PopulatedRequest, currentRun.Response.InputReplacement)
}

if isChat {
    session.Set(previousExecutionKey, currentRun)
}
```

The `replaceLastUserMessage` helper finds the last message with `Role == "user"` in `PopulatedRequest.Input` and replaces its text content items:

```go
func replaceLastUserMessage(req *types.CompletionRequest, replacement string) {
    for i := len(req.Input) - 1; i >= 0; i-- {
        if req.Input[i].Role == "user" {
            req.Input[i].Items = []types.CompletionItem{
                {
                    Content: &mcp.Content{
                        Type: "text",
                        Text: replacement,
                    },
                },
            }
            return
        }
    }
}
```

## Data Flow

```
User sends message
  │
  ▼
Nanobot stores message in Execution.PopulatedRequest.Input
  │
  ▼
Nanobot calls LLM via /api/llm-proxy/openai/chat/completions
  │
  ▼
llmProviderProxy.proxy() evaluates input policies
  │  violation detected → replaces message in request body
  │                      stores replacement text for response header
  │
  ▼
Request proxied to upstream LLM (with replaced message)
  │
  ▼
modifyResponse() sets X-Obot-Message-Policy-Replacement header on response
  │
  ▼
Nanobot LLM client reads header, sets CompletionResponse.InputReplacement
  │
  ▼
agents.Complete() sees InputReplacement, updates PopulatedRequest.Input
  │
  ▼
session.Set() persists the updated Execution (with replaced message)
  │
  ▼
Next time UI loads chat history → GetMessages() returns replaced text
```

## What the User Sees

- **During the current conversation**: The optimistic user message is shown briefly, then replaced when the session state updates and the event stream replays.
- **On page refresh / history load**: The replaced message text is shown (the `<system_notification>` content), which the UI's `isPolicyViolation()` check detects and renders as a policy violation alert.
- **The LLM's response**: Naturally explains the policy violation since it received the replacement notification.
