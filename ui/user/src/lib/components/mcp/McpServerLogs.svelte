<script lang="ts">
	import { EventStreamService } from '$lib/services/admin/eventstream.svelte';
	import { AlertTriangle, RefreshCw } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		mcpServerId: string;
		name?: string;
	}

	const { mcpServerId, name }: Props = $props();

	let messages = $state<string[]>([]);
	let error = $state<string>();
	let logsContainer: HTMLDivElement;
	let refreshingLogs = $state(false);

	const logsUrl = `/api/mcp-servers/${mcpServerId}/logs`;
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
		eventStream.connect(logsUrl, {
			onMessage: (data) => {
				messages = [...messages, data];
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

	async function handleRefreshLogs() {
		refreshingLogs = true;
		try {
			messages = [];
			eventStream.disconnect();
			eventStream.connect(logsUrl, {
				onMessage: (data) => {
					messages = [...messages, data];
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
</script>

<div>
	<div class="mb-2 flex items-center gap-2">
		<h2 class="text-lg font-semibold">Deployment Logs{name ? ` - ${name}` : ''}</h2>
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
		class="dark:bg-surface1 dark:border-surface3 default-scrollbar-thin bg-background flex max-h-84 min-h-64 flex-col overflow-y-auto rounded-lg border border-transparent p-4 shadow-sm"
	>
		{#if messages.length > 0}
			<div class="space-y-2">
				{#each messages as message, i (i)}
					<div class="font-mono text-sm" in:fade>
						<span class="text-on-surface1">{message}</span>
					</div>
				{/each}
			</div>
		{:else}
			<span class="text-on-surface1 text-sm font-light">No deployment logs.</span>
		{/if}
	</div>
</div>
