<script lang="ts">
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import { type SystemMCPServerCatalogEntry } from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { StepForward } from 'lucide-svelte';

	interface Props {
		query: string;
		entries: SystemMCPServerCatalogEntry[];
		onSelect?: (entry: SystemMCPServerCatalogEntry) => void;
	}

	let { query, entries: systemCatalogEntries, onSelect }: Props = $props();
	let builtInFiltersData = $derived(
		systemCatalogEntries.map((entry) => {
			return {
				...entry,
				name: entry.manifest.name
			};
		})
	);
	let filteredBuiltInFiltersData = $derived.by(() =>
		builtInFiltersData.filter((entry) =>
			entry.manifest.name?.toLowerCase().includes(query.toLowerCase())
		)
	);
</script>

{#if filteredBuiltInFiltersData.length > 1}
	<Table
		data={filteredBuiltInFiltersData}
		fields={['name', 'created']}
		filterable={['name']}
		onClickRow={(d) => {
			onSelect?.(d);
		}}
		sortable={['name', 'status', 'type', 'created']}
		noDataMessage="No built-in servers available."
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex shrink-0 items-center gap-2">
					<p class="flex items-center gap-2">
						{d.name}
					</p>
				</div>
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{:else}
				{d[property as keyof typeof d]}
			{/if}
		{/snippet}
		{#snippet actions()}
			<IconButton class="hover:dark:bg-base-100/50">
				<StepForward class="size-4 text-muted-content" />
			</IconButton>
		{/snippet}
	</Table>
{:else if filteredBuiltInFiltersData.length > 0}
	<div class="p-4 md:p-0">
		<button
			class="p-4 rounded-sm flex items-center gap-6 justify-between bg-base-100 dark:bg-base-200 shadow-md transition-colors duration-300 hover:bg-base-300 dark:hover:bg-base-300"
			onclick={() => onSelect?.(filteredBuiltInFiltersData[0])}
		>
			<p class="flex flex-col gap-1 text-left">
				<span class="text-base font-semibold">{filteredBuiltInFiltersData[0].name}</span>
				<span class="text-sm font-light text-muted-content line-clamp-3">
					{stripMarkdownToText(filteredBuiltInFiltersData[0].manifest.description)}
				</span>
			</p>
			<StepForward class="size-4 shrink-0 text-muted-content" />
		</button>
	</div>

	<div class="text-muted-content text-sm font-light mt-4 text-center italic">More Coming Soon!</div>
{/if}
