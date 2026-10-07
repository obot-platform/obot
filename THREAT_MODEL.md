# Threat Model

This document explains what we treat as a security vulnerability in Obot and what we don't. Read it before you report an issue. Reports about behavior this document describes as expected will be closed. If you aren't sure whether an issue is in scope, report it privately.

For how Obot's security controls work, see the [security overview](docs/docs/security/model.md) in the documentation.

## Who we trust

**Admins and Owners** are fully trusted. Issues where the attacker must already have one of these roles are out of scope. Issues where an Admin or Owner is the victim, such as a malicious server abusing an admin's connection, are not excluded by this rule.

**Power Users and Power User+** are privileged roles. They can deploy MCP servers that run their own code, through npx, uvx, or container images, and they can configure the URLs Obot connects to. Issues where the attacker must already have one of these roles are out of scope, unless they let that user:

- read or change another user's data,
- gain Admin or Owner access, or
- reach the host or Obot's own internal services in a way their deployed code cannot, or
- meet one of the in-scope SSRF conditions below.

**Standard users** are not trusted with each other's data. Issues that let a standard user read or change another user's data, use a server or resource they were not granted, or gain a higher role are in scope.

**Unauthenticated users** are not trusted. Any issue an unauthenticated user can exploit is in scope.

**Remote MCP servers** are not trusted. Issues that let a remote MCP server obtain credentials for another service, another user, or Obot itself are in scope.

## Server-side request forgery (SSRF) and outbound connections

Obot connects to URLs as part of its normal work: remote MCP servers, OAuth endpoints, registry icons, and more. Admins and Power Users choose many of these URLs on purpose.

Obot's egress guard blocks connections to loopback, private, and link-local addresses for some of these requests, such as connections to remote MCP servers and OAuth discovery. It does not cover every outbound request Obot makes. The guard is defense in depth. It is not a security boundary against users who can already configure URLs or run code. Restricting the Obot server's outbound network access is the operator's job. See [Restricting Obot Server Egress](docs/docs/installation/kubernetes-deployment.md#restricting-obot-server-egress).

An SSRF issue is **in scope** if it meets any of these conditions, even if it also needs a Power User role:
- an unauthenticated user can trigger it,
- it returns the response body to the attacker, or
- it sends Obot's own credentials or tokens to an attacker, or
- it changes data or state on an internal service, or otherwise gives the attacker access they did not have.

If it meets none of them, it is **out of scope as an advisory**. That covers blind requests that only show whether a host or port is reachable, and ways around the egress guard with no further impact, when they need an Admin or Power User role. Report these as regular bugs or send a pull request.

Bypasses that only work with non-default network setups, such as NAT64 or DNS64 gateways, are hardening, not vulnerabilities.

## MCP server isolation

The **Docker** backend runs MCP servers as sibling containers through the host's Docker socket. It is for development and trusted, single-tenant use, and it is not an isolation boundary. Issues that only affect isolation between MCP servers, or between an MCP server and the host, on the Docker backend are out of scope.

The **Kubernetes** backend, with the restricted Pod Security policy and NetworkPolicy, is the supported configuration for multi-tenant use. Container or network isolation issues in that configuration are in scope.

See [Network and workload isolation](docs/docs/security/isolation.md).

## Device management (Obot Sentry)

Device enforcement is experimental, and it depends on the device cooperating. It is not a security boundary against the device's own user. Ways for a user to get around enforcement on a device they control are out of scope.

## Identity provider groups

For most identity providers, Obot caches each user's group memberships and refreshes them from the provider about every 10 minutes, so a change made in the provider can take that long to apply in Obot. For providers that sync through SCIM, Obot doesn't refresh memberships itself. They change when the provider sends a SCIM update, so the timing depends on the provider. Both delays are expected. To remove access immediately, change the user's access in Obot directly.

## URL templates

Values in a remote MCP server's URL template, including static values, become part of the server's URL. URLs are not secret. They can appear in logs and browser history, and users of the server may be able to see them. Put secrets in HTTP headers, not in URL templates.

## Other things that are out of scope

- Findings with no demonstrated security impact, such as missing security headers, version numbers in responses, or best-practice suggestions.
- Issues in third-party dependencies. Report them to the dependency's maintainers.
- Issues that only affect deprecated or end-of-life Obot versions.
- Social engineering, and attacks that need physical access to a user's device.
