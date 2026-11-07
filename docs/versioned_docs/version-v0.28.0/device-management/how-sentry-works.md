---
title: "Device Management (Sentry) overview"
---

Device management gives administrators visibility into the AI clients, MCP servers, skills, and plugins configured on user workstations. It can also audit and control tool calls made by supported local AI clients.

Device management is a **beta feature**.


## What it does {#what-it-does}

Device management helps administrators:

1. Install and configure [Obot Sentry](https://github.com/obot-platform/obot-sentry) on user workstations to enable device scanning, local agent audit logs, and optional tool call enforcement.
2. Monitor device scan coverage across the organization.
3. Review each device's latest inventory of AI clients, MCP servers, skills, and plugins.
4. Drill into where a specific MCP server or skill appears across devices.
5. Inspect scan history and compare previous submissions from a device.
6. Review captured config and manifest files for a specific scan item.
7. Define which tool calls supported local AI clients may run and review the resulting enforcement decisions.


## Devices {#devices}

Under Device Management in the Obot Administration section, Devices contains the following views:

| View | What it shows |
|------|---------------|
| Configuration | Obot Sentry agent settings, install downloads, and enrollment keys. |
| Overview | Organization-level scan coverage, top observed clients, MCP servers, and skills, and scan submission activity over time. |
| Devices | Workstations that have submitted scans, with high-level inventory counts and links to device history. |
| Device Skills | Skills observed across scanned devices, with drilldowns into where each skill appears. |
| Device MCP Servers | MCP servers observed across scanned devices, with drilldowns into affected devices and client configurations. |
| Device Clients | AI clients observed across scanned devices, with drilldowns into associated users, MCP servers, and skills. |

The **Enforcement Decisions** view under Device Management shows the tool calls that Obot Sentry allowed or blocked.
