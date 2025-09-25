<script lang="ts" generics="T extends { id: string | number }">
	import { ChevronsLeft, ChevronsRight } from 'lucide-svelte';
	import { type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';
	import TableHeader from './TableHeader.svelte';

	interface Props<T> {
		actions?: Snippet<[T]>;
		classes?: {
			root?: string;
		};
		headers?: { title: string; property: string }[];
		headerClasses?: { property: string; class: string }[];
		fields: string[];
		data: T[];
		onSelectRow?: (row: T, isCtrlClick: boolean) => void;
		onRenderColumn?: Snippet<[string, T]>;
		setRowClasses?: (row: T) => string;
		noDataMessage?: string;
		pageSize?: number;
		sortable?: string[];
		filterable?: string[];
		initSort?: { property: string; order: 'asc' | 'desc' };
	}

	const {
		actions,
		classes,
		headers,
		headerClasses,
		data,
		fields,
		onSelectRow,
		onRenderColumn,
		pageSize,
		noDataMessage = 'No data',
		setRowClasses,
		sortable,
		filterable,
		initSort
	}: Props<T> = $props();

	let page = $state(0);
	let total = $state(data.length);

	let sortableFields = $derived(new Set(sortable));
	let filterableFields = $derived(new Set(filterable));
	let sortedBy = $state<{ property: string; order: 'asc' | 'desc' } | undefined>(
		initSort ? initSort : sortable?.[0] ? { property: sortable[0], order: 'asc' } : undefined
	);
	let filteredBy = $state<Record<string, (string | number)[]>>();
	let filterValues = $derived.by(() => {
		if (!filterable) return {};

		return data.reduce(
			(acc, item) => {
				for (const property of filterable) {
					if (!acc[property]) {
						acc[property] = new Set();
					}

					const value = item[property as keyof T];
					if (typeof value === 'string' || typeof value === 'number') {
						acc[property].add(value);
					}
				}
				return acc;
			},
			{} as Record<string, Set<string | number>>
		);
	});

	let tableData = $derived.by(() => {
		let updatedTableData = data;
		updatedTableData = sortedBy
			? data.sort((a, b) => {
					const aValue = a[sortedBy!.property as keyof T];
					const bValue = b[sortedBy!.property as keyof T];

					if (sortedBy?.property === 'created') {
						const aDate = new Date(aValue as string);
						const bDate = new Date(bValue as string);
						return sortedBy!.order === 'asc'
							? aDate.getTime() - bDate.getTime()
							: bDate.getTime() - aDate.getTime();
					}

					if (typeof aValue === 'number' && typeof bValue === 'number') {
						return sortedBy!.order === 'asc' ? aValue - bValue : bValue - aValue;
					}

					if (typeof aValue === 'string' && typeof bValue === 'string') {
						return sortedBy!.order === 'asc'
							? aValue.localeCompare(bValue)
							: bValue.localeCompare(aValue);
					}

					return 0;
				})
			: data;

		updatedTableData =
			filteredBy && Object.keys(filteredBy).length > 0
				? updatedTableData.filter((d) =>
						Object.keys(filteredBy || {}).every((property) => {
							const value = d[property as keyof T];
							if (typeof value === 'string' || typeof value === 'number') {
								return filteredBy?.[property]?.includes(value.toString());
							}
							return false;
						})
					)
				: updatedTableData;
		return updatedTableData;
	});

	function handleSort(property: string) {
		if (!sortable?.includes(property)) return;
		if (!sortedBy || sortedBy.property !== property) {
			sortedBy = { property, order: 'asc' };
		} else {
			sortedBy.order = sortedBy.order === 'asc' ? 'desc' : 'asc';
		}
	}

	function handleFilter(property: string, values: string[]) {
		if (!filterable?.includes(property)) return;
		if (values.length === 0) {
			delete filteredBy?.[property];
		} else {
			filteredBy = {
				...filteredBy,
				[property]: values
			};
		}
	}
</script>

<div
	class={twMerge(
		'dark:bg-surface2 w-full overflow-hidden overflow-x-auto rounded-md bg-white shadow-sm',
		classes?.root
	)}
>
	<table class="w-full border-collapse">
		<thead class="dark:bg-surface1 bg-surface2">
			<tr>
				{#each fields as property (property)}
					{@const headerClass = headerClasses?.find((hc) => hc.property === property)?.class}
					{@const headerTitle = headers?.find((h) => h.property === property)?.title}
					<TableHeader
						sortable={sortableFields.has(property)}
						filterable={filterableFields.has(property)}
						filterOptions={filterValues[property] ? Array.from(filterValues[property]) : []}
						{headerClass}
						{headerTitle}
						{property}
						onFilter={handleFilter}
						onSort={handleSort}
						activeSort={sortedBy?.property === property}
						order={sortedBy?.order}
					/>
				{/each}
				{#if actions}
					{@const actionHeaderClass = headerClasses?.find((hc) => hc.property === 'actions')?.class}
					<th
						class={twMerge(
							'text-md float-right w-auto px-4 py-2 text-left font-medium text-gray-500',
							actionHeaderClass
						)}
					></th>
				{/if}
			</tr>
		</thead>
		{#if tableData.length > 0}
			<tbody>
				{#each pageSize ? tableData.slice(page * pageSize, (page + 1) * pageSize) : tableData as d (sortedBy ? `${d.id}-${sortedBy.property}-${sortedBy.order}` : d.id)}
					{@render row(d)}
				{/each}
			</tbody>
		{/if}
	</table>
</div>
{#if tableData.length === 0}
	<div class="my-2 flex flex-col items-center justify-center gap-2">
		{#if Object.keys(filteredBy || {}).length > 0}
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">No results found.</p>
			<button class="button text-sm" onclick={() => (filteredBy = undefined)}>
				Clear All Filters
			</button>
		{:else}
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">{noDataMessage}</p>
		{/if}
	</div>
{/if}

{#if pageSize && tableData.length > pageSize}
	<div class="flex items-center justify-center gap-4">
		<button
			class="button-text flex items-center gap-1 text-xs disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:no-underline"
			disabled={page === 0}
			onclick={() => page--}
		>
			<ChevronsLeft class="size-4" /> Previous
		</button>

		<p class="text-xs text-gray-500">
			{page + 1} of {Math.ceil(total / pageSize)}
		</p>

		<button
			class="button-text flex items-center gap-1 text-xs disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:no-underline"
			disabled={page === Math.floor(total / pageSize)}
			onclick={() => page++}
		>
			Next <ChevronsRight class="size-4" />
		</button>
	</div>
{/if}

{#snippet row(d: T)}
	<tr
		class={twMerge(
			'border-surface2 dark:border-surface2 border-t shadow-xs transition-colors duration-300',
			onSelectRow && ' hover:bg-surface1 dark:hover:bg-surface3 cursor-pointer',
			setRowClasses?.(d)
		)}
		onclick={(e) => {
			const isTouchDevice = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
			const isCtrlClick = isTouchDevice ? false : e.metaKey || e.ctrlKey;
			onSelectRow?.(d, isCtrlClick);
		}}
	>
		{#each fields as fieldName (fieldName)}
			<td class="overflow-hidden text-sm font-light">
				<div class="flex h-full min-h-12 w-full items-center px-4 py-2">
					{#if onRenderColumn}
						{@render onRenderColumn(fieldName, d)}
					{:else}
						{d[fieldName as keyof T]}
					{/if}
				</div>
			</td>
		{/each}
		{#if actions}
			<td class="flex justify-end px-4 py-2 text-sm font-light">
				{@render actions(d)}
			</td>
		{/if}
	</tr>
{/snippet}
