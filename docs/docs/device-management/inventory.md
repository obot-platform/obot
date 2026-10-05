---
title: "Inspect device inventory"
---

## Overview {#overview}

The Overview is a dashboard that summarizes the submitted scans for the selected time range. Use the date range filter to change the window.

It helps administrators answer questions like:

- How many devices are reporting scan data?
- Which users are submitting scans?
- Which local AI clients are most common?
- Which MCP servers and skills appear most often?
- Is scan submission activity increasing or dropping off?

Its inventory totals use the latest scan for each device in the selected window. The scan submission timeline counts every scan submitted in the window.


## Devices {#devices-1}

The Devices view lists scanned devices using each device's latest scan.

From this view, administrators can:

- Search by device ID or submitting user.
- See operating system and architecture.
- See the user who submitted the latest scan.
- Compare counts for MCP servers, skills, plugins, and clients observed on each device.
- Open a device detail page.

The device detail page shows metadata from the latest scan and separates the latest inventory into MCP servers, skills, plugins, and clients.

It also shows latest inventory tabs for:

- MCP Servers
- Skills
- Plugins
- Clients

Each tab supports drilldown into the specific observed item. The page also includes scan history, so administrators can open an older scan and inspect what was reported at that time.


## MCP Server inventory {#mcp-server-inventory}

The Device MCP Servers view helps administrators understand which MCP servers are configured on user workstations and where they appear.

From this view, administrators can:

- Search by server name.
- Sort and filter the fleet-wide server inventory.
- Open an MCP server detail page.

The detail page shows the server configuration summary and the devices where that server was observed.

Occurrences link back to the device scan item that reported the server. This makes it possible to move from a fleet-wide MCP server view to the device and scan where it was observed.


## Skill inventory {#skill-inventory}

The Device Skills view groups observed skills by skill name.

From this view, administrators can:

- Search by skill name.
- Sort the observed skills inventory.
- Open a skill detail page.

The skill detail page shows a skill summary and the devices where that skill was observed.

Occurrences link back to the device scan item that reported the skill.


## Client inventory {#client-inventory}

The Device Clients view groups observed AI clients by client name.

From this view, administrators can:

- Search by client name.
- Sort by client name, MCP server count, skill count, or user count.
- Open a client detail page.

The client detail page shows the users whose latest device scans include that client, along with the MCP servers and skills associated with that client.


## Scan item details {#scan-item-details}

Opening an item from a device or scan page shows the scan-scoped details for that observation.

MCP server details include the client and scope where the server was found, its endpoint or command, related configuration file, and a reconstructed configuration snippet. Secret values are not shown; keys that were present are rendered with placeholder values.

Skill details include the client and scope where the skill was found, description, source information when available, parent plugin when applicable, and supporting files. If a collected file includes content, the UI displays that content.

Plugin details include the client and scope where the plugin was found, plugin metadata, enabled state, detected capabilities, and supporting files.


## Supported local clients {#supported-local-clients}

Device scans detect and inventory these local AI clients:

| Client | What scan can collect |
|--------|-----------------------|
| Claude Code | Client presence, MCP servers, skills, plugins |
| Claude Desktop | Client presence, MCP servers, skills, plugins |
| Codex | Client presence, MCP servers, skills, plugins |
| Cursor | Client presence, MCP servers, skills, plugins |
| Goose | Client presence, MCP servers |
| Hermes | Client presence, MCP servers, skills |
| OpenClaw | Client presence |
| OpenCode | Client presence, MCP servers, skills, plugins |
| VS Code | Client presence, MCP servers |
| Windsurf | Client presence, MCP servers, skills |
| Zed | Client presence, MCP servers |

Project-scoped configuration is found by walking the user's home directory. Global client configuration is read directly from each client's known config location. Skills are detected from known skill directories, nested plugin skills, and `SKILL.md` files found during the project crawl.


## What scans include {#what-scans-include}

Submitted scans include:

- Device metadata, such as hostname, OS, architecture, OS username, scanner version, scan time, and stable device identity.
- Detected AI clients and their local install or configuration paths when available.
- MCP server observations, including where the server was found and the command or URL used to launch it.
- Skill observations, including skill metadata, related files, script presence, and source information when available.
- Plugin observations, including plugin metadata, enabled state, related files, and detected capabilities.
- Captured config or manifest files.

MCP server environment variable values and HTTP header values are not part of the structured MCP server observations. Only their key names are recorded.

:::caution
Captured config and manifest file content may contain whatever is present in those files. Files larger than 1 MiB are recorded as oversized and their content is not included.
:::


## Access and permissions {#access-and-permissions}

Any authenticated user can submit a device scan.

Reading submitted scan data is limited to users with administrative, owner, or auditor access. In the Obot UI, submitted scans appear in the admin Device Management section. Repeated submissions from the same workstation are grouped as scans from the same device.

Admins and owners can delete an individual device scan from the scan detail page.

Creating the device configuration, changing agent or enforcement settings, and managing enrollment keys require administrative or owner access. Auditors can view the Configuration and Enforcement Decisions views but cannot make changes.


## Troubleshooting {#troubleshooting}

### Server submission fails {#server-submission-fails}

Check that the Obot server is reachable from the workstation and that the API key has permission to submit device scans.

### A tool call is unexpectedly blocked {#a-tool-call-is-unexpectedly-blocked}

Open **Enforcement Decisions**, select the blocked call, and review its reason and resolved target. Confirm that an allow rule covers the reported server identity and tool. If the call does not appear, or if Obot Sentry could not identify the server or reach Obot, consult the `INSTRUCTIONS.md` included in the device configuration download for platform-specific diagnostics.
