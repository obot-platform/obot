---
title: Device Management
---

Device management gives administrators visibility into the AI clients, MCP servers, skills, and plugins configured on user workstations.

Device data is collected by the `obot scan` CLI command and submitted to Obot. Each submitted scan is stored as a point-in-time inventory for that device.

## What it does

Device management helps administrators:

1. Monitor device scan coverage across the organization.
2. Review the latest inventory submitted by each device.
3. See which AI clients, MCP servers, skills, and plugins are present on scanned devices.
4. Drill into where a specific MCP server or skill appears across devices.
5. Inspect scan history and compare previous submissions from a device.
6. Review captured config and manifest files for a specific scan item.

:::note
Device management is inventory and visibility. It does not install clients, change local client configuration, start MCP servers, or grant access to Obot resources.
:::

## Device Management views

The Obot admin UI includes a Device Management section with these views:

| View | What it shows |
|------|---------------|
| Dashboard | Organization-level scan coverage, top observed clients, MCP servers, and skills, and scan submission activity over time. |
| Devices | Workstations that have submitted scans, with high-level inventory counts and links to device history. |
| Device Skills | Skills observed across scanned devices, with drilldowns into where each skill appears. |
| Device MCP Servers | MCP servers observed across scanned devices, with drilldowns into affected devices and client configurations. |
| Device Clients | AI clients observed across scanned devices, with drilldowns into associated users, MCP servers, and skills. |

The Dashboard supports a date range filter. The device, skill, MCP server, and client list views support search, sorting, and filtering where available.

## Device dashboard

The Dashboard summarizes submitted scans for the selected time range.

It helps administrators answer questions like:

- How many devices are reporting scan data?
- Which users are submitting scans?
- Which local AI clients are most common?
- Which MCP servers and skills appear most often?
- Is scan submission activity increasing or dropping off?

Dashboard inventory totals use the latest scan for each device in the selected window. The scan submission timeline counts every scan submitted in the window.

## Devices

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

## MCP Server inventory

The Device MCP Servers view helps administrators understand which MCP servers are configured on user workstations and where they appear.

From this view, administrators can:

- Search by server name.
- Sort and filter the fleet-wide server inventory.
- Open an MCP server detail page.

The detail page shows the server configuration summary and the devices where that server was observed.

Occurrences link back to the device scan item that reported the server. This makes it possible to move from a fleet-wide MCP server view to the device and scan where it was observed.

## Skill inventory

The Device Skills view groups observed skills by skill name.

From this view, administrators can:

- Search by skill name.
- Sort the observed skills inventory.
- Open a skill detail page.

The skill detail page shows a skill summary and the devices where that skill was observed.

Occurrences link back to the device scan item that reported the skill.

## Client inventory

The Device Clients view groups observed AI clients by client name.

From this view, administrators can:

- Search by client name.
- Sort by client name, MCP server count, skill count, or user count.
- Open a client detail page.

The client detail page shows the users whose latest device scans include that client, along with the MCP servers and skills associated with that client.

## Scan item details

Opening an item from a device or scan page shows the scan-scoped details for that observation.

MCP server details include the client and scope where the server was found, its endpoint or command, related configuration file, and a reconstructed configuration snippet. Secret values are not shown; keys that were present are rendered with placeholder values.

Skill details include the client and scope where the skill was found, description, source information when available, parent plugin when applicable, and supporting files. If a collected file includes content, the UI displays that content.

Plugin details include the client and scope where the plugin was found, plugin metadata, enabled state, detected capabilities, and supporting files.

## Submitting device scans

Use `obot scan` from a workstation to collect local AI client inventory.

Run a local scan and print a summary table:

```bash
obot scan
```

Print the full scan manifest as JSON:

```bash
obot scan --json
```

Log in to the Obot server that should receive scan results:

```bash
obot login --url https://obot.example.com
```

Then submit the scan:

```bash
obot scan --submit
```

When `--submit` is used, the CLI prints the local scan result first. After the upload succeeds, it prints the submitted scan ID and server receipt time to stderr.

For unattended jobs, set `OBOT_TOKEN` instead of using browser-based login:

```bash
OBOT_TOKEN=<token> \
obot scan --submit
```

## Supported local clients

`obot scan` detects and inventories these local AI clients:

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

## Choosing scan scope

Use `--max-depth` to control how deep the project crawl descends below the user's home directory:

```bash
obot scan --max-depth 3
```

The default is `5`. A smaller value finishes faster but may miss deeply nested project configuration. A larger value searches more of the home directory.

Use `--timeout` to limit how long the scan may run:

```bash
obot scan --timeout 30
```

The default timeout is `60` seconds. If the timeout expires, the command stops and returns an error.

You can also enable submission and tune scan behavior through environment variables:

| Environment variable | Description |
|----------------------|-------------|
| `OBOT_SCAN_SUBMIT` | Submit the scan to Obot when set to `true`. |
| `OBOT_SCAN_TIMEOUT` | Number of seconds to wait for the scan to complete. |
| `OBOT_SCAN_MAX_DEPTH` | Maximum project crawl depth below the user's home directory. |

## What scans include

Submitted scans include:

- Device metadata, such as hostname, OS, architecture, OS username, scanner version, scan time, and stable device identity.
- Detected AI clients and their local install or configuration paths when available.
- MCP server observations, including where the server was found and the command or URL used to launch it.
- Skill observations, including skill metadata, related files, script presence, and source information when available.
- Plugin observations, including plugin metadata, enabled state, related files, and detected capabilities.
- Captured config or manifest files.

MCP server environment variable values and HTTP header values are not copied into the structured MCP server observations. The scanner records only their key names.

:::caution
Captured config and manifest file content may contain whatever is present in those files. Files larger than 1 MiB are recorded as oversized and their content is not included.
:::

## Access and permissions

Any authenticated user can submit a device scan.

Reading submitted scan data is limited to users with administrative, owner, or auditor access. In the Obot UI, submitted scans appear in the admin Device Management section. Repeated submissions from the same workstation are grouped as scans from the same device.

Admins and owners can delete an individual device scan from the scan detail page.

## Troubleshooting

### Authentication opens a browser

If `OBOT_TOKEN` is not set and no valid token is stored locally, `obot scan --submit` may use the browser-based login flow. For unattended jobs, set `OBOT_TOKEN`.

### Server submission fails

Check that the configured Obot server is reachable from the workstation and that the token has permission to submit device scans.

### The scan misses project configuration

Increase `--max-depth` so the project crawl reaches deeper directories under the user's home directory.

### The scan takes too long

Lower `--max-depth` or `--timeout`. The project crawl skips common dependency, build, cache, and trash directories, but very large home directories can still take longer to scan.
