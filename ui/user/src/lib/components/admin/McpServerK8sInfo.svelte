<script lang="ts">
	import {
		AdminService,
		ChatService,
		type K8sServerDetail,
		type MCPCatalogEntry,
		type OrgUser
	} from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { AlertTriangle, Info, LoaderCircle, RotateCcw, RefreshCw } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import Table from '../table/Table.svelte';
	import Confirm from '../Confirm.svelte';
	import { fade } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';
	import { profile } from '$lib/stores';
	import { page } from '$app/state';
	import SensitiveInput from '../SensitiveInput.svelte';

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
		readonly?: boolean;
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
		entity = 'catalog',
		readonly
	}: Props = $props();

	let listK8sInfo = $state<Promise<K8sServerDetail>>();
	let revealServerValues = $state<Promise<Record<string, string>>>();
	let messages = $state<string[]>([]);
	let error = $state<string>();
	let logsContainer: HTMLDivElement;
	let showRestartConfirm = $state(false);
	let restarting = $state(false);
	let refreshingEvents = $state(false);
	let refreshingLogs = $state(false);
	let isAdminUrl = $derived(page.url.pathname.includes('/admin'));

	let logsUrl = $derived.by(() => {
		if (entity === 'workspace') {
			return catalogEntry?.id
				? `/api/workspaces/${entityId}/entries/${catalogEntry.id}/servers/${mcpServerId}/logs`
				: `/api/workspaces/${entityId}/servers/${mcpServerId}/logs`;
		}

		return `/api/mcp-servers/${mcpServerId}/logs`;
	});

	const eventStream = new EventStreamService<string>();

	function isScrolledToBottom(element: HTMLElement): boolean {
		return Math.abs(element.scrollHeight - element.clientHeight - element.scrollTop) < 10;
	}

	function scrollToBottom(element: HTMLElement) {
		element.scrollTop = element.scrollHeight;
	}

	function handleScroll() {
		if (logsContainer) {
			const wasAtBottom = isScrolledToBottom(logsContainer);
			if (wasAtBottom) {
				setTimeout(() => scrollToBottom(logsContainer), 0);
			}
		}
	}

	onMount(() => {
		revealServerValues = profile.current.isAdmin?.()
			? ChatService.revealSingleOrRemoteMcpServer(mcpServerId, {
					dontLogErrors: true
				})
			: Promise.resolve<Record<string, string>>({});
		listK8sInfo =
			entity === 'workspace' && entityId
				? catalogEntry?.id
					? ChatService.getWorkspaceCatalogEntryServerK8sDetails(
							entityId,
							catalogEntry.id,
							mcpServerId
						)
					: ChatService.getWorkspaceK8sServerDetail(entityId, mcpServerId)
				: AdminService.getK8sServerDetail(mcpServerId);
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
			listK8sInfo =
				entity === 'workspace' && entityId
					? catalogEntry?.id
						? ChatService.getWorkspaceCatalogEntryServerK8sDetails(
								entityId,
								catalogEntry.id,
								mcpServerId
							)
						: ChatService.getWorkspaceK8sServerDetail(entityId, mcpServerId)
					: AdminService.getK8sServerDetail(mcpServerId);
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
			listK8sInfo =
				entity === 'workspace' && entityId
					? catalogEntry?.id
						? ChatService.getWorkspaceCatalogEntryServerK8sDetails(
								entityId,
								catalogEntry.id,
								mcpServerId
							)
						: ChatService.getWorkspaceK8sServerDetail(entityId, mcpServerId)
					: AdminService.getK8sServerDetail(mcpServerId);
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
				label: 'Kubernetes Deployment',
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
				envs.push({
					id: key,
					label: envMap.get(key)?.name ?? 'Unknown',
					value: revealedValues[key] ?? '',
					sensitive: envMap.get(key)?.sensitive || false
				});
			} else if (headerMap.has(key)) {
				headers.push({
					id: key,
					label: headerMap.get(key)?.name ?? 'Unknown',
					value: revealedValues[key] ?? '',
					sensitive: headerMap.get(key)?.sensitive || false
				});
			}
		}
		return {
			envs,
			headers
		};
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
	</div>

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
						{#each headers as h, i (h.id)}
							{@render configurationRow(h.label, h.value, h.sensitive)}
						{/each}
					</div>
				{:else}
					<span class="text-sm font-light text-gray-400 dark:text-gray-600"
						>No configured headers.</span
					>
				{/if}
			</div>
		{/if}

		<div>
			<h2 class="mb-2 text-lg font-semibold">Configuration</h2>
			{#if envs.length > 0}
				<div class="flex flex-col gap-2">
					{#each envs as env, i (env.id)}
						{@render configurationRow(env.label, env.value, env.sensitive)}
					{/each}
				</div>
			{:else}
				<span class="text-sm font-light text-gray-400 dark:text-gray-600"
					>No configured environment of file variables set.</span
				>
			{/if}
		</div>
	{/await}

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
			<span class="text-sm font-light text-gray-400 dark:text-gray-600">No events.</span>
		{/if}
	</div>
{:catch error}
	{@const isPending = error instanceof Error && error.message.includes('ContainerCreating')}
	<div class="flex flex-col gap-2">
		<div
			class="dark:bg-surface1 dark:border-surface3 flex flex-col rounded-lg border border-transparent bg-white p-4 shadow-sm"
		>
			<div class="grid grid-cols-2 gap-4">
				<p class="text-sm font-semibold">Status</p>
				<p class="text-sm font-light">{isPending ? 'Pending' : 'Error'}</p>
			</div>
		</div>
	</div>
{/await}

<div>
	<div class="mb-2 flex items-center gap-2">
		<h2 class="text-lg font-semibold">Deployment Logs</h2>
		<button
			onclick={handleRefreshLogs}
			class="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 disabled:opacity-50 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-300"
			disabled={refreshingLogs}
		>
			<RefreshCw class="size-4 {refreshingLogs ? 'animate-spin' : ''}" />
		</button>
		{#if error}
			<div
				use:tooltip={`An error occurred in connecting to the event stream. This is normal if the server is still starting up.`}
			>
				<AlertTriangle class="size-4 text-yellow-500" />
			</div>
		{/if}
	</div>
	<div
		bind:this={logsContainer}
		class="dark:bg-surface1 dark:border-surface3 default-scrollbar-thin flex max-h-84 min-h-64 flex-col overflow-y-auto rounded-lg border border-transparent bg-white p-4 shadow-sm"
	>
		{#if messages.length > 0}
			<div class="space-y-2">
				{#each messages as message, i (i)}
					<div class="font-mono text-sm" in:fade>
						<span class="text-gray-600 dark:text-gray-400">{message}</span>
					</div>
				{/each}
			</div>
		{:else}
			<span class="text-sm font-light text-gray-400 dark:text-gray-600">No deployment logs.</span>
		{/if}
	</div>
</div>

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
			{#if profile.current?.isAdmin?.() && isAdminUrl}
				{@const mcpId = d.mcpInstanceId ? d.mcpInstanceId : mcpServerId || mcpServerInstanceId}
				{@const id = mcpId?.split('-').at(-1)}
				{@const url =
					entity === 'workspace'
						? catalogEntry?.id
							? `/admin/mcp-servers/w/${entityId}/c/${catalogEntry.id}?view=audit-logs&userId=${d.id}`
							: `/admin/mcp-servers/w/${entityId}/s/${encodeURIComponent(id ?? '')}?view=audit-logs&userId=${d.id}`
						: `/admin/mcp-servers/s/${encodeURIComponent(id ?? '')}?view=audit-logs&userId=${d.id}`}
				<a href={url} class="button-text"> View Audit Logs </a>
			{/if}
		{/snippet}
	</Table>
</div>

{#snippet detailRow(label: string, value: string, id: string)}
	<div
		class="dark:bg-surface1 dark:border-surface3 flex flex-col rounded-lg border border-transparent bg-white p-4 shadow-sm"
	>
		<div class="grid grid-cols-12 gap-4">
			<p class="col-span-4 text-sm font-semibold">{label}</p>
			<div class="col-span-8 flex items-center justify-between">
				<p class="truncate text-sm font-light">{value}</p>
				{#if id === 'status' && !readonly}
					<button
						onclick={() => (showRestartConfirm = true)}
						class="flex items-center gap-2 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50"
						disabled={restarting}
					>
						<RotateCcw class="size-3" />
						Restart
					</button>
				{/if}
			</div>
		</div>
	</div>
{/snippet}

{#snippet configurationRow(label: string, value: string, sensitive?: boolean)}
	<div
		class="dark:bg-surface1 dark:border-surface3 flex flex-col rounded-lg border border-transparent bg-white px-4 py-1.5 shadow-sm"
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
	msg="Are you sure you want to restart this deployment? This will cause a brief service interruption."
	onsuccess={handleRestart}
	oncancel={() => (showRestartConfirm = false)}
	loading={restarting}
/>
