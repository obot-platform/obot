<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import { DEFAULT_SYSTEM_MCP_CATALOG_ID } from '$lib/constants';
	import {
		AdminService,
		type MCPFilter,
		type SystemMCPServer,
		type SystemMCPServerCatalogEntry
	} from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { openUrl } from '$lib/utils';
	import EnableFilter from './EnableFilter.svelte';
	import { Captions, Ellipsis, LightbulbIcon, LightbulbOffIcon } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		query: string;
		connectedFilters?: MCPFilter[];
		urlFilters?: Record<string, (string | number)[]>;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onRefresh?: () => void;
		onSort?: InitSortFn;
		initSort?: InitSort;
	}

	let {
		query,
		connectedFilters,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		onRefresh,
		initSort
	}: Props = $props();

	let systemCatalogLoading = $state(true);
	let systemCatalogEntries = $state<SystemMCPServerCatalogEntry[]>([]);
	let systemCatalogServers = $state<SystemMCPServer[]>([]);
	let enableFilterDialog = $state<ReturnType<typeof EnableFilter>>();

	let connectedFiltersMap = $derived(
		new Map(connectedFilters?.map((filter) => [filter.systemMCPServerCatalogEntryID, filter]))
	);
	let serversMap = $derived(
		new Map(systemCatalogServers.map((server) => [server.id.slice(4), server]))
	);

	let builtInFiltersData = $derived(
		systemCatalogEntries.map((entry) => {
			const matchingFilter = connectedFiltersMap.get(entry.id);
			const matchingServer = matchingFilter && serversMap.get(matchingFilter.id);
			return {
				...entry,
				name: entry.manifest.name,
				status: matchingFilter ? 'Enabled' : 'Disabled',
				filter: matchingFilter,
				server: matchingServer
			};
		})
	);
	let filteredBuiltInFiltersData = $derived.by(() =>
		builtInFiltersData.filter((entry) =>
			entry.manifest.name?.toLowerCase().includes(query.toLowerCase())
		)
	);

	let deleteBuiltInFilter = $state<(typeof builtInFiltersData)[number]>();
	let deleting = $state(false);

	onMount(() => {
		Promise.all([
			AdminService.listSystemMCPCatalogEntries(DEFAULT_SYSTEM_MCP_CATALOG_ID),
			AdminService.listSystemMCPServers()
		])
			.then(([entries, servers]) => {
				systemCatalogEntries = entries;
				systemCatalogServers = servers;
			})
			.finally(() => {
				systemCatalogLoading = false;
			});
	});
</script>

{#if systemCatalogLoading}
	<div class="w-full">
		<div class="w-full h-14 bg-surface3 animate-pulse"></div>
		<div class="w-full h-14 bg-surface3 animate-pulse"></div>
		<div class="w-full h-14 bg-surface3 animate-pulse"></div>
	</div>
{:else}
	<Table
		data={filteredBuiltInFiltersData}
		fields={['name', 'status', 'created']}
		filterable={['name', 'status']}
		{filters}
		onClickRow={(d, isCtrlClick) => {
			const url = d.filter
				? `/admin/filters/c/${d.id}/instance/${d.filter.id}`
				: `/admin/filters/c/${d.id}`;
			openUrl(url, isCtrlClick);
		}}
		{initSort}
		{onFilter}
		{onClearAllFilters}
		{onSort}
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
			{:else if property === 'status'}
				{#if d.status}
					<div
						class={twMerge(
							'pill-primary',
							d.status === 'Enabled'
								? 'bg-primary'
								: 'bg-surface2 dark:bg-surface3 text-on-background/30 italic'
						)}
					>
						{d.status}
					</div>
				{/if}
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{:else}
				{d[property as keyof typeof d]}
			{/if}
		{/snippet}
		{#snippet actions(d)}
			<DotDotDot class="icon-button hover:dark:bg-background/50" classes={{ menu: 'p-0' }}>
				{#snippet icon()}
					<Ellipsis class="size-4" />
				{/snippet}

				{#snippet children({ toggle })}
					<div class="flex flex-col gap-1 p-2">
						<button
							class="menu-button disabled:cursor-not-allowed disabled:opacity-50"
							onclick={async (e) => {
								e.stopPropagation();
								toggle(false);

								if (d.status === 'Enabled') {
									deleteBuiltInFilter = d;
								} else {
									enableFilterDialog?.open({
										server: d.server,
										entry: d
									});
								}
							}}
						>
							{#if d.status === 'Enabled'}
								<LightbulbOffIcon class="size-4" /> Disable Filter
							{:else}
								<LightbulbIcon class="size-4" /> Enable Filter
							{/if}
						</button>
						{#if d.status === 'Enabled'}
							<button
								onclick={(e) => {
									if (!d.filter) return;
									e.stopPropagation();
									const isCtrlClick = e.ctrlKey || e.metaKey;
									const url = `/admin/filters/c/${d.id}/instance/${d.filter.id}?view=audit-logs`;
									openUrl(url, isCtrlClick);
								}}
								class="menu-button text-left"
							>
								<Captions class="size-4" />
								View Audit Logs
							</button>
						{/if}
					</div>
				{/snippet}
			</DotDotDot>
		{/snippet}
	</Table>
{/if}

<EnableFilter
	bind:this={enableFilterDialog}
	configuredFilterServers={systemCatalogServers}
	onSuccess={onRefresh}
/>

<Confirm
	show={!!deleteBuiltInFilter}
	oncancel={() => (deleteBuiltInFilter = undefined)}
	onsuccess={async () => {
		if (!deleteBuiltInFilter?.filter) return;
		deleting = true;
		await AdminService.deleteMCPFilter(deleteBuiltInFilter.filter.id);
		deleteBuiltInFilter = undefined;
		deleting = false;
		onRefresh?.();
	}}
	loading={deleting}
	msg={`Disable ${deleteBuiltInFilter?.name || 'this filter'}?`}
>
	{#snippet note()}
		<p class="text-on-background mb-4 text-sm font-normal">
			To re-enable this filter, you will need to configure it again. Any configuration values,
			selectors, and chosen MCP servers will have be to reapplied. Are you sure you wish to
			continue?
		</p>
	{/snippet}
</Confirm>
