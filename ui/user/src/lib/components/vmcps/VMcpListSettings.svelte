<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import { VMCP_SORT_OPTIONS, VMCP_STATUS_FILTER_OPTIONS } from '$lib/services/vmcps/constants';
	import type { VMcpFilterOption, VMcpListSettingsFilters } from '$lib/services/vmcps/types';
	import { parseSelectedFilterIds } from '$lib/services/vmcps/utils';
	import DotDotDot from '../DotDotDot.svelte';
	import { Funnel, LayoutGrid, X, Table } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const BUTTON_ID = 'vmcp-settings-button';
	const SORT_LABEL_ID = 'vmcp-sort-by-label';
	const STATUS_FILTER_LABEL_ID = 'vmcp-filter-by-status-label';
	const SERVER_FILTER_LABEL_ID = 'vmcp-filter-by-server-label';
	const selectClasses = 'min-h-8 py-1 text-sm bg-base-200 dark:bg-base-100 shadow-inner!';

	interface Props {
		filters: VMcpListSettingsFilters;
		onChange: (property: keyof VMcpListSettingsFilters, values: string[]) => void;
		componentFilterOptions?: VMcpFilterOption[];
		variant?: 'grid' | 'table';
		selecting?: boolean;
	}

	let {
		filters,
		onChange,
		componentFilterOptions = [],
		variant = 'grid',
		selecting = $bindable(false)
	}: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let componentDraft = $state<string | number | undefined>('');
	let statusDraft = $state<string | number | undefined>('');

	let activeFilterPills = $derived([
		...pillsFor(VMCP_STATUS_FILTER_OPTIONS, filters.statusFilterBy).map((item) => ({
			...item,
			property: 'statusFilterBy' as const
		})),
		...pillsFor(componentFilterOptions, filters.componentFilterBy).map((item) => ({
			...item,
			property: 'componentFilterBy' as const
		}))
	]);

	function addFilter(selected: string, id: string) {
		const ids = parseSelectedFilterIds(selected);
		if (ids.includes(id)) return selected;
		return [...ids, id].join(',');
	}

	function removeFilter(selected: string, id: string) {
		return parseSelectedFilterIds(selected)
			.filter((value) => value !== id)
			.join(',');
	}

	function unusedOptions(options: VMcpFilterOption[], selected: string) {
		const ids = new Set(parseSelectedFilterIds(selected));
		return options.filter((option) => !ids.has(String(option.id)));
	}

	function pillsFor(options: VMcpFilterOption[], selected: string) {
		return parseSelectedFilterIds(selected).map(
			(id) => options.find((option) => option.id === id) ?? { id, label: id }
		);
	}
</script>

