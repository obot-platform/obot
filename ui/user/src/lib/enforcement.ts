// Helpers shared by the enforcement policy editor and the decision log's
// quick-allow flow. Keeping the identity, merge, and validation rules in one
// place is what makes those two surfaces agree on what an allowlist entry means.
//
// The validation here mirrors the server's; it exists to spare an administrator a
// round trip, not to replace it. The server is always authoritative.
import type {
	AllowlistServer,
	AllowlistServerPackage,
	AllowlistServerPackageSource,
	EnforcementAllowlist,
	EnforcementDecisionEvent
} from '$lib/services/admin/types';

// Agents the device-side classifier reports. Unknown values are labeled
// generically rather than dropped, so a newer obot-sentry that reports an agent
// this build has never heard of still renders readably.
const AGENT_LABELS: Record<string, string> = {
	claude_code: 'Claude Code',
	codex: 'Codex',
	cursor: 'Cursor',
	vscode: 'VS Code'
};

// Tool kinds the device-side classifier reports. Everything except "mcp" is a
// tool built into the agent itself.
const KIND_LABELS: Record<string, string> = {
	generic: 'Generic',
	mcp: 'MCP',
	read: 'Read',
	shell: 'Shell',
	task: 'Task',
	write: 'Write'
};

// titleCase turns an unrecognized snake_case identifier into something readable.
function titleCase(value: string): string {
	return value
		.split(/[_\-\s]+/)
		.filter(Boolean)
		.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
		.join(' ');
}

export function agentLabel(agent?: string): string {
	if (!agent) return 'Unknown';
	return AGENT_LABELS[agent] ?? titleCase(agent);
}

export function kindLabel(kind?: string): string {
	if (!kind) return 'Unknown';
	return KIND_LABELS[kind] ?? titleCase(kind);
}

export const PACKAGE_SOURCE_LABELS: Record<AllowlistServerPackageSource, string> = {
	npm: 'NPM',
	pypi: 'PyPI'
};

export type AllowlistServerKind = 'url' | 'package' | 'hostname' | 'connector';

export const ALLOWLIST_SERVER_KIND_LABELS: Record<AllowlistServerKind, string> = {
	connector: 'Connector',
	hostname: 'Hostname',
	package: 'Package',
	url: 'URL'
};

// allowlistServerKind reports which single dimension an entry is built on. A
// malformed entry with none (or several) set has no kind.
export function allowlistServerKind(entry: AllowlistServer): AllowlistServerKind | undefined {
	const kinds: AllowlistServerKind[] = [];
	if (entry.url?.trim()) kinds.push('url');
	if (entry.package) kinds.push('package');
	if (entry.hostname?.trim()) kinds.push('hostname');
	if (entry.connector?.trim()) kinds.push('connector');
	return kinds.length === 1 ? kinds[0] : undefined;
}

// canonicalPackageName reduces a package name to the single spelling its registry
// considers canonical. It mirrors the server's enforcement.CanonicalPackageName.
export function canonicalPackageName(source: AllowlistServerPackageSource, name: string): string {
	const trimmed = name.trim();
	// npm names are lowercase, and the scope is part of the name.
	if (source === 'npm') return trimmed.toLowerCase();
	// PyPI, PEP 503: lowercase, then collapse each run of - _ . to a single -.
	if (source === 'pypi') return trimmed.toLowerCase().replace(/[-_.]+/g, '-');
	return trimmed;
}

export function packageLabel(pkg: AllowlistServerPackage): string {
	const base = `${pkg.source}:${pkg.name}`;
	return pkg.version ? `${base}@${pkg.version}` : base;
}

// allowlistServerLabel is the one-line identity shown in tables and dialogs.
export function allowlistServerLabel(entry: AllowlistServer): string {
	switch (allowlistServerKind(entry)) {
		case 'url':
			return entry.url!.trim();
		case 'package':
			return packageLabel(entry.package!);
		case 'hostname':
			return entry.hostname!.trim();
		case 'connector':
			return entry.connector!.trim();
		default:
			return 'Invalid entry';
	}
}

