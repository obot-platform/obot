---
title: Device Management
---

Device management gives administrators visibility into the AI clients, MCP servers, skills, and plugins configured on user workstations.

Device management is a **beta feature**.

## What it does

Device management helps administrators:

1. Install and configure [Obot Sentry](https://github.com/obot-platform/obot-sentry) on user workstations to enable device scanning and local agent audit logs.
2. Monitor device scan coverage across the organization.
3. Review each device's latest inventory of AI clients, MCP servers, skills, and plugins.
4. Drill into where a specific MCP server or skill appears across devices.
5. Inspect scan history and compare previous submissions from a device.
6. Review captured config and manifest files for a specific scan item.

## Devices

Under Device Management in the Obot Administration section, Devices contains the following views:

| View | What it shows |
|------|---------------|
| Configuration | Obot Sentry agent settings, install downloads, and enrollment keys. |
| Overview | Organization-level scan coverage, top observed clients, MCP servers, and skills, and scan submission activity over time. |
| Devices | Workstations that have submitted scans, with high-level inventory counts and links to device history. |
| Device Skills | Skills observed across scanned devices, with drilldowns into where each skill appears. |
| Device MCP Servers | MCP servers observed across scanned devices, with drilldowns into affected devices and client configurations. |
| Device Clients | AI clients observed across scanned devices, with drilldowns into associated users, MCP servers, and skills. |

## Configuration

The Configuration view sets up [Obot Sentry](https://github.com/obot-platform/obot-sentry), a lightweight agent that enrolls workstations with Obot and enables device scanning and local agent audit logs on enrolled devices.

On first visit, select **Get Started** to create the device configuration, then follow the numbered install guide:

1. **Generate an enrollment key.** New devices enroll with your Obot server using this key. The credential is revealed once, when the key is created.
2. **Select your installation method.** How Obot Sentry is delivered to your devices.
3. **Select an operating system.** The operating system your devices run.
4. **Download the install artifacts.** A ZIP package for the selected installation method and operating system.
5. **Follow the install instructions.** The steps match your selections and show where to put the enrollment key.

Once Obot Sentry is installed and enrolled, devices begin reporting to Obot and appear in the other Device Management views.

### Installation methods

Installation methods and operating systems vary by Obot Sentry release. The current release supports:

| Installation method | Operating system | What the download contains |
|---------------------|------------------|----------------------------|
| Do it Yourself | Windows | An MSI that sets up automatic scanning and audit hook maintenance, and handles upgrades and uninstalls. A standalone `obot-sentry.exe` is also included for running one-off scans or installing hooks manually. |
| Do it Yourself | macOS | A standalone `obot-sentry` binary for running scans and installing audit hooks manually. |
| Microsoft Intune | Windows | An `.intunewin` package to deploy as a Windows app (Win32) from the Intune admin center. Assigned device groups install Obot Sentry on their next check-in, then scan and maintain audit hooks automatically. |

Each download's install instructions cover installing, configuring the enrollment key, and uninstalling for that method and operating system.

### Enrollment keys

- Keys can be named and given an expiration date. The default expiration is one year.
- Revoking a key stops new devices from enrolling with it. Already-enrolled devices are unaffected.
- New devices cannot enroll until at least one key exists.

### Settings and updates

Open the agent settings (the gear icon) to review or change how Obot Sentry behaves on your devices.

The available settings also vary by Obot Sentry release. The current release has one:

| Setting | Description | Default |
|---------|-------------|---------|
| Scan interval (minutes) | How often each signed-in user's device submits a scan. Accepts 15–1440 minutes. | 60 |

Use **Check for updates** to pick up new Obot Sentry releases. If an Update available badge appears, save the agent settings to rebuild your downloads with the new release.

## Overview

The Overview is a dashboard that summarizes the submitted scans for the selected time range. Use the date range filter to change the window.

It helps administrators answer questions like:

- How many devices are reporting scan data?
- Which users are submitting scans?
- Which local AI clients are most common?
- Which MCP servers and skills appear most often?
- Is scan submission activity increasing or dropping off?

Its inventory totals use the latest scan for each device in the selected window. The scan submission timeline counts every scan submitted in the window.

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

## Supported local clients

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

## What scans include

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

## Access and permissions

Any authenticated user can submit a device scan.

Reading submitted scan data is limited to users with administrative, owner, or auditor access. In the Obot UI, submitted scans appear in the admin Device Management section. Repeated submissions from the same workstation are grouped as scans from the same device.

Admins and owners can delete an individual device scan from the scan detail page.

Creating the device configuration, changing agent settings, and managing enrollment keys require administrative or owner access. Auditors can view the Configuration tab but cannot make changes.

## Troubleshooting

### Server submission fails

Check that the Obot server is reachable from the workstation and that the API key has permission to submit device scans.
