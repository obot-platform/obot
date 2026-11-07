---
title: "Control MCP access"
---

Review access in layers. A catalog entry being visible is not the same as a shared vMCP granting all of its tools.

Choose **Managed** tool setup when you need to discover, capture, and select specific tools. **As-is** passes through the upstream tool set and definitions, including future changes; do not treat it as a fixed tool allowlist.

1. [MCP Access Policies](./access.md#mcp-access-policies-overview) grant access to catalog servers.
2. A [vMCP component](./access.md#virtual-mcps-tools-and-profiles) defines the maximum exposed tool set.
3. Matching profiles grant subsets of those tools to users and groups. Grants are additive: a narrow profile does not deny a tool granted by a broader profile.
4. The user's instance can narrow tool selection further.
5. An [authorization scope](../functionality/agent-auth-scopes.md) restricts programmatic credentials within their owner's permissions.

Test both an allowed and an ungranted tool using the intended user's account. Also inspect broad profiles, including default administrator grants, when a tool remains accessible unexpectedly.

Catalog `unsupportedTools` metadata is a usability hint, not an access restriction. Use vMCP tool selection and profiles to control what clients can discover and call.

See [Roles, permissions, and policy coverage](../security/policy-coverage.md) for how MCP controls differ from model, skill, message, and device policies.

## Tools and profiles {#virtual-mcps-tools-and-profiles}

Tool controls apply in layers:

1. The component configuration defines the tools the vMCP can expose. Disabling a tool here prevents every profile and user from using it.
2. Each matching profile grants all or a subset of those component tools.
3. A user's vMCP instance may narrow its selection further, but it cannot enable a tool outside the combined profile grant.

When several components expose the same tool name, configure a component prefix or rename a tool so clients receive unique names. Refreshing tool information may require the component's fixed configuration and OAuth authentication.

## Catalog access policies {#mcp-access-policies-overview}

MCP Access Policies control which MCP servers are available to which users. Administrators use access policies to map server entries from the MCP Servers page to specific users and groups, ensuring each team has access to the tools they need.

To manage access policies, go to **MCP Servers > Access Policies** in the MCP Platform.

## Default Access {#mcp-access-policies-default-access}

The **All Obot Users** selection grants a policy's server access to anyone signed in to Obot. Review policies with this selection before adding narrower team policies: a narrower grant does not cancel an existing broad grant.

To restrict catalog access, edit or remove the broad policy and assign the intended users or groups. Catalog access controls which entries users can select for personal vMCPs. Shared vMCP consumers receive access through profiles instead.

## Creating an Access Policy {#mcp-access-policies-creating-an-access-policy}

To create a new access policy:

1. Click the **Add Access Policy** button in the MCP Access Policies section
2. Give your access policy a name
3. Assign users and groups to the access policy
4. Add the MCP servers that this access policy should include

## Example: Marketing Team Access Policy {#mcp-access-policies-example-marketing-team-access-policy}

For instance, if you were creating an access policy for a marketing team:

1. Create a new access policy named "Marketing Team"
2. Assign your marketing team members, either individually or through an existing group
3. Add relevant MCP servers such as:
   - Email tools
   - Google Calendar
   - Google Sheets
   - CRM systems
   - Other tools your marketing team needs for their day-to-day work

Review other matching policies as well: their grants combine with this policy. Use vMCP component tool selection and profiles to restrict individual tools.

## Related {#mcp-access-policies-related}

For programmatic discovery of available servers and how to contribute servers to Obot's default set, see [MCP Registry API](../functionality/mcp-registry-api.md).
