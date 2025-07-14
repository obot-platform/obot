<script lang="ts">
	import { AdminService, type K8sServerDetail } from '$lib/services';
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { AlertTriangle, Info, LoaderCircle } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import Table from '../Table.svelte';
	import { fade } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		mcpServerId: string;
		name: string;
		mcpServerInstanceId?: string;
	}

	const { mcpServerId, mcpServerInstanceId, name }: Props = $props();

	let listK8sInfo = $state<Promise<K8sServerDetail>>();
	let messages = $state<string[]>([]);
	let error = $state<string>();
	let logsContainer: HTMLDivElement;

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
		listK8sInfo = AdminService.getK8sServerDetail(mcpServerId);
		eventStream.connect(`/api/mcp-servers/${mcpServerId}/logs`, {
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
</script>

<h1 class="text-2xl font-semibold">
	{#if mcpServerInstanceId}
		{name} | {mcpServerInstanceId}
	{:else}
		{name}
	{/if}
</h1>

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
			<div
				class="dark:bg-surface1 dark:border-surface3 flex flex-col rounded-lg border border-transparent bg-white p-4 shadow-sm"
			>
				<div class="grid grid-cols-12 gap-4">
					<p class="col-span-4 text-sm font-semibold">{detail.label}</p>
					<p class="col-span-8 truncate text-sm font-light">{detail.value}</p>
				</div>
			</div>
		{/each}
	</div>

	<div>
		<h2 class="mb-2 text-lg font-semibold">Events</h2>
		{#if info?.events && info.events.length > 0}
			{@const tableData = info.events.map((event) => ({
				id: event.time,
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
		<h2 class=" text-lg font-semibold">Deployment Logs</h2>
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
