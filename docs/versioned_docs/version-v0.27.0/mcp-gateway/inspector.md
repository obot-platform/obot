---
title: Test vMCPs with Inspector
description: Inspect a vMCP's tools, prompts, and resources, call them directly, chat with them, and review raw MCP traffic.
---

Inspector is Obot's built-in MCP client. It connects to a vMCP as the signed-in user, so you can
list and call its tools, read its prompts and resources, test it with a chat model, and watch the
MCP messages exchanged. Use it to confirm that a vMCP works before you set up an AI client, or to
separate gateway problems from client problems.

## Open Inspector

1. Open **vMCPs** and open the vMCP.
2. Select **Inspector** in the vMCP designer. Alternatively, select **Test vMCP**, the icon beside
   **Connect** on the vMCP's card.
3. If **Start Session** appears, select it. Supply any requested configuration and authorize remote
   services if prompted.

**Test vMCP** walks you through connection setup first if you have not connected to the vMCP yet,
then opens Inspector.

Inspector is available for shared vMCPs you can connect to and for personal vMCPs you own.

The header shows the vMCP's status. Inspector opens on the **Tools** tab. The other tabs are
**Prompts**, **Resources**, **Chat**, and **MCP Log**.

### Whose access Inspector tests

Inspector connects using your own vMCP instance and your own grants. It lists only the tools your
profiles grant you, and it uses the configuration and service authorizations saved for your account.

An administrator's successful test does not show what another user can do. To check another user's
experience, have that user open Inspector under their own account. See
[Control MCP access](./access.md) for how profiles determine the tools each user receives.

## Tools

The **Tools** tab lists the tools the vMCP exposes to you. Use the search box to filter the list.

Select a tool to see its description, **Input schema**, **Output schema**, and **Raw tool
metadata**. To call it:

1. Enter arguments in the **Generated form**, which is built from the tool's input schema. To paste
   or edit the arguments as a single object, switch to **Raw JSON**.
2. Select **Call**.
3. Review the result. The call is marked **Succeeded** or **Failed**, or **Cancelled** if you stop
   it. Results show rendered content, structured content when the tool returns it, and the **Raw MCP
   response**.

Tool calls run against the real upstream service. When testing, start with read-only tools whose
effects you understand.

## Prompts

The **Prompts** tab lists the prompts the vMCP provides. Select a prompt, fill in its arguments, and
select **Get prompt** to preview the resolved messages.

To send a resolved prompt to a chat, select **Use in Chat**. Only prompts with text content up to
100 KiB can be staged.

If the vMCP's servers do not provide prompts, the tab shows **This server does not provide
prompts.**

## Resources

The **Resources** tab lists the resources the vMCP provides. Select a resource to read it and
preview its contents.

Text resources up to 100 KiB can be staged for chat with **Use in Chat**. Binary and unsupported
content can still be inspected, but cannot be staged.

## Chat

The **Chat** tab tests the vMCP with a language model. The model sees the vMCP's tools, as they were
when the chat started, and can request tool calls in response to your messages. Use **Help me test
this MCP Server** or **What tools are available from this MCP Server?** as a starting point, or
write your own message.

Chat works as follows:

- **Every tool call needs your approval.** When the model requests a tool, Inspector shows its name
  and arguments. Select **Approve \<tool\>** to run it or **Reject \<tool\>** to deny it. While an
  approved call runs, you can cancel it. Your browser makes approved calls through your Inspector
  session. The model never calls the vMCP directly.
- **Staged prompts and resources** appear above the composer. Nothing staged is sent until you send
  a message.
- **Each turn is limited to 10 model rounds.** When a turn reaches this limit, send another message,
  such as "Continue", to start a new turn.
- **Conversations are not saved.** **New Chat** clears the conversation, staged context, approvals,
  and the tool snapshot. Leaving or refreshing the page also discards the conversation.

### Which model Chat uses

- **With a model provider configured**, Chat uses the default **Language Model (Chat)** model
  through Obot's [LLM gateway](../llm-gateway/how-it-works.md). You must have access to that model
  under a [model access policy](../functionality/model-access-policies.md). Requests are subject to
  the same policies and [LLM audit logging](../llm-gateway/audit.md) as other gateway traffic. If no
  default is set, or the default is inactive or not available to you, Chat explains why it cannot
  run.
- **With no model provider configured**, Chat can use Obot's hosted model service. This requires a
  registered Obot license. Administrators without one see **Unlock MCP Inspector Chat**, which
  offers free registration or [configuring a model provider](../configuration/model-providers.md)
  instead. Other users are told to contact an administrator.

The hosted model service receives the chat conversation, including the vMCP's tool definitions and
any tool results you approve. To disable this, configure your own model provider, or turn
off **Enable Model Proxy** under **Platform > Settings > Model Proxy**. When no model provider is
configured, that section also shows usage against the service quota. To remove the service
entirely, set `OBOT_SERVER_MODEL_PROXY_URL` to an empty string. See [Server configuration](../configuration/server-configuration.md).

## MCP Log

The **MCP Log** tab records the MCP messages exchanged during the session, including calls made from
the **Tools**, **Prompts**, **Resources**, and **Chat** tabs.

- Filter entries by kind: **Requests**, **Responses**, **Notifications**, **Errors**, or
  **Transport**.
- Search by method name or request ID.
- Expand an entry to see its payload, or copy it as JSON. Large payloads are collapsed until you
  select **Render anyway**.
- **Copy All** copies the listed entries. The output may contain tool arguments, so handle it
  accordingly.

The log keeps only this session in your browser. For the persistent record, see the MCP audit logs.

## Auditing

Calls made from Inspector go through the Obot MCP Gateway, like calls from any other client. An
administrator or auditor can find them under **Operations > Audit Logs** by selecting **MCP** and
filtering on the vMCP name, method, and time. See
[Audit data and retention](../security/audit-data.md). The
[Connect AI clients](./connect-clients.md#verify) guide describes a full verification that compares
an Inspector call with the audit record.

## When Inspector cannot connect

Inspector shows one of these messages when it cannot open a session.

- **Start your vMCP to use chat and inspect tools.** You do not have a session for this vMCP yet.
  Select **Start Session**.
- **Before you can continue inspecting this vMCP, an update is required.** Your saved configuration
  is incomplete or out of date. Select **Update Configuration**.
- **\<component\> requires administrator OAuth setup before this vMCP can start.** A component needs
  an OAuth client that has not been configured. Administrators can configure it from this message.
  Other users must ask an administrator.
- **Reauthentication required**: A remote service authorization has expired or been revoked. Select
  **Manage authentication**.
- **Server setup required**: A component still needs configuration. Select **Launch** to complete
  it.
- **Server unavailable**: The deployment is not healthy, so Inspector skipped the connection. Select
  **Retry** to force an attempt.
- **Access denied**: You no longer have permission to connect to this vMCP. Check your profile
  grants.

For other connection and OAuth failures, see [Troubleshooting](./troubleshooting.md).
