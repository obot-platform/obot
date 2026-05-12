<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { formatDeviceCommand } from '$lib/format.js';
	import { AdminService } from '$lib/services';
	import { deleteDeviceScan } from '$lib/services/admin/operations';
	import {
		Group,
		type DeviceScan,
		type DeviceScanClient,
		type DeviceScanMCPServer,
		type DeviceScanPlugin,
		type DeviceScanSkill,
		type OrgUser
	} from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Boxes, Cpu, MonitorCheck, PencilRuler, Server, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	type Tab = 'mcp' | 'skills' | 'plugins' | 'clients';

	const PAGE_SIZE = 50;

	let { data } = $props();
	let scan = $derived<DeviceScan | undefined>(data?.scan);
	let activeTab = $state<Tab>('mcp');

	let submittedByUser = $state<OrgUser | undefined>();
	let submittedById = $derived(scan?.submittedBy);
	let isLatest = $state(false);
	let scanDeviceId = $derived(scan?.deviceID);
	let scanIdNum = $derived(scan?.id);

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

	$effect(() => {
		const deviceId = scanDeviceId;
		const id = scanIdNum;
		if (!deviceId || !id) {
			isLatest = false;
			return;
		}
		AdminService.listDeviceScans({ deviceId: [deviceId], groupByDevice: true, limit: 1 })
			.then((res) => {
				if (scanDeviceId !== deviceId || scanIdNum !== id) return;
				const top = res.items?.[0];
				isLatest = top != null && top.id === id;
			})
			.catch(() => {
				if (scanDeviceId === deviceId && scanIdNum === id) isLatest = false;
			});
	});

	let canDelete = $derived(
		profile.current.groups?.includes(Group.ADMIN) || profile.current.groups?.includes(Group.OWNER)
	);
	let deleteOpen = $state(false);
	let deleting = $state(false);
	let deleteError = $state<string | undefined>();

	async function confirmDelete() {
		if (!scan) return;
		deleting = true;
		deleteError = undefined;
		try {
			await deleteDeviceScan(scan.id);
			deleteOpen = false;
			goto(`/admin/devices/${page.params.device_id}`);
		} catch (e) {
			deleteError = e instanceof Error ? e.message : String(e);
		} finally {
			deleting = false;
		}
	}

	const duration = PAGE_TRANSITION_DURATION;

	let mcpServers = $derived<DeviceScanMCPServer[]>(scan?.mcpServers ?? []);
	let skills = $derived<DeviceScanSkill[]>(scan?.skills ?? []);
	let plugins = $derived<DeviceScanPlugin[]>(scan?.plugins ?? []);
	let clients = $derived<DeviceScanClient[]>(scan?.clients ?? []);

	let scannedTime = $derived(
		scan ? formatTimeAgo(scan.scannedAt) : { relativeTime: '', fullDate: '' }
	);

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

	let mcpRows = $derived<MCPRow[]>(
		mcpServers.map((m) => ({
			...m,
			scope: deriveScope(m.projectPath),
			endpoint: m.transport === 'stdio' ? formatDeviceCommand(m.command, m.args) : m.url || '—'
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

	let deviceIdParam = $derived(page.params.device_id);
	let scanIdParam = $derived(page.params.scan_id);

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
</script>

<svelte:head>
	<title>Obot | Device Scan</title>
</svelte:head>

<Layout
	title="Device Scan"
	showBackButton
	onBackButtonClick={() => goto(`/admin/devices/${deviceIdParam}`)}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !scan}
			<p class="text-on-surface1 text-sm font-light">Scan not found.</p>
		{:else}
			<!-- Header card -->
			<div
				class="dark:bg-surface2 bg-background flex flex-col gap-4 rounded-md p-4 shadow-sm md:flex-row md:items-start md:justify-between"
			>
				<dl class="grid flex-1 grid-cols-[max-content_1fr] items-center gap-x-4 gap-y-2 text-sm">
					<dt class="text-on-surface1 text-xs font-medium uppercase tracking-wide">Device ID</dt>
					<dd class="flex items-center gap-2">
						<span class="font-mono text-base font-semibold">{scan.deviceID}</span>
						<CopyButton text={scan.deviceID} />
						{#if isLatest}
							<span
								class="bg-primary/15 text-primary rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide"
							>
								Latest
							</span>
						{/if}
					</dd>

					<dt class="text-on-surface1 text-xs font-medium uppercase tracking-wide">OS / Arch</dt>
					<dd>
						<span class="pill-primary bg-primary">{scan.os}/{scan.arch}</span>
					</dd>

					<dt class="text-on-surface1 text-xs font-medium uppercase tracking-wide">Submitted by</dt>
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
						{:else if scan.submittedBy}
							<span class="font-mono text-xs">{scan.submittedBy}</span>
						{:else}
							<span class="text-on-surface1">—</span>
						{/if}
					</dd>

					<dt class="text-on-surface1 text-xs font-medium uppercase tracking-wide">Scanner</dt>
					<dd class="font-mono">{scan.scannerVersion || '—'}</dd>

					<dt class="text-on-surface1 text-xs font-medium uppercase tracking-wide">Scanned</dt>
					<dd use:tooltip={scannedTime.fullDate}>
						{scannedTime.relativeTime || '—'}
					</dd>
				</dl>
				{#if canDelete}
					<button
						type="button"
						class="button-destructive flex items-center gap-1.5 self-start"
						onclick={() => (deleteOpen = true)}
					>
						<Trash2 class="size-4" /> Delete
					</button>
				{/if}
			</div>

			<!-- Tabs -->
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
						{@render emptyTab('No MCP servers found in this scan.')}
					{:else}
						<Table
							data={mcpRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'client', 'scope', 'transport', 'endpoint']}
							headers={[
								{ title: 'Name', property: 'name' },
								{ title: 'Client', property: 'client' },
								{ title: 'Scope', property: 'scope' },
								{ title: 'Transport', property: 'transport' },
								{ title: 'Endpoint', property: 'endpoint' }
							]}
							sortable={['client', 'name', 'transport', 'scope']}
							filterable={['client', 'transport', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									`/admin/devices/${deviceIdParam}/scans/${scanIdParam}/mcp/${d.id}`,
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
						</Table>
					{/if}
				{:else if activeTab === 'skills'}
					{#if skillRows.length === 0}
						{@render emptyTab('No skills found in this scan.')}
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
									`/admin/devices/${deviceIdParam}/scans/${scanIdParam}/skills/${d.id}`,
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
						</Table>
					{/if}
				{:else if activeTab === 'plugins'}
					{#if pluginRows.length === 0}
						{@render emptyTab('No plugins found in this scan.')}
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
									`/admin/devices/${deviceIdParam}/scans/${scanIdParam}/plugins/${d.id}`,
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
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									resolve(`/admin/device-clients/${encodeURIComponent(d.name)}`),
									isCtrlClick
								);
							}}
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
		{/if}
	</div>
</Layout>

<Confirm
	show={deleteOpen}
	loading={deleting}
	title="Delete device scan"
	msg={scan ? `Delete scan for device ${scan.deviceID}?` : 'Delete this scan?'}
	note={deleteError ??
		'This permanently removes the scan and all associated MCP server, skill, plugin, client, and file rows. This cannot be undone.'}
	onsuccess={confirmDelete}
	oncancel={() => {
		deleteOpen = false;
		deleteError = undefined;
	}}
/>

{#snippet emptyTab(msg: string)}
	<div class="text-on-surface1 flex items-center gap-2 p-4 text-sm font-light">
		<Cpu class="size-4 opacity-50" />
		{msg}
	</div>
{/snippet}
