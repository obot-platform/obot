<script module lang="ts">
	import { m } from '$lib/i18n';

	export type SessionSortBy = 'lastUsed' | 'displayName' | 'user' | 'status';
	export type SessionSortDirection = 'asc' | 'desc';
	export type SessionListVariant = 'grid' | 'table';

	export interface SessionListFilters {
		showMySessionsOnly: boolean;
		sortBy: SessionSortBy;
		sortDirection: SessionSortDirection;
		query: string;
		statusFilterBy: string;
		userFilterBy: string;
		variant: SessionListVariant;
	}

	export interface SessionFilterOption {
		id: string;
		label: string;
	}

	export const SESSION_SORT_OPTIONS: SessionFilterOption[] = [
		{ id: 'lastUsed', label: m.hosted_agents_sessions_last_used() },
		{ id: 'displayName', label: m.core_name() },
		{ id: 'user', label: m.hosted_agents_sessions_user() },
		{ id: 'status', label: m.core_status() }
	];

	export const SESSION_SORT_DIRECTION_OPTIONS: SessionFilterOption[] = [
		{ id: 'asc', label: m.hosted_agents_sessions_sort_ascending() },
		{ id: 'desc', label: m.hosted_agents_sessions_sort_descending() }
	];

	export const SESSION_STATUS_FILTER_OPTIONS: SessionFilterOption[] = [
		{ id: 'running', label: m.hosted_agents_sessions_running() },
		{ id: 'resuming', label: m.hosted_agents_sessions_resuming() },
		{ id: 'suspending', label: m.hosted_agents_sessions_suspending() },
		{ id: 'pending', label: m.hosted_agents_sessions_pending() },
		{ id: 'error', label: m.hosted_agents_sessions_error() },
		{ id: 'suspended', label: m.hosted_agents_sessions_suspended() },
		{ id: 'deleting', label: m.hosted_agents_sessions_deleting() }
	];
</script>

