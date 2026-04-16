---
title: Message Policies
---

## Overview

Message Policies let administrators enforce content rules written in natural language on Obot Agent traffic at the LLM proxy layer. A policy can apply to:

- **User messages** before they are sent to the model
- **Tool calls** before the agent is allowed to execute them
- **Both** user messages and tool calls

Message Policies are an **experimental feature** and are disabled by default.
To enable them, set `OBOT_SERVER_ENABLE_MESSAGE_POLICIES=true` and restart Obot.

When enabled, Obot adds **Message Policies** and **Message Policy Violations** under **Obot Agent Management** in the admin UI.

## How Policies Work

Each policy defines four things:

- **Display name** - The label shown in the UI and violation logs
- **Definition** - A natural language rule describing what is or is not allowed
- **Applies to** - `User Messages`, `Tool Calls`, or `Both`
- **Subjects** - The users or groups the policy applies to

### Subjects

A policy can apply to:

- **Individual users**
- **Groups** from the authentication provider
- **All Obot Users**

If a user matches multiple policies, Obot evaluates all of them in parallel.
A message or tool call is allowed only if it passes every applicable policy.

## Enforcement Flow

### Subject Matching

Obot determines applicable policies from:

- The authenticated user ID
- The user's authentication-provider groups
- Any wildcard policy for all users

Policies marked `Both` apply to both user-message checks and tool-call checks.

### Two-Stage Review

Obot evaluates each applicable policy in two stages:

1. A fast screening pass using the `Chat - Fast` default language model returns only `yes` or `no`.
2. If the first pass says the content should be blocked, a second review uses the full `Chat` default language model to confirm or overturn that result and produce the user-facing explanation.

If policy evaluation cannot run correctly, Obot fails closed and blocks the content.

## User Message Policies

For user-message enforcement, Obot checks only the **most recent text message** in the request, and only when it is the final message in the conversation payload. This avoids re-evaluating an already-approved user prompt during tool-calling continuations.

If one or more policies are violated:

- The violation is logged
- The upstream model receives a fixed refusal instruction instead of the original user message
- The UI receives a policy-violation notice containing the explanation returned by policy review

## Tool Call Policies

For tool-call enforcement, Obot evaluates the tool calls produced by the model rather than normal assistant text.

Behavior differs slightly by response type, but the effective result is the same:

- Assistant text can continue streaming normally
- Tool call data is buffered and evaluated before execution
- If the tool call violates a policy, Obot signals to the Obot Agent that the tool call cannot be executed
- The violation is logged with the blocked tool call payload

Obot preserves the tool-call events in the response so conversation state remains valid, but execution is prevented.

## Violation Logging

Every confirmed violation is stored as a **Message Policy Violation** record. Each record includes:

- Time of the violation
- User ID
- Policy ID and policy name
- Policy definition
- Direction (`user-message` or `tool-calls`)
- User-facing explanation
- Blocked content
- Project ID and thread ID when available

Blocked-content payloads are encrypted at rest when Obot encryption is configured; otherwise they may be stored unencrypted. See the Obot encryption configuration documentation for setup details.

## Reviewing Violations

When the feature is enabled, administrators can open **Obot Agent Management > Message Policy Violations** to review enforcement activity.

The violations view includes:

- Aggregate counts
- Breakdown by direction
- Timeline charts grouped by policy or user
- Filters for user, policy, direction, project, thread, and time range

### Visibility of Blocked Content

Admins, Owners, and Auditors can view violation records.

Only users with the **Auditor** role can see the stored blocked content in the violation detail view. Other admins can see the metadata and explanation, but not the blocked payload itself.

## Managing Policies

To manage policies, go to **Obot Agent Management > Message Policies**.

### Creating a Policy

1. Click **Add New Policy**
2. Enter a descriptive name
3. Write the policy definition in natural language. Be as specific as you can about specific actions that are or are not allowed.
4. Choose whether it applies to user messages, tool calls, or both
5. Add the users or groups the policy should cover
6. Save the policy

### Editing a Policy

Click a policy in the list to update its definition, direction, or subjects. Changes take effect immediately.

### Deleting a Policy

Deleting a policy removes that enforcement rule immediately for the affected users and groups.

## Writing Effective Policies

- Be specific about what is disallowed or required
- Write the rule as a natural language instruction, not as implementation notes
- Prefer separate policies for distinct concerns instead of one broad policy
- Use tool-call policies for operational constraints such as purchase, booking, or outbound-action limits
- Use user-message policies for input restrictions such as prohibited data sharing or disallowed requests

## Tokens and Latency

Each additional policy added to the system will increase the overall token usage, due to tokens spent during policy evaluation.
The tokens consumed do not count against the user's token usage.

Adding policies also increases latency between request and response when chatting with the Obot Agent, but since they are executed in parallel,
latency will not scale as much as token usage will when more than one policy is evaluated.

## Related Topics

- [Obot Agent Management](../obot-agent-management/) - Overview of the admin area where Message Policies appear
- [Model Providers](/configuration/model-providers) - Configure the default `llm` and `llm-mini` aliases used for policy evaluation
- [Obot Configuration Reference](/configuration/server-configuration) - Enable the feature with server configuration
- [User Roles](/configuration/user-roles) - Understand Admin, Owner, and Auditor permissions
