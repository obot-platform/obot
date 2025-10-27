<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { LoaderCircle, Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { ChatService, type MCPCatalogServer } from '$lib/services';

	interface Props {
		compositeMcpId: string;
		oauthAuthRequestId?: string;
		onComplete?: () => void;
	}

	let { compositeMcpId, oauthAuthRequestId, onComplete }: Props = $props();

	type PendingItem = {
		mcpServerID: string;
		catalogEntryID?: string;
		authURL: string;
		loading?: boolean;
	};

	let compositeServer = $state<MCPCatalogServer>();
	let componentInfos = $state<Record<string, { name?: string; icon?: string }>>({});
	let pending = $state<PendingItem[]>([]);
	let loading = $state(true);
	let error = $state<string>('');

	const allAuthenticated = $derived(pending.length === 0);

	// trigger onComplete when done
	$effect(() => {
		if (onComplete && allAuthenticated && !loading && !error) {
			onComplete();
		}
	});

	async function fetchParentAndMeta() {
		try {
			compositeServer = await ChatService.getSingleOrRemoteMcpServer(compositeMcpId);

			const componentServers = compositeServer?.manifest?.compositeConfig?.componentServers || [];
			componentInfos = componentServers.reduce(
				(
					acc: Record<string, { name?: string; icon?: string }>,
					c: { catalogEntryID: string; manifest?: { name?: string; icon?: string } }
				) => {
					acc[c.catalogEntryID] = {
						name: c.manifest?.name,
						icon: c.manifest?.icon
					};
					return acc;
				},
				{}
			);
		} catch (_err) {
			// ignore; UI will fallback to IDs
		}
	}

	async function fetchPending() {
		loading = true;
		error = '';
		try {
			const data = await ChatService.checkCompositeOAuth(compositeMcpId, {
				oauthAuthRequestID: oauthAuthRequestId
			});
			pending = (data as PendingItem[]).map((d) => ({ ...d }));
		} catch (_err) {
			const { message } = parseErrorContent(_err);
			error = message;
		} finally {
			loading = false;
		}
	}

	function setItemLoading(id: string, value: boolean) {
		pending = pending.map((p) => (p.mcpServerID === id ? { ...p, loading: value } : p));
	}

	async function skip(id: string) {
		setItemLoading(id, true);
		try {
			const item = pending.find((p) => p.mcpServerID === id);
			if (!item || !item.catalogEntryID) return;

			// Use configure endpoint to set disabled=true for this component
			const payload: Record<string, { config: Record<string, string>; disabled: boolean }> = {
				[item.catalogEntryID]: { config: {}, disabled: true }
			};
			await ChatService.configureCompositeMcpServer(compositeMcpId, payload);

			// Re-check pending from server; item should disappear
			await fetchPending();
		} catch (err) {
			const { message } = parseErrorContent(err);
			error = message;
		} finally {
			setItemLoading(id, false);
		}
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			fetchPending();
		}
	}

	onMount(() => {
		fetchParentAndMeta();
		fetchPending();
		document.addEventListener('visibilitychange', handleVisibilityChange);
		return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
	});
</script>

<div class="colors-background flex min-h-screen items-center justify-center p-4">
	<div class="default-dialog w-full max-w-lg p-6">
		<div class="mb-6 flex items-center gap-3">
			<div class="bg-surface1 flex-shrink-0 rounded-md p-2">
				{#if compositeServer?.manifest?.icon}
					<img
						src={compositeServer.manifest.icon}
						alt={compositeServer.alias || 'MCP Server'}
						class="size-8"
					/>
				{:else}
					<Server class="size-8" />
				{/if}
			</div>
			<h1 class="text-2xl font-semibold">
				{compositeServer?.alias || compositeServer?.manifest?.name || 'MCP Server Authentication'}
			</h1>
		</div>

		<p class="mb-6 text-sm">
			This composite MCP server requires authentication with multiple services. Please authenticate
			with each service below.
		</p>

		{#if loading && pending.length === 0}
			<div class="flex items-center justify-center gap-2 py-8">
				<LoaderCircle class="size-6 animate-spin" />
				<span>Loading servers...</span>
			</div>
		{:else if error}
			<div class="notification-error">
				{error}
			</div>
		{:else}
			<div class="flex flex-col gap-4">
				{#each pending as item (item.mcpServerID)}
					<div
						class="border-surface3 bg-surface1 flex items-center justify-between rounded-lg border p-4"
					>
						<div class="flex items-center gap-3">
							{#if componentInfos[item.catalogEntryID || '']?.icon}
								<img
									src={componentInfos[item.catalogEntryID || '']?.icon}
									alt="icon"
									class="size-6"
								/>
							{:else}
								<Server class="size-6" />
							{/if}
							<span class="text-base font-medium"
								>{componentInfos[item.catalogEntryID || '']?.name ||
									item.catalogEntryID ||
									item.mcpServerID}</span
							>
						</div>
						<div class="flex items-center gap-2">
							<a href={item.authURL} target="_blank" class="button-primary">Authenticate</a>
							<button
								class="button-text"
								disabled={item.loading}
								onclick={() => skip(item.mcpServerID)}
							>
								{#if item.loading}
									<LoaderCircle class="size-4 animate-spin" />
								{:else}
									Skip
								{/if}
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}

		{#if allAuthenticated}
			<div class="notification-info mt-6">
				<div class="flex flex-col gap-2">
					<p class="font-semibold">All services authenticated successfully!</p>
				</div>
			</div>
		{/if}
	</div>
</div>
