<script lang="ts">
	import {
		AdminService,
		ChatService,
		Group,
		type K8sServerDetail,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type ServerK8sSettings
	} from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { formatTimeAgo } from '$lib/time';
	import {
		AlertTriangle,
		Info,
		LoaderCircle,
		RotateCcw,
		RefreshCw,
		CircleFadingArrowUp
	} from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import Table from '../table/Table.svelte';
	import Confirm from '../Confirm.svelte';
	import { twMerge } from 'tailwind-merge';
	import { profile } from '$lib/stores';
	import { page } from '$app/state';
	import SensitiveInput from '../SensitiveInput.svelte';
	import { resolve } from '$app/paths';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import DeploymentLogs from './DeploymentLogs.svelte';

	interface Props {
		id?: string;
		entity?: 'workspace' | 'catalog';
		mcpServerId: string;
		name: string;
		mcpServerInstanceId?: string;
		connectedUsers: (OrgUser & { mcpInstanceId?: string })[];
		title?: string;
		classes?: {
			title?: string;
		};
		catalogEntry?: MCPCatalogEntry;
		mcpServer?: MCPCatalogServer;
		readonly?: boolean;
		compositeParentName?: string;
	}

	const {
		id: entityId,
		mcpServerId,
		mcpServerInstanceId,
		name,
		connectedUsers,
		title,
		classes,
		catalogEntry,
		mcpServer,
		compositeParentName,
		entity = 'catalog',
		readonly
	}: Props = $props();

	let listK8sInfo = $state<Promise<K8sServerDetail>>();
	let listK8sSettingsStatus = $state<Promise<ServerK8sSettings>>();
	let revealServerValues = $state<Promise<Record<string, string>>>();
	let messages = $state<string[]>([]);
	let error = $state<string>();
	let showRestartConfirm = $state(false);
	let restarting = $state(false);
	let refreshingEvents = $state(false);
	let refreshingLogs = $state(false);
	let showUpdateK8sSettingsConfirm = $state(false);
	let updatingK8sSettings = $state(false);
	let isAdminUrl = $derived(page.url.pathname.includes('/admin'));

	let logsUrl = $derived.by(() => {
		if (entity === 'workspace') {
			return catalogEntry?.id
				? `/api/workspaces/${entityId}/entries/${catalogEntry.id}/servers/${mcpServerId}/logs`
				: `/api/workspaces/${entityId}/servers/${mcpServerId}/logs`;
		}

		return `/api/mcp-servers/${mcpServerId}/logs`;
	});

	let deploymentLogsInstance = $state<ReturnType<typeof DeploymentLogs>>();

	const eventStream = new EventStreamService<string>();
	const dontLogErrors = true;

	function handleScroll() {
		deploymentLogsInstance?.scroll();
	}

	function getK8sInfo() {
		return entity === 'workspace' && entityId
			? catalogEntry?.id
				? ChatService.getWorkspaceCatalogEntryServerK8sDetails(
						entityId,
						catalogEntry.id,
						mcpServerId,
						{ dontLogErrors }
					)
				: ChatService.getWorkspaceK8sServerDetail(entityId, mcpServerId, { dontLogErrors })
			: AdminService.getK8sServerDetail(mcpServerId, { dontLogErrors });
	}

	function getK8sSettingsStatus() {
		return entity === 'workspace' && entityId
			? catalogEntry?.id
				? ChatService.getWorkspaceCatalogEntryServerK8sSettingsStatus(
						entityId,
						catalogEntry.id,
						mcpServerId,
						{
							dontLogErrors
						}
					)
				: ChatService.getWorkspaceK8sServerStatus(entityId, mcpServerId, {
						dontLogErrors
					})
			: catalogEntry?.id
				? AdminService.getMCPCatalogServerK8sSettingsStatus(catalogEntry.id, mcpServerId, {
						dontLogErrors
					})
				: AdminService.getK8sSettingsStatus(mcpServerId, { dontLogErrors });
	}

	onMount(() => {
		revealServerValues = profile.current.isAdmin?.()
			? ChatService.revealSingleOrRemoteMcpServer(mcpServerId, {
					dontLogErrors: true
				})
			: Promise.resolve<Record<string, string>>({});
		listK8sInfo = getK8sInfo();
		listK8sSettingsStatus = getK8sSettingsStatus();
		eventStream.connect(logsUrl, {
			onMessage: (data) => {
				messages = [...messages, data];
				// Trigger auto-scroll after adding new message
				handleScroll();
			},
			onOpen: () => {
				console.debug(`${mcpServerId} event stream opened`);
				error = undefined;
			},
			onError: () => {
				error = 'Connection failed';
			},
			onClose: () => {
				console.debug(`${mcpServerId} event stream closed`);
			}
		});
	});

	onDestroy(() => {
		eventStream.disconnect();
	});

	async function handleRestart() {
		restarting = true;
		try {
			await (entity === 'workspace' && entityId
				? catalogEntry?.id
					? ChatService.restartWorkspaceCatalogEntryServerDeployment(
							entityId,
							catalogEntry.id,
							mcpServerId
						)
					: ChatService.restartWorkspaceK8sServerDeployment(entityId, mcpServerId)
				: AdminService.restartK8sDeployment(mcpServerId));
			// Refresh the k8s info after restart
			listK8sInfo = getK8sInfo();
		} catch (err) {
			console.error('Failed to restart deployment:', err);
		} finally {
			restarting = false;
			showRestartConfirm = false;
		}
	}

	async function handleRefreshEvents() {
		refreshingEvents = true;
		try {
			listK8sInfo = getK8sInfo();
		} catch (err) {
			console.error('Failed to refresh events:', err);
		} finally {
			refreshingEvents = false;
		}
	}

	async function handleRefreshLogs() {
		refreshingLogs = true;
		try {
			// Clear existing messages and reconnect to get fresh logs
			messages = [];
			eventStream.disconnect();
			eventStream.connect(logsUrl, {
				onMessage: (data) => {
					messages = [...messages, data];
					// Trigger auto-scroll after adding new message
					handleScroll();
				},
				onOpen: () => {
					console.debug(`${mcpServerId} event stream opened`);
					error = undefined;
				},
				onError: () => {
					error = 'Connection failed';
				},
				onClose: () => {
					console.debug(`${mcpServerId} event stream closed`);
				}
			});
		} catch (err) {
			console.error('Failed to refresh logs:', err);
		} finally {
			refreshingLogs = false;
		}
	}

	function compileK8sInfo(info?: K8sServerDetail) {
		if (!info) return [];
		const details = [
			{
				id: 'kubernetes_deployments',
				label: 'Deployment',
				value: `${info.namespace}/${info.deploymentName}`
			},
			{
				id: 'last_restart',
				label: 'Last Restart',
				value: formatTimeAgo(info.lastRestart).relativeTime
			},
			{
				id: 'status',
				label: 'Status',
				value: info.isAvailable ? 'Healthy' : 'Unhealthy'
			}
		];
		return details;
	}

	async function handleRedeployWithK8sSettings() {
		updatingK8sSettings = true;
		try {
			await (entity === 'workspace' && entityId
				? catalogEntry?.id
					? ChatService.redeployWorkspaceCatalogEntryServerWithK8sSettings(
							entityId,
							catalogEntry.id,
							mcpServerId
						)
					: ChatService.redeployWorkspaceK8sServerWithK8sSettings(entityId, mcpServerId)
				: catalogEntry?.id
					? AdminService.redeployMCPCatalogServerWithK8sSettings(catalogEntry.id, mcpServerId)
					: AdminService.redeployWithK8sSettings(
							mcpServerId,
							mcpServer?.mcpCatalogID ?? DEFAULT_MCP_CATALOG_ID
						));
			listK8sSettingsStatus = getK8sSettingsStatus();
		} catch (err) {
			console.error('Failed to update Kubernetes settings:', err);
		} finally {
			updatingK8sSettings = false;
			showUpdateK8sSettingsConfirm = false;
		}
	}

	function compileRevealedValues(
		revealedValues?: Record<string, string>,
		catalogEntry?: MCPCatalogEntry
	) {
		if (!catalogEntry || !revealedValues) {
			return {
				headers: [],
				envs: []
			};
		}

		const envMap = new Map(catalogEntry.manifest.env?.map((env) => [env.key, env]));
		const headerMap = new Map(
			catalogEntry.manifest.remoteConfig?.headers?.map((header) => [header.key, header])
		);

		const envs: { id: string; label: string; value: string; sensitive: boolean }[] = [];
		const headers: { id: string; label: string; value: string; sensitive: boolean }[] = [];

		for (const key in revealedValues) {
			if (envMap.has(key)) {
				const env = envMap.get(key);
				envs.push({
					id: key,
					label: env?.name ?? 'Unknown',
					value: env?.prefix ? env.prefix + revealedValues[key] : (revealedValues[key] ?? ''),
					sensitive: env?.sensitive || false
				});
			} else if (headerMap.has(key)) {
				const header = headerMap.get(key);
				headers.push({
					id: key,
					label: header?.name ?? 'Unknown',
					value: header?.prefix ? header.prefix + revealedValues[key] : (revealedValues[key] ?? ''),
					sensitive: header?.sensitive || false
				});
			}
		}
		return {
			envs,
			headers
		};
	}

	function getAuditLogUrl(d: (typeof connectedUsers)[number]) {
		const id = mcpServerId || mcpServerInstanceId;

		if (compositeParentName) return null;

		if (isAdminUrl) {
			if (!profile.current?.hasAdminAccess?.()) return null;
			return entity === 'workspace'
				? catalogEntry?.id
					? `/admin/mcp-servers/w/${entityId}/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
					: `/admin/mcp-servers/w/${entityId}/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`
				: catalogEntry?.id
					? `/admin/mcp-servers/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
					: `/admin/mcp-servers/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`;
		}

		if (!profile.current?.groups.includes(Group.POWERUSER)) return null;
		return catalogEntry?.id
			? `/mcp-servers/c/${catalogEntry.id}?view=audit-logs&user_id=${d.id}`
			: `/mcp-servers/s/${encodeURIComponent(id ?? '')}?view=audit-logs&user_id=${d.id}`;
	}
