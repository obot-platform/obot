<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { APIKey } from '$lib/services/api-keys/types';
	import type { MCPCatalogServer } from '$lib/services/chat/types';
	import { stripMarkdownToText } from '$lib/markdown';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { Server, Trash2 } from 'lucide-svelte';

	interface Props {
		apiKey?: APIKey & { prefix: string };
		mcpServers: MCPCatalogServer[];
		onClose: () => void;
		onDelete: (key: APIKey) => void;
		hideDelete?: boolean;
	}

	let { apiKey, mcpServers, onClose, onDelete, hideDelete }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	$effect(() => {
		if (apiKey) {
			dialog?.open();
		}
	});

	function handleClose() {
		onClose();
		dialog?.close();
	}

	let serverMap = $derived(new Map(mcpServers.map((s) => [s.id, s])));

	let isAllServers = $derived(apiKey?.mcpServerIds?.includes('*') ?? false);

	let resolvedServers = $derived.by(() => {
		if (!apiKey?.mcpServerIds || isAllServers) return [];
		return apiKey.mcpServerIds.map((id) => {
			const server = serverMap.get(id);
			return {
				id,
				name: server?.alias || server?.manifest.name || 'Deleted Server',
				description: server?.manifest.description,
				icon: server?.manifest.icon,
				exists: !!server
			};
		});
	});

	let createdDisplay = $derived(apiKey ? formatTimeAgo(apiKey.createdAt).relativeTime : '');
	let lastUsedDisplay = $derived(
		apiKey?.lastUsedAt ? formatTimeAgo(apiKey.lastUsedAt).relativeTime : 'Never'
	);
	let expiresDisplay = $derived(
		apiKey?.expiresAt ? formatTimeUntil(apiKey.expiresAt).relativeTime : 'Never'
	);
</script>

{#if apiKey}
	<ResponsiveDialog
		bind:this={dialog}
		onClose={handleClose}
		title={apiKey.name}
		class="w-full max-w-lg"
	>
		<div class="flex flex-col gap-4">
			{#if apiKey.description}
				<div>
					<p class="text-muted text-xs">Description</p>
					<p class="mt-1">{apiKey.description}</p>
				</div>
			{/if}

			<div>
				<p class="text-muted text-xs">Key</p>
				<p class="mt-1 font-mono text-sm">{apiKey.prefix}</p>
			</div>

			<div class="grid grid-cols-3 gap-4">
				<div>
					<p class="text-muted text-xs">Created</p>
					<p class="mt-1 text-sm">{createdDisplay}</p>
				</div>
				<div>
					<p class="text-muted text-xs">Last Used</p>
					<p class="mt-1 text-sm">{lastUsedDisplay}</p>
				</div>
				<div>
					<p class="text-muted text-xs">Expires</p>
					<p class="mt-1 text-sm">{expiresDisplay}</p>
				</div>
			</div>

			<div>
				<p class="text-muted text-xs">Authorized Servers</p>
				{#if isAllServers}
					<p class="mt-1 text-sm">All My Servers</p>
				{:else if resolvedServers.length === 0}
					<p class="text-muted mt-1 text-sm">No servers authorized</p>
				{:else}
					<div
						class="bg-surface1 default-scrollbar-thin mt-2 flex max-h-48 flex-col overflow-y-auto rounded-lg"
					>
						{#each resolvedServers as server (server.id)}
							<div class="flex w-full items-center gap-3 px-3 py-2.5">
								<div class="flex-shrink-0">
									{#if server.icon}
										<img src={server.icon} alt={server.name} class="size-6" />
									{:else}
										<Server class="text-on-surface1 size-6" />
									{/if}
								</div>
								<div class="flex min-w-0 grow flex-col">
									<p class="truncate text-sm" class:text-muted={!server.exists}>
										{server.name}
									</p>
									{#if server.description}
										<span class="text-on-surface1 line-clamp-1 text-xs">
											{@html stripMarkdownToText(server.description)}
										</span>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>

		{#if !hideDelete}
			<div class="mt-6 flex justify-end border-t pt-4">
				<button
					class="button-destructive flex items-center gap-2"
					onclick={() => {
						handleClose();
						onDelete(apiKey);
					}}
				>
					<Trash2 class="size-4" />
					Delete API Key
				</button>
			</div>
		{/if}
	</ResponsiveDialog>
{/if}