<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import { Funnel, LayoutGrid, Table, X } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	const BUTTON_ID = 'claude-sessions-settings-button';
	const SORT_LABEL_ID = 'claude-sessions-sort-by-label';
	const STATUS_FILTER_LABEL_ID = 'claude-sessions-filter-by-status-label';
	const USER_FILTER_LABEL_ID = 'claude-sessions-filter-by-user-label';
	const selectClasses = 'min-h-8 py-1 text-sm bg-base-200 dark:bg-base-100 shadow-inner!';

	interface Props {
		filters: SessionListFilters;
		onChange: (property: keyof SessionListFilters, values: string[]) => void;
		showMine?: boolean;
		userFilterOptions?: SessionFilterOption[];
	}

	let { filters, onChange, showMine = false, userFilterOptions = [] }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let statusDraft = $state<string | number | undefined>('');
	let userDraft = $state<string | number | undefined>('');

	let sortOptions = $derived(
		showMine ? SESSION_SORT_OPTIONS : SESSION_SORT_OPTIONS.filter((option) => option.id !== 'user')
	);

	type MultiFilter = 'statusFilterBy' | 'userFilterBy';

	let activeFilterPills = $derived([
		...pillsFor(SESSION_STATUS_FILTER_OPTIONS, filters.statusFilterBy).map((item) => ({
			...item,
			property: 'statusFilterBy' as const
		})),
		...(showMine
			? pillsFor(userFilterOptions, filters.userFilterBy).map((item) => ({
					...item,
					property: 'userFilterBy' as const
				}))
			: [])
	]);

	function selectedFilter(property: MultiFilter) {
		return property === 'statusFilterBy' ? filters.statusFilterBy : filters.userFilterBy;
	}

	function parseSelectedFilterIds(selected: string) {
		return selected
			.split(',')
			.map((id) => id.trim())
			.filter(Boolean);
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

	function unusedOptions(options: SessionFilterOption[], selected: string) {
		const ids = new Set(parseSelectedFilterIds(selected));
		return options.filter((option) => !ids.has(option.id));
	}

	function pillsFor(options: SessionFilterOption[], selected: string) {
		return parseSelectedFilterIds(selected).map(
			(id) => options.find((option) => option.id === id) ?? { id, label: id }
		);
	}
</script>

<div class="sticky top-16 left-0 z-20 w-full bg-base-200 py-1 dark:bg-base-100">
	<div class="flex items-center gap-4">
		<Search
			value={filters.query}
			onChange={(value) => onChange('query', value ? [value] : [])}
			placeholder={m.hosted_agents_sessions_search()}
			class="border border-transparent bg-base-100 shadow-sm dark:border-base-400 dark:bg-base-200"
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
						aria-label={m.hosted_agents_sessions_remove_named({ name: item.label })}
						onclick={() =>
							onChange(
								item.property,
								parseSelectedFilterIds(removeFilter(selectedFilter(item.property), item.id))
							)}
					>
						<X class="size-3" />
					</button>
				</div>
			{/each}
		</div>
	{/if}
</div>

<div class="flex items-center justify-end gap-2">
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

<ResponsiveDialog bind:this={dialog} title={m.hosted_agents_sessions_settings()} class="md:w-md">
	<div class="flex flex-col gap-2">
		{#if showMine}
			<label class="flex w-fit items-center gap-1.5 text-sm">
				<input
					type="checkbox"
					class="checkbox checkbox-xs rounded-sm"
					checked={filters.showMySessionsOnly}
					onchange={(event) =>
						onChange(
							'showMySessionsOnly',
							(event.currentTarget as HTMLInputElement).checked ? ['true'] : []
						)}
				/>
				{m.hosted_agents_sessions_show_mine()}
			</label>
		{/if}

		<label id={SORT_LABEL_ID} for="claude-sessions-sort-by" class="divider my-2 text-xs uppercase">
			{m.hosted_agents_sessions_sort_by_heading()}
		</label>
		<div class="flex flex-col gap-2">
			<Select
				id="claude-sessions-sort-by"
				options={sortOptions}
				selected={filters.sortBy}
				placeholder={m.hosted_agents_sessions_sort_by()}
				ariaLabelledby={SORT_LABEL_ID}
				class={selectClasses}
				classes={{ root: 'grow', option: 'text-sm' }}
				onSelect={(option) => onChange('sortBy', [String(option.id)])}
			/>
			<Select
				id="claude-sessions-sort-direction"
				options={SESSION_SORT_DIRECTION_OPTIONS}
				selected={filters.sortDirection}
				placeholder={m.hosted_agents_sessions_sort_direction()}
				class={selectClasses}
				classes={{ root: 'grow', option: 'text-sm' }}
				onSelect={(option) => onChange('sortDirection', [option.id === 'asc' ? 'asc' : 'desc'])}
			/>
		</div>

		<label
			id={STATUS_FILTER_LABEL_ID}
			for="claude-sessions-filter-by-status"
			class="divider my-2 text-xs uppercase"
		>
			{m.hosted_agents_sessions_filter_by_status_heading()}
		</label>
		<Select
			id="claude-sessions-filter-by-status"
			options={unusedOptions(SESSION_STATUS_FILTER_OPTIONS, filters.statusFilterBy)}
			bind:selected={statusDraft}
			placeholder={m.hosted_agents_sessions_filter_by_status()}
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
			pillsFor(SESSION_STATUS_FILTER_OPTIONS, filters.statusFilterBy),
			'statusFilterBy'
		)}

		{#if showMine}
			<label
				id={USER_FILTER_LABEL_ID}
				for="claude-sessions-filter-by-user"
				class="divider my-2 text-xs uppercase"
			>
				{m.hosted_agents_sessions_filter_by_user_heading()}
			</label>
			<Select
				id="claude-sessions-filter-by-user"
				options={unusedOptions(userFilterOptions, filters.userFilterBy)}
				bind:selected={userDraft}
				searchInDropdown
				placeholder={m.hosted_agents_sessions_filter_by_user()}
				searchPlaceholder={m.hosted_agents_sessions_search_users()}
				ariaLabelledby={USER_FILTER_LABEL_ID}
				class={selectClasses}
				classes={{ root: 'grow', option: 'text-sm' }}
				onSelect={(option) => {
					onChange(
						'userFilterBy',
						parseSelectedFilterIds(addFilter(filters.userFilterBy, String(option.id)))
					);
					userDraft = '';
				}}
			/>
			{@render dialogFilterPills(pillsFor(userFilterOptions, filters.userFilterBy), 'userFilterBy')}
		{/if}
	</div>
</ResponsiveDialog>

{#snippet dialogFilterPills(items: SessionFilterOption[], property: MultiFilter)}
	{#if items.length > 0}
		<ul class="mt-2 flex flex-wrap gap-1">
			{#each items as item (item.id)}
				<li class="filter-primary">
					<span>{item.label}</span>
					<button
						type="button"
						class="btn btn-square btn-ghost size-4 min-h-4 text-muted-content hover:text-base-content"
						aria-label={m.hosted_agents_sessions_remove_named({ name: item.label })}
						onclick={() =>
							onChange(
								property,
								parseSelectedFilterIds(removeFilter(selectedFilter(property), item.id))
							)}
					>
						<X class="size-3" />
					</button>
				</li>
			{/each}
		</ul>
	{/if}
{/snippet}
