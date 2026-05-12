<script lang="ts">
	import { resolve } from '$app/paths';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		type DeviceScan,
		type DeviceScanClient,
		type DeviceScanMCPServer,
		type DeviceScanPlugin,
		type DeviceScanSkill,
		type OrgUser
	} from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Boxes, Cpu, Ellipsis, MonitorCheck, PencilRuler, Scale, Server } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	type Tab = 'mcp' | 'skills' | 'plugins' | 'clients';

	const PAGE_SIZE = 50;

	let { data } = $props();
	let scans = $derived<DeviceScan[]>(data?.scans?.items ?? []);
	let deviceId = $derived(data?.deviceId ?? '');
	let latest = $derived<DeviceScan | undefined>(scans[0]);

	let activeTab = $state<Tab>('mcp');

	let submittedByUser = $state<OrgUser | undefined>();
	let submittedById = $derived(latest?.submittedBy);

	$effect(() => {
		const id = submittedById;
		if (!id) {
			submittedByUser = undefined;
			return;
		}
		AdminService.getUser(id, { dontLogErrors: true })
			.then((u) => {
				if (submittedById === id) submittedByUser = u;
			})
			.catch(() => {
				if (submittedById === id) submittedByUser = undefined;
			});
	});

	let scannedTime = $derived(
		latest ? formatTimeAgo(latest.scannedAt) : { relativeTime: '', fullDate: '' }
	);

	let mcpServers = $derived<DeviceScanMCPServer[]>(latest?.mcpServers ?? []);
	let skills = $derived<DeviceScanSkill[]>(latest?.skills ?? []);
	let plugins = $derived<DeviceScanPlugin[]>(latest?.plugins ?? []);
	let clients = $derived<DeviceScanClient[]>(latest?.clients ?? []);

	type MCPRow = DeviceScanMCPServer & {
		id: number;
		scope: string;
		endpoint: string;
	};
	type SkillRow = DeviceScanSkill & {
		id: number;
		scope: string;
		files_count: number;
	};
	type PluginRow = DeviceScanPlugin & {
		id: number;
		scope: string;
		capabilities: string;
	};
	type ClientRow = DeviceScanClient & {
		id: string;
		paths_display: string;
		has_display: string;
	};

	function deriveScope(projectPath?: string): string {
		return projectPath ? 'project' : 'global';
	}

	function formatCommand(cmd?: string, args?: string[]): string {
		if (!cmd) return '—';
		const parts = [cmd, ...(args ?? [])];
		return parts.join(' ');
	}

	function capabilitySummary(p: DeviceScanPlugin): string {
		const caps: string[] = [];
		if (p.hasMCPServers) caps.push('mcp');
		if (p.hasSkills) caps.push('skills');
		if (p.hasRules) caps.push('rules');
		if (p.hasCommands) caps.push('commands');
		if (p.hasHooks) caps.push('hooks');
		return caps.length ? caps.join(', ') : '—';
	}

	function clientHasSummary(c: DeviceScanClient): string {
		const caps: string[] = [];
		if (c.hasMCPServers) caps.push('mcp');
		if (c.hasSkills) caps.push('skills');
		if (c.hasPlugins) caps.push('plugins');
		return caps.length ? caps.join(', ') : '—';
	}

	function clientPathsSummary(c: DeviceScanClient): string {
		const parts: string[] = [];
		if (c.binaryPath) parts.push(c.binaryPath);
		if (c.installPath) parts.push(c.installPath);
		if (c.configPath) parts.push(c.configPath);
		return parts.join(', ') || '—';
	}

	function userDisplay(u: OrgUser): string {
		return u.displayName ?? u.email ?? u.username ?? u.id;
	}

	let mcpRows = $derived<MCPRow[]>(
		mcpServers.map((m) => ({
			...m,
			scope: deriveScope(m.projectPath),
			endpoint: m.transport === 'stdio' ? formatCommand(m.command, m.args) : m.url || '—'
		}))
	);

	let skillRows = $derived<SkillRow[]>(
		skills.map((s) => ({
			...s,
			scope: deriveScope(s.projectPath),
			files_count: (s.files ?? []).length
		}))
	);

	let pluginRows = $derived<PluginRow[]>(
		plugins.map((p) => ({
			...p,
			scope: deriveScope(p.projectPath),
			capabilities: capabilitySummary(p)
		}))
	);

	let clientRows = $derived<ClientRow[]>(
		clients.map((c, i) => ({
			...c,
			id: `${c.name}-${i}`,
			paths_display: clientPathsSummary(c),
			has_display: clientHasSummary(c)
		}))
	);

	type HistoryRow = {
		id: number;
		scanned_at: string;
		scanned_relative: string;
		scanner_version: string;
		mcp_count: number;
		skill_count: number;
		plugin_count: number;
		client_count: number;
		is_latest: boolean;
	};

	let historyRows = $derived<HistoryRow[]>(
		scans.map((s, i) => ({
			id: s.id,
			scanned_at: s.scannedAt,
			scanned_relative: formatTimeAgo(s.scannedAt).relativeTime,
			scanner_version: s.scannerVersion || '—',
			mcp_count: s.mcpServers?.length ?? 0,
			skill_count: s.skills?.length ?? 0,
			plugin_count: s.plugins?.length ?? 0,
			client_count: s.clients?.length ?? 0,
			is_latest: i === 0
		}))
	);

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Device {deviceId.slice(0, 12)}</title>
</svelte:head>

