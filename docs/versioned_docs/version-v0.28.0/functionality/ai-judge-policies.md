---
displayed_sidebar: sidebar
title: AI Judge Policies
---

Message policies, later called AI judge policies, are now part of [Guardrails](./guardrails.md). A message policy is an **LLM Judge Policy** guardrail used by a blocking guardrail policy, and guardrails can apply it to MCP tool calls as well as to LLM traffic.

When Obot starts, it converts each existing message policy into guardrails, once:

| Message policy | Guardrail |
| --- | --- |
| Display name and definition | An **LLM Judge Policy** guardrail with the same name and policy |
| Applies to *User Messages* | A guardrail policy that blocks on **input** |
| Applies to *Tool Calls* | A guardrail policy that blocks on **output** |
| Applies to *Both* | A guardrail policy that blocks on input and output |
| Subjects (users, groups, Obot groups, all users) | The guardrail policy's [profile](./model-access-policies.md): the profile for everyone if the policy applied to all users, otherwise a new profile named `<policy name> (message policy)` with its subjects and no models |

The converted guardrail policies start **disabled**. To enable one, open its profile under **LLM Gateway** → **Profiles**, click the converted applied guardrail, turn on **Enabled** at the top of the dialog, click **Apply**, then **Save** the profile. A profile converted from a policy that applied to no one needs users or groups before it can be saved. Once enabled, they fail closed, as message policies did: when the judge's model is unavailable, the request is blocked.

The original MessagePolicy resources are kept for the conversion but no longer enforced, and the message policy and violation APIs were removed. Earlier violation records stay in the database but are not carried over to the new UI or API. New findings are recorded in the [LLM audit log](./guardrails.md#events).

The `OBOT_SERVER_ENABLE_MESSAGE_POLICIES` setting is removed. Guardrails are always available.

## How LLM judge policies are evaluated

Each policy is evaluated in two stages:

1. A fast screening pass using the `Chat - Fast` default model returns only `yes` or `no`.
2. If the first pass says the content should be blocked, the full `Chat` default model confirms or overturns that result and writes the explanation.

Input policies judge the latest user message in the context of the conversation. Output policies judge the tool calls the model makes. On MCP tool calls, they judge the tool's arguments and results.

Policy evaluation consumes tokens and adds latency. The tokens don't count against users' budgets.

## Writing effective policies

- Be specific about what is disallowed or required.
- Write the rule as a natural language instruction, not as implementation notes.
- Prefer separate policies for distinct concerns instead of one broad policy.
- Use output (tool call) policies for operational constraints, such as limits on purchases, bookings or outbound actions.
- Use input policies for input restrictions, such as prohibited data sharing or disallowed requests.