</script>

<div class="flex items-center gap-3">
	<h1 class={twMerge('text-2xl font-semibold', classes?.title)}>
		{#if title}
			{title}
		{:else if mcpServerInstanceId}
			{name} | {mcpServerInstanceId}
		{:else}
			{name}
		{/if}
	</h1>
	<button
		onclick={handleRefreshEvents}
		class="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 disabled:opacity-50 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-300"
		disabled={refreshingEvents}
	>
		<RefreshCw class="size-4 {refreshingEvents ? 'animate-spin' : ''}" />
	</button>
</div>

{#if mcpServerInstanceId}
	<div class="notification-info p-3 text-sm font-light">
		<div class="flex items-center gap-3">
			<Info class="size-6" />
			<p>
				This is a multi-user server instance. The server information displayed here is the root
				server that is shared between all server instances.
			</p>
		</div>
	</div>
{/if}

{#await listK8sInfo}
	<div class="flex w-full justify-center">
		<LoaderCircle class="size-6 animate-spin" />
	</div>
{:then info}
	{@const k8sInfo = compileK8sInfo(info)}
	<div class="flex flex-col gap-2">
		{#each k8sInfo as detail (detail.id)}
			{@render detailRow(detail.label, detail.value, detail.id)}
		{/each}
		{#if catalogEntry?.manifest.runtime === 'remote' && mcpServer?.manifest.remoteConfig?.url}
			{@render configurationRow('URL', mcpServer?.manifest.remoteConfig?.url)}
		{/if}
	</div>

	{#if profile.current?.isAdmin?.()}
		{#await revealServerValues}
			<div class="flex w-full justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:then revealedValues}
			{@const { headers, envs } = compileRevealedValues(revealedValues, catalogEntry)}
			{#if catalogEntry?.manifest.runtime === 'remote'}
				<div>
					<h2 class="mb-2 text-lg font-semibold">Headers</h2>
					{#if headers.length > 0}
						<div class="flex flex-col gap-2">
							{#each headers as h (h.id)}
								{@render configurationRow(h.label, h.value, h.sensitive)}
							{/each}
						</div>
					{:else}
						<span class="text-on-surface1 text-sm font-light">No configured headers.</span>
					{/if}
				</div>
			{/if}

			<div>
				<h2 class="mb-2 text-lg font-semibold">Configuration</h2>
				{#if envs.length > 0}
					<div class="flex flex-col gap-2">
						{#each envs as env (env.id)}
							{@render configurationRow(env.label, env.value, env.sensitive)}
						{/each}
					</div>
				{:else}
					<span class="text-on-surface1 text-sm font-light"
						>No configured environment or file variables set.</span
					>
				{/if}
			</div>
		{/await}
	{/if}

	<div>
		<h2 class="mb-2 text-lg font-semibold">Recent Events</h2>
		{#if info?.events && info.events.length > 0}
			{@const tableData = info.events.map((event, index) => ({
				id: `${event.time}-${index}`,
				...event
			}))}
			<Table
				data={tableData}
				fields={['time', 'eventType', 'message']}
				headers={[{ title: 'Event Type', property: 'eventType' }]}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'time'}
						{formatTimeAgo(d.time).fullDate}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
			</Table>
		{:else}
			<span class="text-on-surface1 text-sm font-light">No events.</span>
		{/if}
	</div>
{:catch error}
	{@const isPending = error instanceof Error && error.message.includes('ContainerCreating')}
	{@const needsUpdate = error instanceof Error && error.message.includes('missing required config')}

	{#if needsUpdate}
		<div class="notification-alert">
			<div class="flex grow flex-col gap-2">
				<div class="flex items-center gap-2">
					<AlertTriangle class="size-6 flex-shrink-0 self-start text-yellow-500" />
					<p class="my-0.5 flex flex-col text-sm font-semibold">
						User Configuration Update Required
					</p>
				</div>
				<span class="text-sm font-light break-all">
					The server was recently updated and requires the user to update their configuration.
					Server details and logs are temporarily unavailable as a result.
				</span>
			</div>
		</div>
	{/if}

	<div class="flex flex-col gap-2">
		<div
			class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col rounded-lg border border-transparent p-4 shadow-sm"
		>
			<div class="grid grid-cols-2 gap-4">
				<p class="text-sm font-semibold">Status</p>
				<p class="text-sm font-light">
					{isPending ? 'Pending' : needsUpdate ? 'Update Required' : 'Error'}
				</p>
			</div>
		</div>
	</div>
{/await}

<DeploymentLogs
	bind:this={deploymentLogsInstance}
	bind:messages
	{error}
	refreshing={refreshingLogs}
	onRefresh={handleRefreshLogs}
	onClear={() => (messages = [])}
/>

<div>
	<h2 class="mb-2 text-lg font-semibold">Connected Users</h2>
	<Table data={connectedUsers ?? []} fields={['name']}>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				{d.email || d.username || 'Unknown'}
			{:else}
				{d[property as keyof typeof d]}
			{/if}
		{/snippet}

		{#snippet actions(d)}
			{@const auditLogsUrl = getAuditLogUrl(d)}
			{#if auditLogsUrl}
				<a href={resolve(auditLogsUrl as `/${string}`)} class="button-text"> View Audit Logs </a>
			{/if}
		{/snippet}
	</Table>
</div>

{#snippet detailRow(label: string, value: string, id: string)}
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col rounded-lg border border-transparent p-4 shadow-sm"
	>
		<div class="grid grid-cols-12 gap-4">
			<p class="col-span-4 text-sm font-semibold">{label}</p>
			<div class="col-span-8 flex items-center justify-between">
				<p class="truncate text-sm font-light">{value}</p>
				{#if id === 'status' && !readonly}
					<button
						onclick={() => (showRestartConfirm = true)}
						class="button-primary flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
						disabled={restarting}
					>
						<RotateCcw class="size-3" />
						Restart
					</button>
				{:else if id === 'kubernetes_deployments' && !readonly}
					{#await listK8sSettingsStatus}
						<div class="flex w-full justify-center">
							<LoaderCircle class="size-6 animate-spin" />
						</div>
					{:then k8sSettingsStatus}
						{#if k8sSettingsStatus?.needsK8sUpdate}
							<button
								class="flex items-center gap-2 rounded-md bg-yellow-500/75 px-3 py-1.5 text-xs font-medium text-white hover:bg-yellow-500 disabled:opacity-50"
								disabled={readonly}
								onclick={() => (showUpdateK8sSettingsConfirm = true)}
							>
								<CircleFadingArrowUp class="size-3" />
								Redeploy with Latest Settings
							</button>
						{/if}
					{/await}
				{/if}
			</div>
		</div>
	</div>
{/snippet}

{#snippet configurationRow(label: string, value: string, sensitive?: boolean)}
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col rounded-lg border border-transparent px-4 py-1.5 shadow-sm"
	>
		<div class="grid grid-cols-12 items-center gap-4">
			<p class="col-span-4 text-sm font-semibold">{label}</p>
			<div class="col-span-8 flex items-center justify-between">
				{#if sensitive}
					<SensitiveInput {value} disabled name={label} />
				{:else}
					<input type="text" {value} class="text-input-filled" disabled />
				{/if}
			</div>
		</div>
	</div>
{/snippet}

<Confirm
	show={showRestartConfirm}
	msg={`Restart ${title || name}?`}
	onsuccess={handleRestart}
	oncancel={() => (showRestartConfirm = false)}
	loading={restarting}
>
	{#snippet note()}
		Are you sure you want to restart this deployment? This will cause a brief service interruption.
	{/snippet}
</Confirm>
<Confirm
	show={showUpdateK8sSettingsConfirm}
	msg={`Redeploy ${title || name}?`}
	onsuccess={handleRedeployWithK8sSettings}
	oncancel={() => (showUpdateK8sSettingsConfirm = false)}
	loading={updatingK8sSettings}
>
	{#snippet note()}
		Are you sure you want to redeploy this server with the latest Kubernetes settings? This will
		cause a brief service interruption.
	{/snippet}
</Confirm>