// allowlistServerKey is a stable identity used to dedupe and merge entries. It
// matches how the server compares them: hostnames and connectors
// case-insensitively, URLs exactly, and packages on source + name + version.
export function allowlistServerKey(entry: AllowlistServer): string {
	switch (allowlistServerKind(entry)) {
		case 'url':
			return `url:${entry.url!.trim()}`;
		case 'package': {
			const pkg = entry.package!;
			const name = canonicalPackageName(pkg.source, pkg.name);
			return `package:${pkg.source}:${name}:${pkg.version?.trim() ?? ''}`;
		}
		case 'hostname':
			return `hostname:${entry.hostname!.trim().toLowerCase()}`;
		case 'connector':
			return `connector:${entry.connector!.trim().toLowerCase()}`;
		default:
			return 'invalid';
	}
}

export function isAllowlistEmpty(allowlist: EnforcementAllowlist): boolean {
	return (
		!allowlist.allowEverything &&
		!allowlist.allowAllObotHostedMcpServers &&
		!allowlist.allowAllBuiltinAgentTools &&
		!allowlist.allowAllBuiltinAgentMcpServers &&
		(allowlist.servers?.length ?? 0) === 0
	);
}

// defaultAllowlist is the sensible starting policy: Obot's own hosted servers
// plus whatever ships inside the coding agent. It matches what the server seeds
// when a configuration is created with enforcement already enabled.
export function defaultAllowlist(): EnforcementAllowlist {
	return {
		allowAllBuiltinAgentMcpServers: true,
		allowAllBuiltinAgentTools: true,
		allowAllObotHostedMcpServers: true
	};
}

// normalizeAllowlist applies the same cleanup the server does, so a saved policy
// and its freshly-composed equivalent compare equal and the editor does not read
// as dirty after a successful save.
export function normalizeAllowlist(allowlist: EnforcementAllowlist): EnforcementAllowlist {
	const normalized: EnforcementAllowlist = {
		allowEverything: allowlist.allowEverything || undefined,
		allowAllObotHostedMcpServers: allowlist.allowAllObotHostedMcpServers || undefined,
		allowAllBuiltinAgentTools: allowlist.allowAllBuiltinAgentTools || undefined,
		allowAllBuiltinAgentMcpServers: allowlist.allowAllBuiltinAgentMcpServers || undefined
	};

	const servers = (allowlist.servers ?? []).map((server) => {
		const entry: AllowlistServer = {};
		if (server.url?.trim()) entry.url = server.url.trim();
		if (server.hostname?.trim()) entry.hostname = server.hostname.trim().toLowerCase();
		// A connector keeps its case: it is a display name an administrator reads
		// back, and the server matches it case-insensitively anyway.
		if (server.connector?.trim()) entry.connector = server.connector.trim();
		if (server.package) {
			entry.package = {
				source: server.package.source,
				name: canonicalPackageName(server.package.source, server.package.name),
				...(server.package.version?.trim() ? { version: server.package.version.trim() } : {})
			};
		}
		const tools = (server.tools ?? []).map((tool) => tool.trim()).filter(Boolean);
		if (tools.length > 0) entry.tools = tools;
		return entry;
	});
	if (servers.length > 0) normalized.servers = servers;

	return normalized;
}

// canonicalAllowlist is a comparison string for dirty tracking. It normalizes
// first so cosmetic differences (whitespace, hostname case, an explicit false)
// don't register as changes. Server order is preserved — reordering is a real
// edit, even though it does not change the outcome.
export function canonicalAllowlist(allowlist: EnforcementAllowlist): string {
	const normalized = normalizeAllowlist(allowlist);
	return JSON.stringify({
		allowEverything: normalized.allowEverything ?? false,
		allowAllObotHostedMcpServers: normalized.allowAllObotHostedMcpServers ?? false,
		allowAllBuiltinAgentTools: normalized.allowAllBuiltinAgentTools ?? false,
		allowAllBuiltinAgentMcpServers: normalized.allowAllBuiltinAgentMcpServers ?? false,
		servers: (normalized.servers ?? []).map((server) => ({
			key: allowlistServerKey(server),
			tools: [...(server.tools ?? [])].sort()
		}))
	});
}

