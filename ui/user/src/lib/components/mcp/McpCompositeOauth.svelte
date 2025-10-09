<script lang="ts">
	import {
		ChatService,
		type MCPCatalogServer
	} from '$lib/services';
	import { parseErrorContent } from '$lib/errors';
	import { Info, LoaderCircle, Server, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';

	interface Props {
		mcpID: string;
		oauthAuthRequestID: string;
		asDialog?: boolean;
		onComplete?: () => void;
	}

	let { mcpID, oauthAuthRequestID, asDialog = false, onComplete }: Props = $props();

	interface ChildServerAuth {
		id: string;
		name: string;
		icon?: string;
		oauthURL: string | null;
		authenticated: boolean;
		loading: boolean;
		error?: string;
	}

	let parentServer = $state<MCPCatalogServer | null>(null);
	let childServers = $state<ChildServerAuth[]>([]);
	let loading = $state(true);
	let error = $state<string>('');
	let dialog = $state<HTMLDialogElement>();
	let initializedListener = $state(false);

	const allAuthenticated = $derived(
		childServers.length > 0 && childServers.every((c) => c.authenticated)
	);

	async function loadServers() {
		loading = true;
		error = '';

		try {
			// Load all servers
			const servers = await ChatService.listSingleOrRemoteMcpServers();

			// Find parent server
			parentServer = servers.find((s) => s.id === mcpID) || null;

			if (!parentServer) {
				throw new Error('Composite MCP server not found');
			}

			// For composite servers, we need to check each potential child server
			// Since we can't access labels from the frontend MCPCatalogServer type,
			// we'll need to get this information differently. For now, let's check all servers
			// and collect those that need OAuth - this will be refined when we have a proper API
			const allServers = servers.filter((s) => s.id !== mcpID && s.manifest?.runtime === 'remote');

			// Initialize child server auth status by checking each server
			const childrenWithAuth = [];
			for (const server of allServers) {
				const authStatus = await checkChildAuthStatus(server.id);
				// Only include servers that actually need OAuth
				if (!authStatus.authenticated || authStatus.oauthURL) {
					childrenWithAuth.push({
						id: server.id,
						name: server.alias || server.manifest?.name || server.id,
						icon: server.manifest?.icon,
						oauthURL: authStatus.oauthURL,
						authenticated: authStatus.authenticated,
						loading: false
					});
				}
			}

			childServers = childrenWithAuth;
		} catch (err) {
			const { message } = parseErrorContent(err);
			error = message;
		} finally {
			loading = false;
		}
	}

	async function checkChildAuthStatus(
		childID: string
	): Promise<{ authenticated: boolean; oauthURL: string | null }> {
		try {
			// Try to get OAuth URL - if none needed, server is authenticated
			const url = await ChatService.getMcpServerOauthURL(childID);
			return { authenticated: !url, oauthURL: url || null };
		} catch (err) {
			// If error or no URL, assume authenticated
			return { authenticated: true, oauthURL: null };
		}
	}

	async function refreshChildStatus(childID: string) {
		const child = childServers.find((c) => c.id === childID);
		if (!child) return;

		child.loading = true;
		child.error = undefined;

		try {
			const status = await checkChildAuthStatus(childID);
			child.authenticated = status.authenticated;
			child.oauthURL = status.oauthURL;

			if (allAuthenticated) {
				// All children authenticated - notify completion
				onComplete?.();
				if (asDialog && dialog?.open) {
					dialog.close();
				}
			}
		} catch (err) {
			const { message } = parseErrorContent(err);
			child.error = message;
		} finally {
			child.loading = false;
		}
	}

	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			// User returned to page - refresh all non-authenticated children
			childServers.forEach((child) => {
				if (!child.authenticated) {
					refreshChildStatus(child.id);
				}
			});
		}
	}

	onMount(() => {
		loadServers();

		if (asDialog && dialog) {
			dialog.showModal();
		}

		document.addEventListener('visibilitychange', handleVisibilityChange);
		initializedListener = true;

		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	function handleClose() {
		if (asDialog && dialog?.open) {
			dialog.close();
		}
	}
</script>

{#if asDialog}
	<dialog
		bind:this={dialog}
		class="default-dialog w-full max-w-lg"
		use:dialogAnimation={{ type: 'fade' }}
	>
		<div class="flex flex-col gap-4 p-4">
			<div class="flex items-center justify-between gap-3">
				<div class="flex items-center gap-3">
					<div class="flex-shrink-0 rounded-md bg-surface1 p-2">
						{#if parentServer?.manifest?.icon}
							<img src={parentServer.manifest.icon} alt={parentServer.alias || 'MCP Server'} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<h3 class="default-dialog-title">
						{parentServer?.alias || parentServer?.manifest?.name || 'MCP Server Authentication'}
					</h3>
				</div>
				<button class="icon-button" onclick={handleClose}>
					<X class="size-5" />
				</button>
			</div>

			<p class="text-sm">
				This composite MCP server requires authentication with multiple services. Please
				authenticate with each service below.
			</p>

			{#if loading}
				<div class="flex items-center justify-center gap-2 py-4">
					<LoaderCircle class="size-4 animate-spin" />
					<span>Loading servers...</span>
				</div>
			{:else if error}
				<div class="notification-error">
					{error}
				</div>
			{:else}
				<div class="flex flex-col gap-3">
					{#each childServers as child (child.id)}
						<div class="flex items-center justify-between rounded-lg border border-surface3 bg-surface1 p-3">
							<div class="flex items-center gap-3">
								{#if child.icon}
									<img src={child.icon} alt={child.name} class="size-5" />
								{:else}
									<Server class="size-5" />
								{/if}
								<span class="font-medium">{child.name}</span>
							</div>

							{#if child.authenticated}
								<span class="text-sm text-green-600 dark:text-green-400">✓ Authenticated</span>
							{:else if child.loading}
								<div class="flex items-center gap-2 text-sm">
									<LoaderCircle class="size-4 animate-spin" />
									<span>Checking...</span>
								</div>
							{:else if child.error}
								<span class="text-sm text-red-600 dark:text-red-400">{child.error}</span>
							{:else if child.oauthURL}
								<a
									href={child.oauthURL}
									target="_blank"
									class="button-primary text-sm"
									onclick={() => {
										setTimeout(() => {
											child.loading = true;
										}, 500);
									}}
								>
									Authenticate
								</a>
							{:else}
								<button
									class="button-secondary text-sm"
									onclick={() => refreshChildStatus(child.id)}
								>
									Refresh
								</button>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			{#if allAuthenticated}
				<div class="notification-info">
					All services authenticated successfully!
				</div>
			{/if}
		</div>
	</dialog>
{:else}
	<!-- Standalone page mode -->
	<div class="colors-background flex min-h-screen items-center justify-center p-4">
		<div class="default-dialog w-full max-w-lg p-6">
			<div class="mb-6 flex items-center gap-3">
				<div class="flex-shrink-0 rounded-md bg-surface1 p-2">
					{#if parentServer?.manifest?.icon}
						<img src={parentServer.manifest.icon} alt={parentServer.alias || 'MCP Server'} class="size-8" />
					{:else}
						<Server class="size-8" />
					{/if}
				</div>
				<h1 class="text-2xl font-semibold">
					{parentServer?.alias || parentServer?.manifest?.name || 'MCP Server Authentication'}
				</h1>
			</div>

			<p class="mb-6 text-sm">
				This composite MCP server requires authentication with multiple services. Please
				authenticate with each service below.
			</p>

			{#if loading}
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
					{#each childServers as child (child.id)}
						<div class="flex items-center justify-between rounded-lg border border-surface3 bg-surface1 p-4">
							<div class="flex items-center gap-3">
								{#if child.icon}
									<img src={child.icon} alt={child.name} class="size-6" />
								{:else}
									<Server class="size-6" />
								{/if}
								<span class="text-base font-medium">{child.name}</span>
							</div>

							{#if child.authenticated}
								<span class="text-sm text-green-600 dark:text-green-400">✓ Authenticated</span>
							{:else if child.loading}
								<div class="flex items-center gap-2 text-sm">
									<LoaderCircle class="size-5 animate-spin" />
									<span>Checking...</span>
								</div>
							{:else if child.error}
								<span class="text-sm text-red-600 dark:text-red-400">{child.error}</span>
							{:else if child.oauthURL}
								<a
									href={child.oauthURL}
									target="_blank"
									class="button-primary"
									onclick={() => {
										setTimeout(() => {
											child.loading = true;
										}, 500);
									}}
								>
									Authenticate
								</a>
							{:else}
								<button class="button-secondary" onclick={() => refreshChildStatus(child.id)}>
									Refresh
								</button>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			{#if allAuthenticated}
				<div class="notification-info mt-6">
					<div class="flex flex-col gap-1">
						<p class="font-semibold">All services authenticated successfully!</p>
						<p class="text-sm">You can now close this page and return to your application.</p>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}
