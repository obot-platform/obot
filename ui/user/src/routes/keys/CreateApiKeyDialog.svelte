<script lang="ts">
	import DatePicker from '$lib/components/DatePicker.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import { ApiKeysService } from '$lib/services';
	import type { APIKeyCreateResponse } from '$lib/services/api-keys/types';
	import type { MCPCatalogServer } from '$lib/services/chat/types';
	import { stripMarkdownToText } from '$lib/markdown';
	import { Check, LoaderCircle, Server } from 'lucide-svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		show: boolean;
		mcpServers: MCPCatalogServer[];
		onCreate: (key: APIKeyCreateResponse) => void;
	}

	let { show = $bindable(), mcpServers, onCreate }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let name = $state('');
	let description = $state('');
	let expiresAt = $state<Date | null>(null);
	let selectedServerIds = new SvelteSet<string>();
	let search = $state('');
	let loading = $state(false);
	let showValidation = $state(false);

	let nameError = $derived(showValidation && !name.trim());
	let serverError = $derived(showValidation && selectedServerIds.size === 0);

	const allServersOption = {
		id: '*',
		manifest: {
			name: 'All MCP Servers',
			description: 'Grant access to all MCP servers, including any added in the future'
		}
	} as MCPCatalogServer;

	function getServerDisplayName(server: MCPCatalogServer): string {
		return server.alias || server.manifest.name || '';
	}

	let filteredServers = $derived.by(() => {
		const searchLower = search.toLowerCase();
		const servers = search
			? mcpServers.filter((s) => getServerDisplayName(s).toLowerCase().includes(searchLower))
			: mcpServers;

		// Include "All MCP Servers" option if it matches the search or there's no search
		const allServersMatches = !search || 'all mcp servers'.includes(searchLower);

		return allServersMatches ? [allServersOption, ...servers] : servers;
	});

	$effect(() => {
		if (show) {
			resetForm();
			dialog?.open();
		}
	});

	function resetForm() {
		name = '';
		description = '';
		expiresAt = null;
		selectedServerIds.clear();
		search = '';
		showValidation = false;
	}

	function handleClose() {
		show = false;
		dialog?.close();
	}

	function toggleServer(serverId: string) {
		if (selectedServerIds.has(serverId)) {
			selectedServerIds.delete(serverId);
		} else {
			// If selecting "All MCP Servers", clear other selections
			if (serverId === '*') {
				selectedServerIds.clear();
			} else {
				// If selecting a specific server, remove "All MCP Servers" if selected
				selectedServerIds.delete('*');
			}
			selectedServerIds.add(serverId);
		}
	}

	async function handleCreate() {
		showValidation = true;
		if (!name.trim() || selectedServerIds.size === 0) {
			return;
		}

		loading = true;
		try {
			const response = await ApiKeysService.createApiKey({
				name: name.trim(),
				description: description.trim() || undefined,
				expiresAt: expiresAt?.toISOString(),
				mcpServerIds: Array.from(selectedServerIds)
			});
			onCreate(response);
			handleClose();
		} finally {
			loading = false;
		}
	}
</script>

{#if show}
	<ResponsiveDialog
		bind:this={dialog}
		onClose={handleClose}
		title="Create API Key"
		class="h-full w-full overflow-visible md:h-auto md:max-h-[90vh] md:max-w-xl"
		classes={{ content: 'min-h-0 flex-1' }}
	>
		<div class="default-scrollbar-thin flex flex-1 flex-col gap-6 overflow-y-auto">
			<div class="flex flex-col gap-2">
				<label for="api-key-name" class="input-label">Name</label>
				<input
					id="api-key-name"
					type="text"
					bind:value={name}
					placeholder="My API Key"
					class={twMerge(
						'text-input-filled',
						nameError && 'border-red-500 focus:border-red-500 focus:ring-red-500'
					)}
				/>
				{#if nameError}
					<p class="text-xs text-red-600 dark:text-red-400">Name is required</p>
				{/if}
			</div>

			<div class="flex flex-col gap-2">
				<label for="api-key-description" class="input-label">Description (Optional)</label>
				<input
					id="api-key-description"
					type="text"
					bind:value={description}
					placeholder="What is this API key for?"
					class="text-input-filled"
				/>
			</div>

			<div class="flex flex-col gap-2">
				<label for="api-key-expires" class="input-label">Expiration Date (Optional)</label>
				<DatePicker
					id="api-key-expires"
					bind:value={expiresAt}
					onChange={(date) => (expiresAt = date)}
					placeholder="No expiration"
					minDate={new Date()}
				/>
				<p class="input-description">Leave empty for no expiration</p>
			</div>

			<div class="flex flex-col gap-2">
				<label class="input-label">MCP Servers</label>
				<p class="input-description">Select which MCP servers this API key can access</p>
				{#if serverError}
					<p class="text-xs text-red-600 dark:text-red-400">Select at least one server</p>
				{/if}

				<Search
					class="text-input-filled"
					onChange={(val) => (search = val)}
					value={search}
					placeholder="Search servers..."
				/>

				<div
					class={twMerge(
						'bg-surface1 default-scrollbar-thin flex max-h-48 flex-col overflow-y-auto rounded-lg',
						serverError && 'ring-1 ring-red-500'
					)}
				>
					{#if filteredServers.length === 0}
						<div class="text-on-surface1 flex items-center justify-center py-8 text-sm">
							{search ? 'No servers match your search' : 'No MCP servers available'}
						</div>
					{:else}
						{#each filteredServers as server (server.id)}
							<button
								type="button"
								class={twMerge(
									'hover:bg-surface2 flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors',
									selectedServerIds.has(server.id) && 'bg-surface2'
								)}
								onclick={() => toggleServer(server.id)}
							>
								<div class="flex w-full items-center gap-3 overflow-hidden">
									<div class="flex-shrink-0">
										{#if server.manifest.icon}
											<img
												src={server.manifest.icon}
												alt={getServerDisplayName(server)}
												class="size-6"
											/>
										{:else}
											<Server class="text-on-surface1 size-6" />
										{/if}
									</div>
									<div class="flex min-w-0 grow flex-col">
										<p class="truncate text-sm">{getServerDisplayName(server)}</p>
										{#if server.manifest.description}
											<span class="text-on-surface1 line-clamp-1 text-xs">
												{@html stripMarkdownToText(server.manifest.description)}
											</span>
										{/if}
									</div>
								</div>
								<div class="flex size-5 flex-shrink-0 items-center justify-center">
									{#if selectedServerIds.has(server.id)}
										<Check class="text-primary size-5" />
									{/if}
								</div>
							</button>
						{/each}
					{/if}
				</div>

				{#if selectedServerIds.size > 0}
					<p class="input-description">
						{#if selectedServerIds.has('*')}
							All servers selected
						{:else}
							{selectedServerIds.size} server{selectedServerIds.size === 1 ? '' : 's'} selected
						{/if}
					</p>
				{/if}
			</div>
		</div>

		<div class="mt-6 flex flex-shrink-0 flex-col justify-end gap-2 md:flex-row">
			<button class="button" onclick={handleClose}>Cancel</button>
			<button class="button-primary" onclick={handleCreate} disabled={loading}>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{/if}
				Create API Key
			</button>
		</div>
	</ResponsiveDialog>
{/if}
