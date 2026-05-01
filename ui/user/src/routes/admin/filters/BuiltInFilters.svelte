<script lang="ts">
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
			<button class="icon-button hover:dark:bg-background/50">
				<StepForward class="size-4 text-on-surface1" />
			</button>
		{/snippet}
	</Table>
{:else if filteredBuiltInFiltersData.length > 0}
	<div class="p-4 md:p-0">
		<button
			class="p-4 rounded-sm flex items-center gap-6 justify-between bg-background dark:bg-surface1 shadow-md transition-colors duration-300 hover:bg-surface2 dark:hover:bg-surface2"
			onclick={() => onSelect?.(filteredBuiltInFiltersData[0])}
		>
			<p class="flex flex-col gap-1 text-left">
				<span class="text-base font-semibold">{filteredBuiltInFiltersData[0].name}</span>
				<span class="text-sm font-light text-on-surface1 line-clamp-3">
					{stripMarkdownToText(filteredBuiltInFiltersData[0].manifest.description)}
				</span>
			</p>
			<StepForward class="size-4 shrink-0 text-on-surface1" />
		</button>
	</div>

	<div class="text-on-surface1 text-sm font-light mt-4 text-center italic">More Coming Soon!</div>
{/if}