<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
	<div class="flex items-center gap-4">
		<Search
			value={filters.query}
			onChange={(value) => onChange('query', value ? [value] : [])}
			placeholder="Search vMCPs..."
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
		/>
		<button class="btn btn-neutral h-12.5" id={BUTTON_ID} onclick={() => dialog?.open()}>
			<Funnel class="size-4" /> Filters
		</button>
	</div>
	{#if activeFilterPills.length > 0}
		<div class="mt-2 flex flex-wrap gap-1">
			{#each activeFilterPills as item (`${item.property}-${item.id}`)}
				<div class="filter-primary">
					<span>{item.label}</span>
					<button
						type="button"
						class="btn btn-square btn-ghost size-4 min-h-4 text-muted-content hover:text-base-content"
						aria-label="Remove {item.label}"
						onclick={() =>
							onChange(
								item.property,
								parseSelectedFilterIds(
									removeFilter(
										item.property === 'statusFilterBy'
											? filters.statusFilterBy
											: filters.componentFilterBy,
										String(item.id)
									)
								)
							)}
					>
						<X class="size-3" />
					</button>
				</div>
			{/each}
		</div>
	{/if}
</div>

<div class="flex items-center justify-between md:justify-end gap-2">
	{#if variant === 'grid'}
		<div in:fade>
			<DotDotDot placement="bottom-end">
				{#snippet children({ toggle })}
					<button
						class="menu-button"
						onclick={(e) => {
							e.stopPropagation();
							selecting = !selecting;
							toggle(false);
						}}
					>
						{selecting ? 'Cancel Selection' : 'Select Multiple'}
					</button>
				{/snippet}
			</DotDotDot>
		</div>
	{/if}
	<div class="flex items-center gap-2">
		<button
			class={twMerge('btn', variant === 'grid' ? 'btn-active' : undefined)}
			onclick={() => (variant = 'grid')}
		>
			<LayoutGrid class="size-4" /> Grid View
		</button>
		<button
			class={twMerge('btn', variant === 'table' ? 'btn-active' : undefined)}
			onclick={() => {
				variant = 'table';
				selecting = false;
			}}
		>
			<Table class="size-4" /> Table View
		</button>
	</div>
</div>

<ResponsiveDialog bind:this={dialog} title="vMCPs Settings" class="md:w-md">
	<div class="flex flex-col gap-2">
		<label class="flex items-center gap-1.5 w-fit text-sm">
			<input
				type="checkbox"
				class="checkbox checkbox-xs rounded-sm"
				checked={filters.showMyVMcpsOnly}
				onchange={(event) =>
					onChange(
						'showMyVMcpsOnly',
						(event.currentTarget as HTMLInputElement).checked ? ['true'] : []
					)}
			/>
			Show my vMCPs only
		</label>

		<label id={SORT_LABEL_ID} for="vmcp-sort-by" class="divider my-2 text-xs uppercase">
			Sort By
		</label>
		<div class="flex gap-4 items-center">
			<Select
				id="vmcp-sort-by"
				options={VMCP_SORT_OPTIONS}
				selected={filters.sortBy}
				placeholder="Sort by"
				ariaLabelledby={SORT_LABEL_ID}
				class={selectClasses}
				classes={{ root: 'grow', option: 'text-sm' }}
				onSelect={(option) => onChange('sortBy', [String(option.id)])}
			/>
		</div>

		<label
			id={STATUS_FILTER_LABEL_ID}
			for="vmcp-filter-by-status"
			class="divider my-2 text-xs uppercase"
		>
			Filter By Status
		</label>
		<Select
			id="vmcp-filter-by-status"
			options={unusedOptions(VMCP_STATUS_FILTER_OPTIONS, filters.statusFilterBy)}
			bind:selected={statusDraft}
			placeholder="Filter by status"
			ariaLabelledby={STATUS_FILTER_LABEL_ID}
			class={selectClasses}
			classes={{ root: 'grow', option: 'text-sm' }}
			onSelect={(option) => {
				onChange(
					'statusFilterBy',
					parseSelectedFilterIds(addFilter(filters.statusFilterBy, String(option.id)))
				);
				statusDraft = '';
			}}
		/>
		{@render dialogFilterPills(
			pillsFor(VMCP_STATUS_FILTER_OPTIONS, filters.statusFilterBy),
			'statusFilterBy'
		)}

		<label
			id={SERVER_FILTER_LABEL_ID}
			for="vmcp-filter-by-server"
			class="divider my-2 text-xs uppercase"
		>
			Filter By MCP Servers
		</label>
		<Select
			id="vmcp-filter-by-server"
			options={unusedOptions(componentFilterOptions, filters.componentFilterBy)}
			bind:selected={componentDraft}
			searchInDropdown
			placeholder="Filter by MCP server"
			searchPlaceholder="Search MCP servers..."
			ariaLabelledby={SERVER_FILTER_LABEL_ID}
			class={selectClasses}
			classes={{ root: 'grow', option: 'text-sm' }}
			onSelect={(option) => {
				onChange(
					'componentFilterBy',
					parseSelectedFilterIds(addFilter(filters.componentFilterBy, String(option.id)))
				);
				componentDraft = '';
			}}
		/>
		{@render dialogFilterPills(
			pillsFor(componentFilterOptions, filters.componentFilterBy),
			'componentFilterBy'
		)}
	</div>
</ResponsiveDialog>

{#snippet dialogFilterPills(
	items: VMcpFilterOption[],
	property: 'statusFilterBy' | 'componentFilterBy'
)}
	{#if items.length > 0}
		<ul class="mt-2 flex flex-wrap gap-1">
			{#each items as item (item.id)}
				<li class="filter-primary">
					<span>{item.label}</span>
					<button
						type="button"
						class="btn btn-square btn-ghost size-4 min-h-4 text-muted-content hover:text-base-content"
						aria-label="Remove {item.label}"
						onclick={() =>
							onChange(
								property,
								parseSelectedFilterIds(
									removeFilter(
										property === 'statusFilterBy'
											? filters.statusFilterBy
											: filters.componentFilterBy,
										String(item.id)
									)
								)
							)}
					>
						<X class="size-3" />
					</button>
				</li>
			{/each}
		</ul>
	{/if}
{/snippet}