<Layout
	title="Device"
	showBackButton
	onBackButtonClick={() => {
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto(resolve('/admin/devices'));
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !latest}
			<p class="text-on-surface1 text-sm font-light">No scans found for this device.</p>
		{:else}
			<!-- Header card -->
			<div class="dark:bg-surface2 bg-background flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<dl class="grid grid-cols-[max-content_1fr] items-center gap-x-4 gap-y-2 text-sm">
					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">Device ID</dt>
					<dd class="flex items-center gap-2">
						<span class="font-mono text-base font-semibold">{deviceId}</span>
						<CopyButton text={deviceId} />
					</dd>

					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">OS / Arch</dt>
					<dd>
						<span class="pill-primary bg-primary">{latest.os}/{latest.arch}</span>
					</dd>

					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">Submitted by</dt>
					<dd>
						{#if submittedByUser}
							<div class="flex items-center gap-2">
								<div
									class="size-6 shrink-0 overflow-hidden rounded-full bg-gray-50 dark:bg-gray-600"
								>
									{#if submittedByUser.iconURL}
										<img
											src={submittedByUser.iconURL}
											class="h-full w-full object-cover"
											alt=""
											referrerpolicy="no-referrer"
										/>
									{/if}
								</div>
								<span>{userDisplay(submittedByUser)}</span>
							</div>
						{:else if latest.submittedBy}
							<span class="font-mono text-xs">{latest.submittedBy}</span>
						{:else}
							<span class="text-on-surface1">—</span>
						{/if}
					</dd>

					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">OS user</dt>
					<dd class="font-mono">{latest.username || '—'}</dd>

					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">Hostname</dt>
					<dd class="font-mono">{latest.hostname || '—'}</dd>

					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">Scanner</dt>
					<dd class="font-mono">{latest.scannerVersion || '—'}</dd>

					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">Last scanned</dt>
					<dd use:tooltip={scannedTime.fullDate}>
						{scannedTime.relativeTime || '—'}
					</dd>

					<dt class="text-on-surface1 text-xs font-medium tracking-wide uppercase">Total scans</dt>
					<dd>{scans.length}</dd>
				</dl>
			</div>

			<!-- Latest scan tabs -->
			<div class="flex flex-col gap-2">
				<div class="border-surface2 dark:border-surface2 flex gap-2 border-b">
					<button
						class="tab-button"
						class:tab-active={activeTab === 'mcp'}
						onclick={() => (activeTab = 'mcp')}
					>
						<Server class="size-4" /> MCP Servers
						<span class="text-on-surface1">({mcpServers.length})</span>
					</button>
					<button
						class="tab-button"
						class:tab-active={activeTab === 'skills'}
						onclick={() => (activeTab = 'skills')}
					>
						<PencilRuler class="size-4" /> Skills
						<span class="text-on-surface1">({skills.length})</span>
					</button>
					<button
						class="tab-button"
						class:tab-active={activeTab === 'plugins'}
						onclick={() => (activeTab = 'plugins')}
					>
						<Boxes class="size-4" /> Plugins
						<span class="text-on-surface1">({plugins.length})</span>
					</button>
					<button
						class="tab-button"
						class:tab-active={activeTab === 'clients'}
						onclick={() => (activeTab = 'clients')}
					>
						<MonitorCheck class="size-4" /> Clients
						<span class="text-on-surface1">({clients.length})</span>
					</button>
				</div>

				{#if activeTab === 'mcp'}
					{#if mcpRows.length === 0}
						{@render emptyTab('No MCP servers found in the latest scan.')}
					{:else}
						<Table
							data={mcpRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'client', 'scope', 'transport', 'endpoint']}
							headers={[
								{ title: 'Client', property: 'client' },
								{ title: 'Scope', property: 'scope' },
								{ title: 'Name', property: 'name' },
								{ title: 'Transport', property: 'transport' },
								{ title: 'Endpoint', property: 'endpoint' }
							]}
							sortable={['client', 'name', 'transport', 'scope']}
							filterable={['client', 'transport', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									resolve(`/admin/devices/${deviceId}/scans/${latest?.id}/mcp/${d.id}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d: MCPRow)}
								{#if property === 'name'}
									<span class="font-mono text-xs">{d.name}</span>
								{:else if property === 'endpoint'}
									<span class="font-mono text-xs">{d.endpoint}</span>
								{:else if property === 'client'}
									<a
										class="btn-link text-blue-500"
										href={resolve(`/admin/device-clients/${encodeURIComponent(d.client)}`)}
										onclick={(e) => e.stopPropagation()}
									>
										{d.client}
									</a>
								{:else}
									{d[property as keyof MCPRow] ?? '—'}
								{/if}
							{/snippet}

							{#snippet actions(d)}
								<DotDotDot class="icon-button hover:dark:bg-background/50">
									{#snippet icon()}
										<Ellipsis class="size-4" />
									{/snippet}
									{#snippet children({ toggle })}
										<button
											class="menu-button"
											onclick={(e) => {
												if (!d.configHash) {
													console.error('No config hash found for MCP server', d);
													return;
												}
												const isCtrlClick = e.ctrlKey || e.metaKey;
												openUrl(
													resolve(`/admin/device-mcp-servers/${encodeURIComponent(d.configHash)}`),
													isCtrlClick
												);
												toggle();
											}}
										>
											<Scale class="size-4" /> View Related Occurrences
										</button>
									{/snippet}
								</DotDotDot>
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'skills'}
					{#if skillRows.length === 0}
						{@render emptyTab('No skills found in the latest scan.')}
					{:else}
						<Table
							data={skillRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'client', 'scope', 'description', 'hasScripts', 'files_count']}
							headers={[
								{ title: 'Client', property: 'client' },
								{ title: 'Scope', property: 'scope' },
								{ title: 'Name', property: 'name' },
								{ title: 'Description', property: 'description' },
								{ title: 'Has Scripts', property: 'hasScripts' },
								{ title: 'Files', property: 'files_count' }
							]}
							sortable={['client', 'scope', 'name', 'description', 'hasScripts', 'files_count']}
							filterable={['client', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									resolve(`/admin/devices/${deviceId}/scans/${latest?.id}/skills/${d.id}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d: SkillRow)}
								{#if property === 'description'}
									<span class="text-on-surface1 text-xs">{d.description ?? '—'}</span>
								{:else if property === 'hasScripts'}
									{d.hasScripts ? 'yes' : 'no'}
								{:else if property === 'client'}
									<a
										class="btn-link text-blue-500"
										href={resolve(`/admin/device-clients/${encodeURIComponent(d.client)}`)}
										onclick={(e) => e.stopPropagation()}
									>
										{d.client}
									</a>
								{:else}
									{d[property as keyof SkillRow] ?? '—'}
								{/if}
							{/snippet}

							{#snippet actions(d)}
								<DotDotDot class="icon-button hover:dark:bg-background/50">
									{#snippet icon()}
										<Ellipsis class="size-4" />
									{/snippet}
									{#snippet children({ toggle })}
										<button
											class="menu-button"
											onclick={(e) => {
												const isCtrlClick = e.ctrlKey || e.metaKey;
												openUrl(
													resolve(`/admin/device-skills/${encodeURIComponent(d.name)}`),
													isCtrlClick
												);
												toggle();
											}}
										>
											<Scale class="size-4" /> View Related Occurrences
										</button>
									{/snippet}
								</DotDotDot>
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'plugins'}
					{#if pluginRows.length === 0}
						{@render emptyTab('No plugins found in the latest scan.')}
					{:else}
						<Table
							data={pluginRows}
							pageSize={PAGE_SIZE}
							fields={[
								'name',
								'client',
								'scope',
								'pluginType',
								'version',
								'enabled',
								'capabilities'
							]}
							headers={[
								{ title: 'Client', property: 'client' },
								{ title: 'Scope', property: 'scope' },
								{ title: 'Name', property: 'name' },
								{ title: 'Type', property: 'pluginType' },
								{ title: 'Version', property: 'version' },
								{ title: 'Enabled', property: 'enabled' },
								{ title: 'Capabilities', property: 'capabilities' }
							]}
							sortable={['client', 'name', 'pluginType', 'version']}
							filterable={['client', 'pluginType', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									resolve(`/admin/devices/${deviceId}/scans/${latest?.id}/plugins/${d.id}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d: PluginRow)}
								{#if property === 'enabled'}
									{d.enabled ? 'yes' : 'no'}
								{:else if property === 'version'}
									<span class="font-mono text-xs">{d.version ?? '—'}</span>
								{:else if property === 'client'}
									<a
										class="btn-link text-blue-500"
										href={resolve(`/admin/device-clients/${encodeURIComponent(d.client)}`)}
										onclick={(e) => e.stopPropagation()}
									>
										{d.client}
									</a>
								{:else}
									{d[property as keyof PluginRow] ?? '—'}
								{/if}
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'clients'}
					{#if clientRows.length === 0}
						{@render emptyTab('No clients observed on this device.')}
					{:else}
						<Table
							data={clientRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'version', 'paths_display', 'has_display']}
							headers={[
								{ title: 'Name', property: 'name' },
								{ title: 'Version', property: 'version' },
								{ title: 'Paths', property: 'paths_display' },
								{ title: 'Has', property: 'has_display' }
							]}
							sortable={['name']}
							filterable={['name']}
						>
							{#snippet onRenderColumn(property, d: ClientRow)}
								{#if property === 'version'}
									<span class="font-mono text-xs">{d.version ?? '—'}</span>
								{:else if property === 'paths_display'}
									<span class="font-mono text-xs">{d.paths_display}</span>
								{:else if property === 'has_display'}
									<span class="text-xs">{d.has_display}</span>
								{:else}
									{d[property as keyof ClientRow] ?? '—'}
								{/if}
							{/snippet}
						</Table>
					{/if}
				{/if}
			</div>

			<!-- Scan history (includes latest as first row) -->
			<div class="flex flex-col gap-2">
				<h3 class="text-on-surface1 text-sm font-semibold">
					Scan history · {scans.length}
				</h3>
				<Table
					data={historyRows}
					fields={[
						'id',
						'scanned_relative',
						'scanner_version',
						'mcp_count',
						'skill_count',
						'plugin_count',
						'client_count'
					]}
					headers={[
						{ title: 'Scan', property: 'id' },
						{ title: 'Scanned', property: 'scanned_relative' },
						{ title: 'Scanner', property: 'scanner_version' },
						{ title: 'MCP', property: 'mcp_count' },
						{ title: 'Skills', property: 'skill_count' },
						{ title: 'Plugins', property: 'plugin_count' },
						{ title: 'Clients', property: 'client_count' }
					]}
					onClickRow={(d, isCtrlClick) => {
						openUrl(resolve(`/admin/devices/${deviceId}/scans/${d.id}`), isCtrlClick);
					}}
				>
					{#snippet onRenderColumn(property, d: HistoryRow)}
						{#if property === 'id'}
							<span class="flex items-center gap-2">
								<span class="font-mono text-xs">#{d.id}</span>
								{#if d.is_latest}
									<span
										class="bg-primary/15 text-primary rounded-full px-2 py-0.5 text-[10px] font-medium tracking-wide uppercase"
									>
										Latest
									</span>
								{/if}
							</span>
						{:else}
							{d[property as keyof HistoryRow] ?? '—'}
						{/if}
					{/snippet}
				</Table>
			</div>
		{/if}
	</div>
</Layout>

{#snippet emptyTab(msg: string)}
	<div class="text-on-surface1 flex items-center gap-2 p-4 text-sm font-light">
		<Cpu class="size-4 opacity-50" />
		{msg}
	</div>
{/snippet}

<style lang="postcss">
	.tab-button {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		border-bottom: 2px solid transparent;
		font-size: 0.875rem;
		color: var(--on-surface1, #6b7280);
		transition:
			color 200ms,
			border-color 200ms;

		&:hover {
			color: inherit;
		}
	}
	.tab-active {
		color: inherit;
		border-bottom-color: var(--primary);
		font-weight: 500;
	}
</style>