// allowlistServerProblem reports why an entry would be rejected, or undefined
// when it is acceptable. It mirrors the server's validateEnforcementAllowlist.
export function allowlistServerProblem(entry: AllowlistServer): string | undefined {
	const kind = allowlistServerKind(entry);
	if (!kind) {
		return 'Choose exactly one of URL, package, hostname, or connector.';
	}

	if (kind === 'url') {
		const raw = entry.url!.trim();
		let url: URL;
		try {
			url = new URL(raw);
		} catch {
			return 'Enter a valid URL, including the scheme (https://…).';
		}
		if (url.protocol !== 'http:' && url.protocol !== 'https:') {
			return 'The URL must use the http or https scheme.';
		}
		if (!url.hostname) {
			return 'The URL must include a hostname.';
		}
		if (url.username || url.password) {
			return 'The URL must not include a username or password.';
		}
		// A bare trailing "?" parses to an empty search here but is still a forced
		// query the server rejects, so it is matched on the raw string.
		if (url.search || url.hash || raw.includes('?') || raw.includes('#')) {
			return 'The URL must not include a query string or fragment. Entries match on scheme, host, port, and path prefix.';
		}
	}

	if (kind === 'package') {
		const pkg = entry.package!;
		if (pkg.source !== 'npm' && pkg.source !== 'pypi') {
			return 'Choose a package source of NPM or PyPI.';
		}
		if (!pkg.name.trim()) {
			return 'Enter a package name.';
		}
	}

	if (kind === 'hostname') {
		// The same character set the server rejects, so a bad hostname is reported
		// inline rather than coming back as a request error.
		if (/[:/?#@\s]/.test(entry.hostname!.trim())) {
			return 'Enter a bare hostname, with no scheme, port, or path (for example gitmcp.io).';
		}
	}

	// A tools array that survives normalization empty means every name in it was
	// blank, which the server rejects outright.
	if (entry.tools && entry.tools.length > 0) {
		if (entry.tools.every((tool) => !tool.trim())) {
			return 'Remove the blank tool names, or clear the list to allow every tool on this server.';
		}
	}

	return undefined;
}

export type QuickAllowAction = 'hostname' | 'server' | 'tool';

export const QUICK_ALLOW_LABELS: Record<QuickAllowAction, string> = {
	hostname: 'Allow all MCP servers from this hostname',
	server: 'Allow all tools in this MCP server',
	tool: 'Allow this tool in this MCP server'
};

// decisionHostname is the hostname a decision can be allowlisted by, derived from
// the resolved URL when the device did not report one directly.
export function decisionHostname(event: EnforcementDecisionEvent): string | undefined {
	const hostname = event.server?.hostname?.trim();
	if (hostname) return hostname.toLowerCase();
	const url = event.server?.url?.trim();
	if (!url) return undefined;
	try {
		return new URL(url).hostname.toLowerCase() || undefined;
	} catch {
		return undefined;
	}
}

// decisionServerIdentity builds the allowlist dimension that identifies the
// decision's target server, preferring the most specific evidence available.
// Package entries deliberately omit the version so that upgrading the package
// does not re-block it.
function decisionServerIdentity(event: EnforcementDecisionEvent): AllowlistServer | undefined {
	const server = event.server;
	if (!server) return undefined;
	if (server.url?.trim()) {
		// Query strings and fragments are rejected by the server, and a recorded
		// URL may carry them, so strip everything past the path.
		try {
			const url = new URL(server.url.trim());
			return { url: `${url.origin}${url.pathname}`.replace(/\/$/, '') || url.origin };
		} catch {
			return undefined;
		}
	}
	if (server.package?.name?.trim()) {
		return { package: { source: server.package.source, name: server.package.name.trim() } };
	}
	if (server.connector?.trim()) {
		return { connector: server.connector.trim() };
	}
	return undefined;
}

// quickAllowEntry is the allowlist entry a quick-allow action would add, or
// undefined when the action cannot apply to this decision.
export function quickAllowEntry(
	event: EnforcementDecisionEvent,
	action: QuickAllowAction
): AllowlistServer | undefined {
	if (event.unresolved || event.kind !== 'mcp') return undefined;

	if (action === 'hostname') {
		const hostname = decisionHostname(event);
		return hostname ? { hostname } : undefined;
	}

	const identity = decisionServerIdentity(event);
	if (!identity) return undefined;
	if (action === 'server') return identity;

	const tool = event.tool?.trim();
	return tool ? { ...identity, tools: [tool] } : undefined;
}

// quickAllowBlockedReason explains why an action is unavailable, so the button can
// render disabled with the reason rather than silently vanishing.
export function quickAllowBlockedReason(
	event: EnforcementDecisionEvent,
	action: QuickAllowAction
): string | undefined {
	if (quickAllowEntry(event, action)) return undefined;

	if (event.unresolved) {
		return 'The device could not identify what this call targets, so there is nothing to allow. Fix the agent MCP configuration on the device, or allow the tool type instead.';
	}
	if (event.kind !== 'mcp') {
		return `This is not an MCP tool call. Use the "All built-in agent tools" rule to allow ${kindLabel(event.kind).toLowerCase()} tools.`;
	}
	if (action === 'tool' && !event.tool?.trim()) {
		return 'This call recorded no tool name.';
	}

	const identity = decisionServerIdentity(event);
	if (action === 'hostname' && identity) {
		const kind = allowlistServerKind(identity);
		return `This server has no hostname. Allow it by ${kind === 'connector' ? 'connector' : 'package'} with one of the actions below instead.`;
	}

	const command = event.server?.command?.trim();
	if (command) {
		return `This server runs a local command (${command}) and cannot be matched by URL, package, or hostname.`;
	}
	if (action === 'hostname') {
		return 'This server has no hostname.';
	}
	return 'This call has no server identity that can be allowlisted.';
}

// mergeAllowlistEntry adds an entry to an allowlist, folding it into an existing
// entry with the same identity instead of creating a duplicate.
//
// The effect it reports is what the confirm dialog shows the administrator:
//   added      — a new entry was appended
//   widened    — an entry limited to specific tools now allows every tool
//   tool-added — one tool name was added to an existing entry's list
//   no-op      — the allowlist already covers the call
export type MergeEffect = 'added' | 'widened' | 'tool-added' | 'no-op';

export function mergeAllowlistEntry(
	allowlist: EnforcementAllowlist,
	entry: AllowlistServer
): { allowlist: EnforcementAllowlist; effect: MergeEffect } {
	const servers = [...(allowlist.servers ?? [])];
	const key = allowlistServerKey(entry);
	const index = servers.findIndex((server) => allowlistServerKey(server) === key);

	if (index < 0) {
		return { allowlist: { ...allowlist, servers: [...servers, entry] }, effect: 'added' };
	}

	const existing = servers[index];
	const existingTools = existing.tools ?? [];
	const incomingTools = entry.tools ?? [];

	// The incoming entry allows every tool. That is broader than any tool list, so
	// it replaces one; against an entry that already allows everything it changes
	// nothing.
	if (incomingTools.length === 0) {
		if (existingTools.length === 0) return { allowlist, effect: 'no-op' };
		const widened = { ...existing };
		delete widened.tools;
		servers[index] = widened;
		return { allowlist: { ...allowlist, servers }, effect: 'widened' };
	}

	// The existing entry already allows every tool on this server, so naming one
	// tool would narrow nothing and add nothing.
	if (existingTools.length === 0) {
		return { allowlist, effect: 'no-op' };
	}

	const missing = incomingTools.filter((tool) => !existingTools.includes(tool));
	if (missing.length === 0) {
		return { allowlist, effect: 'no-op' };
	}

	servers[index] = { ...existing, tools: [...existingTools, ...missing] };
	return { allowlist: { ...allowlist, servers }, effect: 'tool-added' };
}
