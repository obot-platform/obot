<script lang="ts">
	import DatePicker from '$lib/components/DatePicker.svelte';
	import Search from '$lib/components/Search.svelte';
	import { ApiKeysService } from '$lib/services';
	import type { APIKeyCreateResponse } from '$lib/services/api-keys/types';
	import type { MCPCatalogServer } from '$lib/services/chat/types';
	import { stripMarkdownToText } from '$lib/markdown';
	import { Check, LoaderCircle, Server } from 'lucide-svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';
	import { fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';

	interface Props {
		mcpServers: MCPCatalogServer[];
		onCreate: (key: APIKeyCreateResponse) => void;
		onCancel: () => void;
	}

	let { mcpServers, onCreate, onCancel }: Props = $props();

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
		} finally {
			loading = false;
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div
		class="dark:bg-surface2 dark:border-surface3 bg-background rounded-lg border border-transparent p-4"
	>
		<div class="flex flex-col gap-6">
			<div class="flex flex-col gap-2">
				<label for="api-key-name" class="input-label">
					Name
					{#if nameError}
						<span class="text-xs text-red-600 dark:text-red-400">Name is required</span>
					{/if}
				</label>
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
		</div>
	</div>

	<div class="mt-4 flex flex-col gap-2">
		<p>
			<span class="text-lg font-semibold">MCP Servers</span>
			{#if serverError}
				<span class="text-xs text-red-600 dark:text-red-400">Select at least one server</span>
			{/if}
		</p>
		<p class="input-description">
			Select which MCP servers this API key can access
			{#if selectedServerIds.size > 0}
				<span class="italic">
					({#if selectedServerIds.has('*')}All Selected{:else}{selectedServerIds.size} Selected{/if})
				</span>
			{/if}
		</p>

		<Search
			class="text-input-filled"
			onChange={(val) => (search = val)}
			value={search}
			placeholder="Search servers..."
		/>

		<div
			class={twMerge(
				'bg-surface1 default-scrollbar-thin flex max-h-64 flex-col overflow-y-auto rounded-lg',
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
	</div>

	<div class="flex grow"></div>

	<div
		class="bg-surface1 dark:bg-background dark:text-on-surface1 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400"
		out:fly={{ x: -100, duration }}
		in:fly={{ x: -100 }}
	>
		<div class="flex w-full justify-end gap-2">
			<button class="button text-sm" onclick={onCancel}>Cancel</button>
			<button class="button-primary text-sm" disabled={loading} onclick={handleCreate}>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Create API Key
				{/if}
			</button>
		</div>
	</div>
</div>
