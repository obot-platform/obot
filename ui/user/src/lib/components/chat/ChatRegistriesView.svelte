<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { type MCPCatalogEntry, type MCPCatalogServer, type OrgUser } from '$lib/services';
	import {
		convertEntriesAndServersToTableData,
		getServerTypeLabelByType
	} from '$lib/services/chat/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { CircleFadingArrowUp, LoaderCircle, Server, StepForward } from 'lucide-svelte';

	interface Props {
		usersMap?: Map<string, OrgUser>;
		query?: string;
		classes?: {
			tableHeader?: string;
		};
		onSelect: (item: MCPCatalogEntry | MCPCatalogServer) => void;
		onConnect: (item: MCPCatalogEntry | MCPCatalogServer) => void;
	}

	let { query, onSelect, onConnect }: Props = $props();

	let tableData = $derived(
		convertEntriesAndServersToTableData(
			mcpServersAndEntries.current.entries,
			mcpServersAndEntries.current.servers
		)
	);

	let filteredTableData = $derived.by(() => {
		const sorted = tableData.sort((a, b) => {
			return a.name.localeCompare(b.name);
		});
		return query
			? sorted.filter(
					(d) =>
						d.name.toLowerCase().includes(query.toLowerCase()) ||
						d.registry.toLowerCase().includes(query.toLowerCase())
				)
			: sorted;
	});
</script>

<div class="flex flex-col gap-2">
	{#if mcpServersAndEntries.current.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else}
		<Table
			data={filteredTableData}
			classes={{
				root: 'rounded-none rounded-b-md shadow-none'
			}}
			fields={['name', 'type', 'users', 'created', 'registry']}
			filterable={['name', 'type', 'registry']}
			onClickRow={(d) => onSelect?.(d.data)}
			sortable={['name', 'type', 'users', 'created', 'registry']}
			noDataMessage="No catalog servers added."
			setRowClasses={(d) => ('needsUpdate' in d && d.needsUpdate ? 'bg-primary/10' : '')}
			disablePortal
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'name'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div class="icon">
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-2">
							{d.name}
							{#if 'needsUpdate' in d && d.needsUpdate}
								<span
									use:tooltip={{
										classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
										text: 'An update requires your attention'
									}}
								>
									<CircleFadingArrowUp class="text-primary size-4" />
								</span>
							{/if}
						</p>
					</div>
				{:else if property === 'type'}
					{getServerTypeLabelByType(d.type)}
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<button
					class="icon-button hover:dark:bg-background/50"
					onclick={(e) => {
						e.stopPropagation();
						onConnect?.(d.data);
					}}
				>
					<StepForward class="size-4" />
				</button>
			{/snippet}
		</Table>
	{/if}
</div>
