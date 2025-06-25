<script lang="ts">
	import { ChatService, type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import { LoaderCircle, Server } from 'lucide-svelte';
	import Search from '../Search.svelte';

	interface Props {
		onSelect: () => void;
	}

	let { onSelect }: Props = $props();
	let search = $state('');
	let mcpEntries = $state<MCPCatalogEntry[]>([]);
	let mcpServers = $state<MCPCatalogServer[]>([]);
	let loading = $state(false);
	let allData = $derived([...mcpEntries, ...mcpServers]);

	async function loadData() {
		loading = true;
		mcpEntries = await ChatService.listMCPs();
		mcpServers = await ChatService.listMCPCatalogServers();
		loading = false;
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading}
		<div class="flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else}
		<Search
			class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
			onChange={(val) => (search = val)}
			placeholder="Search by name..."
		/>

		<div class="flex flex-col">
			{#each allData as item}
				<button onclick={() => console.log(item)} class="flex items-center gap-2">
					{#if 'manifest' in item}
						{#if item.manifest.icon}
							<img src={item.manifest.icon} alt={item.manifest.name} class="size-4" />
						{:else}
							<Server class="size-4" />
						{/if}
						{item.manifest.name}
					{:else}
						{@const icon = item.commandManifest?.icon || item.urlManifest?.icon}
						{#if icon}
							<img
								src={icon}
								alt={item.commandManifest?.name || item.urlManifest?.name}
								class="size-4"
							/>
						{:else}
							<Server class="size-4" />
						{/if}
						{item.commandManifest?.name || item.urlManifest?.name}
					{/if}
				</button>
			{/each}
		</div>
	{/if}
</div>
