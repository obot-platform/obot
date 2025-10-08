<script lang="ts" generics="T extends { id: string | number }">
	import { ChevronsLeft, ChevronsRight, Square, SquareCheck, SquareMinus } from 'lucide-svelte';
	import { onMount, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';
	import TableHeader from './TableHeader.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props<T> {
		actions?: Snippet<[T]>;
		classes?: {
			root?: string;
			thead?: string;
		};
		headers?: { title: string; property: string }[];
		headerClasses?: { property: string; class: string }[];
		fields: string[];
		data: T[];
		onClickRow?: (row: T, isCtrlClick: boolean) => void;
		onFilter?: (property: string, values: string[]) => void;
		onRenderColumn?: Snippet<[string, T]>;
		onRenderSubrowContent?: Snippet<[T]>;
		setRowClasses?: (row: T) => string;
		noDataMessage?: string;
		pageSize?: number;
		sortable?: string[];
		filterable?: string[];
		filters?: Record<string, (string | number)[]>;
		initSort?: { property: string; order: 'asc' | 'desc' };
		tableSelectActions?: Snippet<[Record<string, T>]>;
		validateSelect?: (row: T) => boolean;
		disabledSelectMessage?: string;
	}

	const {
		actions,
		classes,
		headers,
		headerClasses,
		data,
		fields,
		onClickRow,
		onFilter,
		onRenderColumn,
		onRenderSubrowContent,
		pageSize,
		noDataMessage = 'No data',
		setRowClasses,
		sortable,
		filterable,
		initSort,
		filters,
		tableSelectActions,
		validateSelect,
		disabledSelectMessage
	}: Props<T> = $props();

	let page = $state(0);
	let total = $state(data.length);

	let sortableFields = $derived(new Set(sortable));
	let filterableFields = $derived(new Set(filterable));
	let sortedBy = $state<{ property: string; order: 'asc' | 'desc' } | undefined>(
		initSort ? initSort : sortable?.[0] ? { property: sortable[0], order: 'asc' } : undefined
	);
	let filteredBy = $state<Record<string, (string | number)[]> | undefined>(filters);
	let filterValues = $derived.by(() => {
		if (!filterable) return {};

		return data.reduce(
			(acc, item) => {
				for (const property of filterable) {
					if (!acc[property]) {
						acc[property] = new Set();
					}

					const value = item[property as keyof T];
					if (Array.isArray(value)) {
						value.forEach((v) => {
							if (typeof v === 'string' || typeof v === 'number') {
								acc[property].add(v);
							}
						});
					} else if (typeof value === 'string' || typeof value === 'number') {
						acc[property].add(value);
					}
				}
				return acc;
			},
			{} as Record<string, Set<string | number>>
		);
	});

	let selected = $state<Record<string, T>>({});

	let tableData = $derived.by(() => {
		let updatedTableData = data;

		if (sortedBy) {
			updatedTableData = data.sort((a, b) => {
				// If tableSelectActions and validateSelect are available, sort by selectability first
				if (tableSelectActions && validateSelect) {
					const aSelectable = validateSelect(a);
					const bSelectable = validateSelect(b);

					// First sort by selectability (selectable items first)
					if (aSelectable !== bSelectable) {
						return aSelectable ? -1 : 1;
					}
				}

				// Then sort by the specified property
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
			});
		}

		updatedTableData =
			filteredBy && Object.keys(filteredBy).length > 0
				? updatedTableData.filter((d) =>
						Object.keys(filteredBy || {}).every((property) => {
							const value = d[property as keyof T];
							if (Array.isArray(value)) {
								return value.some((v) => filteredBy?.[property]?.includes(v.toString()));
							} else if (typeof value === 'string' || typeof value === 'number') {
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

		onFilter?.(property, values);
	}

	let visibleItems = $derived(
		pageSize ? tableData.slice(page * pageSize, (page + 1) * pageSize) : tableData
	);

	let totalSelectable = $derived(
		visibleItems.filter((d) => (validateSelect ? validateSelect(d) : true)).length
	);

	let theadElement: HTMLTableSectionElement;
	let tHeadYPosition = $state(0);
	let tHeadWidth = $state(0);

	function updateTHeadYPosition() {
		if (!theadElement) return;
		const rect = theadElement.getBoundingClientRect();
		tHeadYPosition = rect.top;
		tHeadWidth = rect.width;
	}

	onMount(() => {
		// Find the closest scrollable container
		const scrollableElement = theadElement?.closest('[class*="overflow-y-auto"]') as HTMLElement;

		if (scrollableElement) {
			scrollableElement.addEventListener('scroll', updateTHeadYPosition);
			window.addEventListener('resize', updateTHeadYPosition);

			// Initial check
			updateTHeadYPosition();

			return () => {
				scrollableElement.removeEventListener('scroll', updateTHeadYPosition);
				window.removeEventListener('resize', updateTHeadYPosition);
			};
		}
	});

	export function clearSelectAll() {
		selected = {};
	}
</script>

<div
	class={twMerge(
		'dark:bg-surface2 relative overflow-hidden overflow-x-auto rounded-md bg-white shadow-sm',
		tableSelectActions && 'overflow-visible',
		classes?.root
	)}
>
	{#if Object.keys(selected).length > 0 && tableSelectActions}
		<div
			class={twMerge('dark:bg-surface1 bg-surface2 fixed z-40 w-full', classes?.thead)}
			style={`top: ${tHeadYPosition}px; width: ${tHeadWidth}px`}
		>
			<div class="flex w-full items-center">
				<div class="flex-shrink-0 p-2">
					{@render selectAll()}
				</div>
				<div class="px-4 py-2 text-left text-sm font-semibold text-gray-500">
					{Object.keys(selected).length} of {totalSelectable} selected
				</div>
				<div class="flex grow items-center justify-end">
					{@render tableSelectActions(selected)}
				</div>
			</div>
		</div>
	{/if}
	<table class="w-full border-collapse">
		<thead
			bind:this={theadElement}
			class={twMerge(
				'dark:bg-surface1 bg-surface2',
				tableSelectActions && 'sticky top-0 left-0 z-30 w-full',
				classes?.thead
			)}
		>
			<tr>
				{#if tableSelectActions}
					<th class="w-4 p-2">
						{@render selectAll()}
					</th>
				{/if}

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
				{#each visibleItems as d (sortedBy ? `${d.id}-${sortedBy.property}-${sortedBy.order}` : d.id)}
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

{#snippet selectAll()}
	<button
		class="icon-button"
		onclick={(e) => {
			e.stopPropagation();
			if (Object.keys(selected).length > 0) {
				selected = {};
			} else {
				selected = visibleItems.reduce(
					(acc, d) => {
						const isSelectable = validateSelect ? validateSelect(d) : true;
						if (isSelectable) {
							acc[d.id] = d;
						}
						return acc;
					},
					{} as Record<string, T>
				);
			}
		}}
	>
		{#if Object.keys(selected).length === totalSelectable && totalSelectable > 0}
			<SquareCheck class="size-5" />
		{:else if Object.keys(selected).length > 0}
			<SquareMinus class="size-5" />
		{:else}
			<Square class="size-5" />
		{/if}
	</button>
{/snippet}

{#snippet row(d: T)}
	<tr
		class={twMerge(
			'border-surface2 dark:border-surface2 border-t shadow-xs transition-colors duration-300',
			onClickRow && ' hover:bg-surface1 dark:hover:bg-surface3 cursor-pointer',
			setRowClasses?.(d)
		)}
		onclick={(e) => {
			const isTouchDevice = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
			const isCtrlClick = isTouchDevice ? false : e.metaKey || e.ctrlKey;
			onClickRow?.(d, isCtrlClick);
		}}
	>
		{#if tableSelectActions}
			{@const canSelect = validateSelect ? validateSelect(d) : true}
			{#if canSelect}
				<td class="p-2">
					<button
						class="button-icon"
						onclick={(e) => {
							e.stopPropagation();
							if (selected[d.id]) {
								delete selected[d.id];
							} else {
								selected[d.id] = d;
							}
						}}
					>
						{#if selected[d.id]}
							<SquareCheck class="size-5" />
						{:else}
							<Square class="size-5" />
						{/if}
					</button>
				</td>
			{:else}
				<td class="p-2" use:tooltip={disabledSelectMessage || 'This item is not selectable'}>
					<button class="button-icon opacity-30" disabled>
						<Square class="size-5" />
					</button>
				</td>
			{/if}
		{/if}
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
	{#if onRenderSubrowContent}
		<tr>
			<td colspan={fields.length + (actions ? 1 : 0)}>
				{@render onRenderSubrowContent(d)}
			</td>
		</tr>
	{/if}
{/snippet}
