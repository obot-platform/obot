<script lang="ts">
	import Table from '$lib/components/table/Table.svelte';
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
			<StepForward class="size-4" />
		</button>
	{/snippet}
</Table>
