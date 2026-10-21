<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import { m } from '$lib/i18n';
	import {
		VMCP_SORT_DIRECTION_OPTIONS,
		VMCP_SORT_FIELD_OPTIONS,
		VMCP_STATUS_FILTER_OPTIONS
	} from '$lib/services/vmcps/constants';
	import type { VMcpFilterOption, VMcpListSettings } from '$lib/services/vmcps/types';
	import { parseSelectedFilterIds } from '$lib/services/vmcps/utils';
	import { profile } from '$lib/stores';
	import { Funnel, LayoutGrid, X, Table } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	const BUTTON_ID = 'vmcp-settings-button';
	const SORT_LABEL_ID = 'vmcp-sort-by-label';
	const CREATOR_FILTER_LABEL_ID = 'vmcp-filter-by-creator-label';
	const STATUS_FILTER_LABEL_ID = 'vmcp-filter-by-status-label';
	const SERVER_FILTER_LABEL_ID = 'vmcp-filter-by-server-label';
	const selectClasses = 'text-sm';

	interface Props {
		filters: VMcpListSettings;
		onChange: (property: keyof VMcpListSettings, values: string[]) => void;
		componentFilterOptions?: VMcpFilterOption[];
		creatorFilterOptions?: VMcpFilterOption[];
		actions?: Snippet;
	}

	let {
		filters,
		onChange,
		componentFilterOptions = [],
		creatorFilterOptions = [],
		actions
	}: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let componentDraft = $state<string | number | undefined>('');
	let statusDraft = $state<string | number | undefined>('');
	let creatorDraft = $state<string | number | undefined>('');
	let isAdmin = $derived(Boolean(profile.current.isAdmin?.()));

	type MultiFilter = 'statusFilterBy' | 'componentFilterBy' | 'creatorFilterBy';

	let activeFilterPills = $derived([
		...(!isAdmin && filters.showMyVMcpsOnly
			? [
					{
						id: 'showMyVMcpsOnly',
						label: m.vmcps_show_my_vmcps_only(),
						property: 'showMyVMcpsOnly' as const
					}
				]
			: []),
		...pillsFor(creatorFilterOptions, filters.creatorFilterBy).map((item) => ({
			...item,
			property: 'creatorFilterBy' as const
		})),
		...pillsFor(VMCP_STATUS_FILTER_OPTIONS, filters.statusFilterBy).map((item) => ({
			...item,
			property: 'statusFilterBy' as const
		})),
		...pillsFor(componentFilterOptions, filters.componentFilterBy).map((item) => ({
			...item,
			property: 'componentFilterBy' as const
		}))
	]);

	function selectedFilter(property: MultiFilter) {
		if (property === 'statusFilterBy') return filters.statusFilterBy;
		if (property === 'creatorFilterBy') return filters.creatorFilterBy;
		return filters.componentFilterBy;
	}

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
			placeholder={m.vmcps_search_vmcps()}
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
		/>
		<button class="btn btn-neutral h-12.5" id={BUTTON_ID} onclick={() => dialog?.open()}>
			<Funnel class="size-4" />
			{m.core_filters_title()}
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
						aria-label={m.vmcps_remove_named({ name: item.label })}
						onclick={() => {
							if (item.property === 'showMyVMcpsOnly') {
								onChange('showMyVMcpsOnly', []);
								return;
							}
							onChange(
								item.property,
								parseSelectedFilterIds(removeFilter(selectedFilter(item.property), String(item.id)))
							);
						}}
					>
						<X class="size-3" />
					</button>
				</div>
			{/each}
		</div>
	{/if}
</div>

