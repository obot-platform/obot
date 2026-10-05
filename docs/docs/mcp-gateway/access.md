---
title: "Control MCP access"
---

## Tools and profiles {#virtual-mcps-tools-and-profiles}

Tool controls apply in layers:

1. The component configuration defines the tools the vMCP can expose. Disabling a tool here prevents every profile and user from using it.
2. Each matching profile grants all or a subset of those component tools.
3. A user's vMCP instance may narrow its selection further, but it cannot enable a tool outside the combined profile grant.

When several components expose the same tool name, configure a component prefix or rename a tool so clients receive unique names. Refreshing tool information may require the component's fixed configuration and OAuth authentication.


## Overview {#mcp-access-policies-overview}

MCP Access Policies control which MCP servers are available to which users. Administrators use access policies to map server entries from the MCP Servers page to specific users and groups, ensuring each team has access to the tools they need.

To manage access policies, go to **MCP Management > MCP Access Policies** in the MCP Platform.


## Default Access {#mcp-access-policies-default-access}

By default, there's an "everyone" group that's assigned to all users. This means anyone that logs into Obot will have access to all MCP servers that are covered by an access policy that includes the "everyone" group.

If this default behavior is not what you want, you can restrict access to specific users or groups, or remove the "everyone" group entirely. However, it's recommended that administrators at least should have access to all servers.


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

This approach ensures that each team only has access to the tools they need while maintaining security and organization.


## Related {#mcp-access-policies-related}

For programmatic discovery of available servers and how to contribute servers to Obot's default set, see [MCP Registry API](../functionality/mcp-registry-api.md).
