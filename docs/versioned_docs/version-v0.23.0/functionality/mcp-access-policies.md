---
title: MCP Access Policies
---

## Overview

MCP Access Policies control which MCP servers are available to which users. Administrators use access policies to map server entries from the MCP Servers page to specific users and groups, ensuring each team has access to the tools they need.

To manage access policies, go to **MCP Management > MCP Access Policies** in the MCP Platform.

## Default Access

By default, there's an "everyone" group that's assigned to all users. This means anyone that logs into Obot will have access to all MCP servers that are covered by an access policy that includes the "everyone" group.

If this default behavior is not what you want, you can restrict access to specific users or groups, or remove the "everyone" group entirely. However, it's recommended that administrators at least should have access to all servers.

## Creating an Access Policy

To create a new access policy:

1. Click the **Add Access Policy** button in the MCP Access Policies section
2. Give your access policy a name
3. Assign users and groups to the access policy
4. Add the MCP servers that this access policy should include

## Example: Marketing Team Access Policy

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

## Related

For programmatic discovery of available servers and how to contribute servers to Obot's default set, see [MCP Registry API](./mcp-registry-api.md).