<div class="flex items-center justify-between gap-2">
	<div class="flex items-center gap-2">
		{#if actions}
			{@render actions()}
		{/if}
	</div>
	<div class="flex items-center gap-2">
		<button
			class={twMerge('btn', filters.variant === 'grid' ? 'btn-active' : undefined)}
			onclick={() => onChange('variant', ['grid'])}
		>
			<LayoutGrid class="size-4" />
			{m.core_grid_view()}
		</button>
		<button
			class={twMerge('btn', filters.variant === 'table' ? 'btn-active' : undefined)}
			onclick={() => onChange('variant', ['table'])}
		>
			<Table class="size-4" />
			{m.core_table_view()}
		</button>
	</div>
</div>

<ResponsiveDialog bind:this={dialog} title={m.vmcps_settings()} class="md:w-md">
	<div class="flex flex-col gap-2">
		{#if !isAdmin}
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
				{m.vmcps_show_my_vmcps_only()}
			</label>
		{/if}

		<label id={SORT_LABEL_ID} for="vmcp-sort-by" class="divider my-2 text-xs uppercase">
			{m.vmcps_sort_by_heading()}
		</label>
		<div class="flex gap-2 flex-col">
			<Select
				id="vmcp-sort-by"
				options={VMCP_SORT_FIELD_OPTIONS}
				selected={filters.sortBy}
				placeholder={m.vmcps_sort_by()}
				ariaLabelledby={SORT_LABEL_ID}
				class={selectClasses}
				classes={{ option: 'text-sm' }}
				onSelect={(option) => onChange('sortBy', [String(option.id)])}
			/>
			<Select
				id="vmcp-sort-direction"
				options={VMCP_SORT_DIRECTION_OPTIONS}
				selected={filters.sortDirection}
				placeholder={m.vmcps_sort_direction()}
				class={selectClasses}
				classes={{ option: 'text-sm' }}
				onSelect={(option) => onChange('sortDirection', [option.id === 'desc' ? 'desc' : 'asc'])}
			/>
		</div>

		{#if isAdmin}
			<label
				id={CREATOR_FILTER_LABEL_ID}
				for="vmcp-filter-by-creator"
				class="divider my-2 text-xs uppercase"
			>
				{m.vmcps_filter_by_creator_heading()}
			</label>
			<Select
				id="vmcp-filter-by-creator"
				options={unusedOptions(creatorFilterOptions, filters.creatorFilterBy)}
				bind:selected={creatorDraft}
				searchInDropdown
				placeholder={m.vmcps_filter_by_creator()}
				searchPlaceholder={m.vmcps_search_creators()}
				ariaLabelledby={CREATOR_FILTER_LABEL_ID}
				class={selectClasses}
				classes={{ root: 'grow', option: 'text-sm' }}
				onSelect={(option) => {
					onChange(
						'creatorFilterBy',
						parseSelectedFilterIds(addFilter(filters.creatorFilterBy, String(option.id)))
					);
					creatorDraft = '';
				}}
			/>
			{@render dialogFilterPills(
				pillsFor(creatorFilterOptions, filters.creatorFilterBy),
				'creatorFilterBy'
			)}
		{/if}

		<label
			id={STATUS_FILTER_LABEL_ID}
			for="vmcp-filter-by-status"
			class="divider my-2 text-xs uppercase"
		>
			{m.vmcps_filter_by_status_heading()}
		</label>
		<Select
			id="vmcp-filter-by-status"
			options={unusedOptions(VMCP_STATUS_FILTER_OPTIONS, filters.statusFilterBy)}
			bind:selected={statusDraft}
			placeholder={m.vmcps_filter_by_status()}
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
			{m.vmcps_filter_by_mcp_servers_heading()}
		</label>
		<Select
			id="vmcp-filter-by-server"
			options={unusedOptions(componentFilterOptions, filters.componentFilterBy)}
			bind:selected={componentDraft}
			searchInDropdown
			placeholder={m.vmcps_filter_by_mcp_server()}
			searchPlaceholder={m.vmcps_search_mcp_servers()}
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

{#snippet dialogFilterPills(items: VMcpFilterOption[], property: MultiFilter)}
	{#if items.length > 0}
		<ul class="mt-2 flex flex-wrap gap-1">
			{#each items as item (item.id)}
				<li class="filter-primary">
					<span>{item.label}</span>
					<button
						type="button"
						class="btn btn-square btn-ghost size-4 min-h-4 text-muted-content hover:text-base-content"
						aria-label={m.vmcps_remove_named({ name: item.label })}
						onclick={() =>
							onChange(
								property,
								parseSelectedFilterIds(removeFilter(selectedFilter(property), String(item.id)))
							)}
					>
						<X class="size-3" />
					</button>
				</li>
			{/each}
		</ul>
	{/if}
{/snippet}
