<script lang="ts">
	import { resolve } from '$app/paths';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AGENTS_HOME_CLIENT_LABEL, deriveDeviceScope, formatDeviceClient } from '$lib/format.js';
	import { m } from '$lib/i18n';
	import {
		UserService,
		type DeviceScan,
		type DeviceScanClient,
		type DeviceScanMCPServer,
		type DeviceScanPlugin,
		type DeviceScanSkill,
		type OrgUser
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Boxes, Cpu, Ellipsis, MonitorCheck, PencilRuler, Scale, Server } from '@lucide/svelte';
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
		UserService.getUser(id, { dontLogErrors: true })
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
		mcpServers.map((srv) => ({
			...srv,
			client: formatDeviceClient(srv.client, srv.projectPath),
			scope: deriveDeviceScope(srv.projectPath),
			endpoint: srv.transport === 'stdio' ? formatCommand(srv.command, srv.args) : srv.url || '—'
		}))
	);

	let skillRows = $derived<SkillRow[]>(
		skills.map((s) => ({
			...s,
			client: formatDeviceClient(s.client, s.projectPath),
			scope: deriveDeviceScope(s.projectPath),
			files_count: (s.files ?? []).length
		}))
	);

	let pluginRows = $derived<PluginRow[]>(
		plugins.map((p) => ({
			...p,
			client: formatDeviceClient(p.client, p.projectPath),
			scope: deriveDeviceScope(p.projectPath),
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

	const hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
</script>

<svelte:head>
	<title>{m.inventory_enforcement_devices_page_title_device({ id: deviceId.slice(0, 12) })}</title>
</svelte:head>

<Layout
	title={m.inventory_enforcement_devices_device_title()}
	showBackButton
	onBackButtonClick={() => {
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto(resolve('/inventory?view=devices'));
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !latest}
			<p class="text-muted-content text-sm font-light">
				{m.inventory_enforcement_devices_no_scans_for_device()}
			</p>
		{:else}
			<!-- Header card -->
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<dl class="grid grid-cols-[max-content_1fr] items-center gap-x-4 gap-y-2 text-sm">
					<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
						{m.inventory_enforcement_enforcement_events_device_id()}
					</dt>
					<dd class="flex items-center gap-2">
						<span class="text-base font-semibold">{deviceId}</span>
						<CopyButton text={deviceId} />
					</dd>

					<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
						{m.inventory_enforcement_devices_label_os_arch()}
					</dt>
					<dd>
						<span class="pill-primary bg-primary">{latest.os}/{latest.arch}</span>
					</dd>

					{#if hasAdminAccess}
						<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
							{m.inventory_enforcement_devices_label_submitted_by()}
						</dt>
						<dd>
							{#if submittedByUser}
								<div class="flex items-center gap-2">
									<div
										class="size-6 shrink-0 overflow-hidden rounded-full bg-base-100 dark:bg-base-300"
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
								<span class="text-xs">{latest.submittedBy}</span>
							{:else}
								<span class="text-muted-content">—</span>
							{/if}
						</dd>
					{/if}

					<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
						{m.inventory_enforcement_devices_label_os_user()}
					</dt>
					<dd>{latest.username || '—'}</dd>

					<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
						{m.core_col_hostname()}
					</dt>
					<dd>{latest.hostname || '—'}</dd>

					<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
						{m.inventory_enforcement_devices_label_scanner()}
					</dt>
					<dd>{latest.scannerVersion || '—'}</dd>

					<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
						{m.inventory_enforcement_devices_label_last_scanned()}
					</dt>
					<dd use:tooltip={scannedTime.fullDate}>
						{scannedTime.relativeTime || '—'}
					</dd>

					<dt class="text-muted-content text-xs font-medium tracking-wide uppercase">
						{m.inventory_enforcement_devices_label_total_scans()}
					</dt>
					<dd>{scans.length}</dd>
				</dl>
			</div>

			<!-- Latest scan tabs -->
			<div class="flex flex-col gap-2">
				<div class="border-base-300 flex gap-2 border-b">
					<button
						class="tab-button"
						class:tab-active={activeTab === 'clients'}
						onclick={() => (activeTab = 'clients')}
					>
						<MonitorCheck class="size-4" />
						{m.inventory_enforcement_overview_clients()}
						<span class="text-muted-content">({clients.length})</span>
					</button>
					<button
						class="tab-button"
						class:tab-active={activeTab === 'mcp'}
						onclick={() => (activeTab = 'mcp')}
					>
						<Server class="size-4" />
						{m.inventory_enforcement_tab_mcp_servers()}
						<span class="text-muted-content">({mcpServers.length})</span>
					</button>
					<button
						class="tab-button"
						class:tab-active={activeTab === 'skills'}
						onclick={() => (activeTab = 'skills')}
					>
						<PencilRuler class="size-4" />
						{m.inventory_enforcement_skills_tab()}
						<span class="text-muted-content">({skills.length})</span>
					</button>
					<button
						class="tab-button"
						class:tab-active={activeTab === 'plugins'}
						onclick={() => (activeTab = 'plugins')}
					>
						<Boxes class="size-4" />
						{m.inventory_enforcement_devices_col_plugins()}
						<span class="text-muted-content">({plugins.length})</span>
					</button>
				</div>

				{#if activeTab === 'mcp'}
					{#if mcpRows.length === 0}
						{@render emptyTab(m.inventory_enforcement_devices_no_mcp_latest_scan())}
					{:else}
						<Table
							data={mcpRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'client', 'scope', 'transport', 'endpoint']}
							headers={[
								{ title: m.inventory_enforcement_col_client(), property: 'client' },
								{ title: m.inventory_enforcement_col_scope(), property: 'scope' },
								{ title: m.core_name(), property: 'name' },
								{ title: m.inventory_enforcement_col_transport(), property: 'transport' },
								{ title: m.inventory_enforcement_col_endpoint(), property: 'endpoint' }
							]}
							sortable={['client', 'name', 'transport', 'scope']}
							filterable={['client', 'transport', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									resolve(`/inventory/devices/${deviceId}/scans/${latest?.id}/mcp/${d.id}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d: MCPRow)}
								{#if property === 'client'}
									{@render clientLink(d.client)}
								{:else}
									{d[property as keyof MCPRow] ?? '—'}
								{/if}
							{/snippet}

							{#snippet actions(d)}
								{#if hasAdminAccess}
									<DotDotDot class="hover:dark:bg-base-100/50">
										{#snippet icon()}
											<Ellipsis class="size-4" />
										{/snippet}
										{#snippet children({ toggle })}
											<button
												class="menu-button"
												onclick={(e) => {
													e.stopPropagation();
													e.preventDefault();
													if (!d.configHash) {
														console.error('No config hash found for MCP server', d);
														return;
													}
													const isCtrlClick = e.ctrlKey || e.metaKey;
													openUrl(
														resolve(`/inventory/mcp-servers/${encodeURIComponent(d.configHash)}`),
														isCtrlClick
													);
													toggle();
												}}
											>
												<Scale class="size-4" />
												{m.inventory_enforcement_devices_view_related_occurrences()}
											</button>
										{/snippet}
									</DotDotDot>
								{/if}
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'skills'}
					{#if skillRows.length === 0}
						{@render emptyTab(m.inventory_enforcement_devices_no_skills_latest_scan())}
					{:else}
						<Table
							data={skillRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'client', 'scope', 'description', 'hasScripts', 'files_count']}
							headers={[
								{ title: m.inventory_enforcement_col_client(), property: 'client' },
								{ title: m.inventory_enforcement_col_scope(), property: 'scope' },
								{ title: m.core_name(), property: 'name' },
								{ title: m.core_description(), property: 'description' },
								{ title: m.inventory_enforcement_col_has_scripts(), property: 'hasScripts' },
								{ title: m.inventory_enforcement_col_files(), property: 'files_count' }
							]}
							sortable={['client', 'scope', 'name', 'description', 'hasScripts', 'files_count']}
							filterable={['client', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									resolve(`/inventory/devices/${deviceId}/scans/${latest?.id}/skills/${d.id}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d: SkillRow)}
								{#if property === 'description'}
									<span class="text-muted-content text-xs">{d.description ?? '—'}</span>
								{:else if property === 'hasScripts'}
									{d.hasScripts
										? m.inventory_enforcement_devices_yes()
										: m.inventory_enforcement_devices_no()}
								{:else if property === 'client'}
									{@render clientLink(d.client)}
								{:else}
									{d[property as keyof SkillRow] ?? '—'}
								{/if}
							{/snippet}

							{#snippet actions(d)}
								{#if hasAdminAccess}
									<DotDotDot class="hover:dark:bg-base-100/50">
										{#snippet icon()}
											<Ellipsis class="size-4" />
										{/snippet}
										{#snippet children({ toggle })}
											<button
												class="menu-button"
												onclick={(e) => {
													const isCtrlClick = e.ctrlKey || e.metaKey;
													openUrl(
														resolve(`/inventory/skills/${encodeURIComponent(d.name)}`),
														isCtrlClick
													);
													toggle();
												}}
											>
												<Scale class="size-4" />
												{m.inventory_enforcement_devices_view_related_occurrences()}
											</button>
										{/snippet}
									</DotDotDot>
								{/if}
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'plugins'}
					{#if pluginRows.length === 0}
						{@render emptyTab(m.inventory_enforcement_devices_no_plugins_latest_scan())}
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
								{ title: m.inventory_enforcement_col_client(), property: 'client' },
								{ title: m.inventory_enforcement_col_scope(), property: 'scope' },
								{ title: m.core_name(), property: 'name' },
								{ title: m.core_type(), property: 'pluginType' },
								{
									title: m.inventory_enforcement_enforcement_events_version(),
									property: 'version'
								},
								{ title: m.core_status_enabled(), property: 'enabled' },
								{
									title: m.inventory_enforcement_devices_col_capabilities(),
									property: 'capabilities'
								}
							]}
							sortable={['client', 'name', 'pluginType', 'version']}
							filterable={['client', 'pluginType', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									resolve(`/inventory/devices/${deviceId}/scans/${latest?.id}/plugins/${d.id}`),
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d: PluginRow)}
								{#if property === 'enabled'}
									{d.enabled
										? m.inventory_enforcement_devices_yes()
										: m.inventory_enforcement_devices_no()}
								{:else if property === 'version'}
									{d.version ?? '—'}
								{:else if property === 'client'}
									{@render clientLink(d.client)}
								{:else}
									{d[property as keyof PluginRow] ?? '—'}
								{/if}
							{/snippet}
						</Table>
					{/if}
				{:else if activeTab === 'clients'}
					{#if clientRows.length === 0}
						{@render emptyTab(m.inventory_enforcement_devices_no_clients_on_device())}
					{:else}
						<Table
							data={clientRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'version', 'paths_display', 'has_display']}
							headers={[
								{ title: m.core_name(), property: 'name' },
								{
									title: m.inventory_enforcement_enforcement_events_version(),
									property: 'version'
								},
								{ title: m.inventory_enforcement_devices_col_paths(), property: 'paths_display' },
								{ title: m.inventory_enforcement_devices_col_has(), property: 'has_display' }
							]}
							sortable={['name']}
							filterable={['name']}
						>
							{#snippet onRenderColumn(property, d: ClientRow)}
								{d[property as keyof ClientRow] ?? '—'}
							{/snippet}
						</Table>
					{/if}
				{/if}
			</div>

			<!-- Scan history (includes latest as first row) -->
			<div class="flex flex-col gap-2">
				<h3 class="text-muted-content text-sm font-semibold">
					{m.inventory_enforcement_devices_scan_history({ count: scans.length })}
				</h3>
				<Table
					data={historyRows}
					fields={[
						'scanned_relative',
						'scanner_version',
						'mcp_count',
						'skill_count',
						'plugin_count',
						'client_count'
					]}
					headers={[
						{ title: m.inventory_enforcement_col_scanned(), property: 'scanned_relative' },
						{ title: m.inventory_enforcement_devices_label_scanner(), property: 'scanner_version' },
						{ title: m.inventory_enforcement_devices_col_mcp(), property: 'mcp_count' },
						{ title: m.inventory_enforcement_skills_tab(), property: 'skill_count' },
						{ title: m.inventory_enforcement_devices_col_plugins(), property: 'plugin_count' },
						{ title: m.inventory_enforcement_overview_clients(), property: 'client_count' }
					]}
					onClickRow={(d, isCtrlClick) => {
						openUrl(resolve(`/inventory/devices/${deviceId}/scans/${d.id}`), isCtrlClick);
					}}
				>
					{#snippet onRenderColumn(property, d: HistoryRow)}
						{#if property === 'scanned_relative'}
							<span class="flex items-center gap-2" use:tooltip={d.scanned_at}>
								<span>{d.scanned_relative || '—'}</span>
								{#if d.is_latest}
									<span
										class="bg-primary/15 text-primary rounded-full px-2 py-0.5 text-[10px] font-medium tracking-wide uppercase"
									>
										{m.inventory_enforcement_devices_latest()}
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
	<div class="text-muted-content flex items-center gap-2 p-4 text-sm font-light">
		<Cpu class="size-4 opacity-50" />
		{msg}
	</div>
{/snippet}

{#snippet clientLink(client?: string)}
	{#if client && client.trim() !== 'multi' && client !== AGENTS_HOME_CLIENT_LABEL && hasAdminAccess}
		<a
			class="btn-link text-blue-500"
			href={resolve(`/inventory/clients/${encodeURIComponent(client)}`)}
			onclick={(e) => e.stopPropagation()}
		>
			{client}
		</a>
	{:else}
		{client || '-'}
	{/if}
{/snippet}
