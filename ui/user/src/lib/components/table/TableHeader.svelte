<script lang="ts">
	import popover from '$lib/actions/popover.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { ArrowDown, ArrowUp, Funnel } from 'lucide-svelte';
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
		order?: 'asc' | 'desc';
		sortable?: boolean;
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
		order,
		sortable
	}: Props = $props();

	let query = $state('');
	let selectedFilterValues = $state<string[]>([]);
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
		'text-md group px-4 py-2 text-left font-medium text-gray-500 capitalize',
		pointerOnTHeader && 'cursor-pointer',
		headerClass
	)}
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
				<Funnel class="size-3 flex-shrink-0" />
			</button>
		{:else}
			{headerTitle ?? property}
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
		<div use:tooltipRef class="default-dialog w-xs rounded-xs">
			<Select
				class="rounded-xs border border-transparent shadow-inner"
				classes={{
					root: 'flex grow'
				}}
				options={filterOptions?.map((option) => ({
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
