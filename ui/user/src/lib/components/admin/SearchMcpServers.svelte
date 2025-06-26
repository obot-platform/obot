<script lang="ts">
	import { type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import { Check, LoaderCircle, Server } from 'lucide-svelte';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { getAdminMcpServerAndEntries } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onAdd: (mcpCatalogEntryIds: string[], mcpServerIds: string[]) => void;
	}

	let { onAdd }: Props = $props();
	let addMcpServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');
	let selected = $state<(MCPCatalogEntry | MCPCatalogServer)[]>([]);
	let selectedMap = $derived(new Set(selected.map((i) => i.id)));
	const mcpServerAndEntries = getAdminMcpServerAndEntries();

	let loading = $state(false);
	let allData = $derived(
		search
			? [...mcpServerAndEntries.entries, ...mcpServerAndEntries.servers].filter((item) => {
					const name =
						'manifest' in item
							? item.manifest?.name
							: item.commandManifest?.name || item.urlManifest?.name;
					return name?.toLowerCase().includes(search.toLowerCase());
				})
			: [...mcpServerAndEntries.entries, ...mcpServerAndEntries.servers]
	);

	export function open() {
		addMcpServerDialog?.open();
	}

	function onClose() {
		search = '';
		selected = [];
	}

	function handleAdd() {
		const mcpServerIds = [];
		const mcpCatalogEntryIds = [];
		for (const item of selected) {
			if ('manifest' in item) {
				mcpServerIds.push(item.id);
			} else {
				mcpCatalogEntryIds.push(item.id);
			}
		}
		onAdd(mcpCatalogEntryIds, mcpServerIds);
		addMcpServerDialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={addMcpServerDialog}
	{onClose}
	title="Add MCP Server(s)"
	class="h-full w-full overflow-visible p-0 md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="flex flex-col gap-2">
			{#if loading}
				<div class="flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else}
				<div class="px-4">
					<Search
						class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
						onChange={(val) => (search = val)}
						placeholder="Search by name..."
					/>
				</div>

				<div class="flex flex-col">
					{#each allData as item}
						<button
							class={twMerge(
								'dark:hover:bg-surface1 hover:bg-surface2 flex w-full items-center gap-2 px-4 py-2 text-left',
								selectedMap.has(item.id) && 'dark:bg-gray-920 bg-gray-50'
							)}
							onclick={() => {
								if (selectedMap.has(item.id)) {
									const index = selected.findIndex((i) => i.id === item.id);
									if (index !== -1) {
										selected.splice(index, 1);
									}
								} else {
									selected.push(item);
								}
							}}
						>
							<div class="flex w-full items-center gap-2 overflow-hidden">
								{#if 'manifest' in item}
									{#if item.manifest.icon}
										<img
											src={item.manifest.icon}
											alt={item.manifest.name}
											class="size-6 flex-shrink-0"
										/>
									{:else}
										<Server class="size-6 flex-shrink-0" />
									{/if}
									<div class="flex min-w-0 grow flex-col">
										<p class="truncate">{item.manifest.name}</p>
										<span class="truncate text-xs text-gray-500">{item.manifest.description}</span>
									</div>
								{:else}
									{@const icon = item.commandManifest?.icon || item.urlManifest?.icon}
									{#if icon}
										<img
											src={icon}
											alt={item.commandManifest?.name || item.urlManifest?.name}
											class="size-6 flex-shrink-0"
										/>
									{:else}
										<Server class="size-6 flex-shrink-0" />
									{/if}
									<div class="flex min-w-0 grow flex-col">
										<p class="truncate">
											{item.commandManifest?.name || item.urlManifest?.name}
										</p>
										<span class="truncate text-xs font-light text-gray-500">
											{item.commandManifest?.description || item.urlManifest?.description}
										</span>
									</div>
								{/if}
							</div>
							<div class="flex size-6 items-center justify-center">
								{#if selectedMap.has(item.id)}
									<Check class="size-6 text-blue-500" />
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>
	<div class="flex w-full flex-col justify-between gap-4 p-4 md:flex-row">
		<div class="flex items-center gap-1 font-light">
			{#if selected.length > 0}
				<Server class="size-4" />
				{selected.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<button class="button w-full md:w-fit" onclick={() => addMcpServerDialog?.close()}>
				Cancel
			</button>
			<button class="button-primary w-full md:w-fit" onclick={handleAdd}> Confirm </button>
		</div>
	</div>
</ResponsiveDialog>
