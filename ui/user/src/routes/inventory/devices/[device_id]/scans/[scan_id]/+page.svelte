<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AGENTS_HOME_CLIENT_LABEL,
		deriveDeviceScope,
		formatDeviceClient,
		formatDeviceCommand
	} from '$lib/format.js';
	import { m } from '$lib/i18n';
	import {
		UserService,
		AdminService,
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
	import { Boxes, Cpu, MonitorCheck, PencilRuler, Server, Trash2 } from '@lucide/svelte';
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

	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
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

	$effect(() => {
		const deviceId = scanDeviceId;
		const id = scanIdNum;
		if (!deviceId || !id) {
			isLatest = false;
			return;
		}
		UserService.listDeviceScans({ deviceId: [deviceId], groupByDevice: true, limit: 1 })
			.then((res) => {
				if (scanDeviceId !== deviceId || scanIdNum !== id) return;
				const top = res.items?.[0];
				isLatest = top != null && top.id === id;
			})
			.catch(() => {
				if (scanDeviceId === deviceId && scanIdNum === id) isLatest = false;
			});
	});

	let canDelete = $derived(hasAdminAccess && !profile.current.isAdminReadonly?.());
	let deleteOpen = $state(false);
	let deleting = $state(false);
	let deleteError = $state<string | undefined>();

	async function confirmDelete() {
		if (!scan) return;
		deleting = true;
		deleteError = undefined;
		try {
			await AdminService.deleteDeviceScan(scan.id);
			deleteOpen = false;
			goto(`/inventory/devices/${page.params.device_id}`);
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

	let mcpRows = $derived<MCPRow[]>(
		mcpServers.map((srv) => ({
			...srv,
			client: formatDeviceClient(srv.client, srv.projectPath),
			scope: deriveDeviceScope(srv.projectPath),
			endpoint:
				srv.transport === 'stdio' ? formatDeviceCommand(srv.command, srv.args) : srv.url || '—'
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
	<title>{m.inventory_enforcement_devices_page_title_scan()}</title>
</svelte:head>

<Layout
	title={m.inventory_enforcement_devices_scan_title()}
	showBackButton
	onBackButtonClick={() => goto(`/inventory/devices/${deviceIdParam}`)}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !scan}
			<p class="text-muted-content text-sm font-light">
				{m.inventory_enforcement_devices_scan_not_found()}
			</p>
		{:else}
			<!-- Header card -->
			<div
				class="dark:bg-base-300 bg-base-100 flex flex-col gap-4 rounded-md p-4 shadow-sm md:flex-row md:items-start md:justify-between"
			>
				<dl class="grid flex-1 grid-cols-[max-content_1fr] items-center gap-x-4 gap-y-2 text-sm">
					<dt class="text-muted-content text-xs font-medium uppercase tracking-wide">
						{m.inventory_enforcement_enforcement_events_device_id()}
					</dt>
					<dd class="flex items-center gap-2">
						<span class="text-base font-semibold">{scan.deviceID}</span>
						<CopyButton text={scan.deviceID} />
						{#if isLatest}
							<span
								class="bg-primary/15 text-primary rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide"
							>
								{m.inventory_enforcement_devices_latest()}
							</span>
						{/if}
					</dd>

					<dt class="text-muted-content text-xs font-medium uppercase tracking-wide">
						{m.inventory_enforcement_devices_label_os_arch()}
					</dt>
					<dd>
						<span class="pill-primary bg-primary">{scan.os}/{scan.arch}</span>
					</dd>

					{#if hasAdminAccess}
						<dt class="text-muted-content text-xs font-medium uppercase tracking-wide">
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
							{:else if scan.submittedBy}
								<span class="text-xs">{scan.submittedBy}</span>
							{:else}
								<span class="text-muted-content">—</span>
							{/if}
						</dd>
					{/if}

					<dt class="text-muted-content text-xs font-medium uppercase tracking-wide">
						{m.inventory_enforcement_devices_label_scanner()}
					</dt>
					<dd>{scan.scannerVersion || '—'}</dd>

					<dt class="text-muted-content text-xs font-medium uppercase tracking-wide">
						{m.inventory_enforcement_col_scanned()}
					</dt>
					<dd use:tooltip={scannedTime.fullDate}>
						{scannedTime.relativeTime || '—'}
					</dd>
				</dl>
				{#if canDelete}
					<button
						type="button"
						class="btn btn-error flex items-center gap-1.5 self-start"
						onclick={() => (deleteOpen = true)}
					>
						<Trash2 class="size-4" />
						{m.core_delete()}
					</button>
				{/if}
			</div>

			<!-- Tabs -->
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
						{@render emptyTab(m.inventory_enforcement_devices_no_mcp_this_scan())}
					{:else}
						<Table
							data={mcpRows}
							pageSize={PAGE_SIZE}
							fields={['name', 'client', 'scope', 'transport', 'endpoint']}
							headers={[
								{ title: m.core_name(), property: 'name' },
								{ title: m.inventory_enforcement_col_client(), property: 'client' },
								{ title: m.inventory_enforcement_col_scope(), property: 'scope' },
								{ title: m.inventory_enforcement_col_transport(), property: 'transport' },
								{ title: m.inventory_enforcement_col_endpoint(), property: 'endpoint' }
							]}
							sortable={['client', 'name', 'transport', 'scope']}
							filterable={['client', 'transport', 'scope']}
							onClickRow={(d, isCtrlClick) => {
								openUrl(
									`/inventory/devices/${deviceIdParam}/scans/${scanIdParam}/mcp/${d.id}`,
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
						</Table>
					{/if}
				{:else if activeTab === 'skills'}
					{#if skillRows.length === 0}
						{@render emptyTab(m.inventory_enforcement_devices_no_skills_this_scan())}
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
									`/inventory/devices/${deviceIdParam}/scans/${scanIdParam}/skills/${d.id}`,
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
						</Table>
					{/if}
				{:else if activeTab === 'plugins'}
					{#if pluginRows.length === 0}
						{@render emptyTab(m.inventory_enforcement_devices_no_plugins_this_scan())}
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
									`/inventory/devices/${deviceIdParam}/scans/${scanIdParam}/plugins/${d.id}`,
									isCtrlClick
								);
							}}
						>
							{#snippet onRenderColumn(property, d: PluginRow)}
								{#if property === 'enabled'}
									{d.enabled
										? m.inventory_enforcement_devices_yes()
										: m.inventory_enforcement_devices_no()}
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
							onClickRow={hasAdminAccess
								? (d, isCtrlClick) => {
										if (d.name.trim() === 'multi') return;
										openUrl(
											resolve(`/inventory/clients/${encodeURIComponent(d.name)}`),
											isCtrlClick
										);
									}
								: undefined}
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
		{/if}
	</div>
</Layout>

<Confirm
	show={deleteOpen}
	loading={deleting}
	title={m.inventory_enforcement_devices_delete_scan_title()}
	msg={scan
		? m.inventory_enforcement_devices_delete_scan_msg({ deviceId: scan.deviceID })
		: m.inventory_enforcement_devices_delete_scan_msg_generic()}
	note={deleteError ?? m.inventory_enforcement_devices_delete_scan_note()}
	onsuccess={confirmDelete}
	oncancel={() => {
		deleteOpen = false;
		deleteError = undefined;
	}}
/>

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

{#snippet emptyTab(msg: string)}
	<div class="text-muted-content flex items-center gap-2 p-4 text-sm font-light">
		<Cpu class="size-4 opacity-50" />
		{msg}
	</div>
{/snippet}
