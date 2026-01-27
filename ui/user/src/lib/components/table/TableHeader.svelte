<script lang="ts">
	import popover from '$lib/actions/popover.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { ArrowDown, ArrowUp, CircleHelp, Funnel } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Select from '../Select.svelte';

	interface Props {
		onSort?: (property: string) => void;
		onFilter?: (property: string, values: string[]) => void;
		property: string;
		activeSort?: boolean;
		filterable?: boolean;
		filterOptions?: (string | number)[];
		headerClass?: string;
		headerTitle?: string;
		headerTooltip?: string;
		order?: 'asc' | 'desc';
		sortable?: boolean;
		style?: string;
		presetFilters?: (string | number)[];
		disablePortal?: boolean;
	}
	let {
		onSort,
		onFilter,
		property,
		activeSort,
		filterable,
		filterOptions,
		headerClass,
		headerTitle,
		headerTooltip,
		order,
		sortable,
		style,
		presetFilters,
		disablePortal
	}: Props = $props();

	let query = $state('');
	let selectedFilterValues = $derived<string[]>(
		presetFilters?.map((d) => d.toString()).filter(Boolean) ?? []
	);
	let pointerOnTHeader = $derived(sortable && !filterable);

	const {
		tooltip: tooltipRef,
		ref,
		toggle
	} = popover({
		placement: 'bottom-start'
	});
</script>

<th
	class={twMerge(
		'text-md group text-on-surface1 px-4 py-2 text-left font-medium capitalize',
		pointerOnTHeader && 'cursor-pointer',
		headerClass
	)}
	{style}
	onclick={pointerOnTHeader ? () => onSort?.(property) : undefined}
>
	<span class="flex grow items-center justify-between gap-4">
		{#if filterable}
			<button
				class="flex grow items-center gap-1 capitalize"
				use:tooltip={`Filter by ${headerTitle ?? property}`}
				use:ref
				onclick={() => toggle()}
			>
				{headerTitle ?? property}
				{#if headerTooltip}
					<div use:tooltip={{ text: headerTooltip, classes: ['w-64', 'break-normal', 'z-[60]'] }}>
						<CircleHelp class="text-on-surface1 size-3.5" />
					</div>
				{/if}
				<div
					class={twMerge(
						'flex items-center gap-1 px-2 py-0.5',
						selectedFilterValues.length > 0 && 'bg-surface3 rounded-full'
					)}
				>
					<Funnel class="size-3 flex-shrink-0" />
					{#if selectedFilterValues.length > 0}
						<span class="text-xs font-semibold">{selectedFilterValues.length}</span>
					{/if}
				</div>
			</button>
		{:else}
			<span class="flex items-center gap-1">
				{headerTitle ?? property}
				{#if headerTooltip}
					<div use:tooltip={{ text: headerTooltip, classes: ['w-64', 'break-normal', 'z-[60]'] }}>
						<CircleHelp class="text-on-surface1 size-3.5" />
					</div>
				{/if}
			</span>
		{/if}

		{#if sortable}
			{@const isSortable = sortable && activeSort}
			<button
				class="opacity-0 group-hover:opacity-100"
				onclick={!pointerOnTHeader && sortable ? () => onSort?.(property) : undefined}
			>
				{#if isSortable}
					{@const isDesc = order === 'desc'}

					{#if isDesc}
						<ArrowUp class="size-4" />
					{:else}
						<ArrowDown class="size-4" />
					{/if}
				{:else}
					<ArrowDown class="size-4 opacity-25" />
				{/if}
			</button>
		{/if}
	</span>

	{#if filterable}
		<div use:tooltipRef={{ disablePortal }} class="default-dialog w-xs rounded-xs">
			<Select
				class="rounded-xs border border-transparent shadow-inner"
				classes={{
					root: 'flex grow'
				}}
				options={filterOptions?.filter(Boolean).map((option) => ({
					label: option.toString(),
					id: option.toString()
				})) ?? []}
				onClear={(option) => {
					if (!option) return;
					selectedFilterValues = selectedFilterValues.filter((d) => d !== option.id);
					onFilter?.(property, selectedFilterValues);
				}}
				onSelect={(option) => {
					query = '';
					if (selectedFilterValues.includes(option.id)) {
						selectedFilterValues = selectedFilterValues.filter((d) => d !== option.id);
					} else {
						selectedFilterValues.push(option.id);
					}
					onFilter?.(property, selectedFilterValues);
				}}
				{query}
				multiple
				selected={selectedFilterValues.join(',')}
				searchable
				placeholder={`Filter by ${headerTitle ?? property}...`}
			/>
		</div>
	{/if}
</th>
