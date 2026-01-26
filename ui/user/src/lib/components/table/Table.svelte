<script lang="ts" generics="T extends { id: string | number }">
	import {
		ChevronDown,
		ChevronsLeft,
		ChevronsRight,
		Square,
		SquareCheck,
		SquareMinus
	} from 'lucide-svelte';
	import { onMount, type Snippet } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';
	import TableHeader from './TableHeader.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import TableColumnFilter from './TableColumnFilter.svelte';

	export type InitSort = { property: string; order: 'asc' | 'desc' };
	export type InitSortFn = (property: string, order: 'asc' | 'desc') => void;

	interface Props<T> {
		actions?: Snippet<[T]>;
		classes?: {
			root?: string;
			thead?: string;
		};
		headers?: { title: string; property: string; tooltip?: string }[];
		headerClasses?: { property: string; class: string }[];
		fields: string[];
		data: T[];
		onClickRow?: (row: T, isCtrlClick: boolean) => void;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onRenderColumn?: Snippet<[string, T]>;
		onRenderSubrowContent?: Snippet<[T]>;
		onSort?: InitSortFn;
		setRowClasses?: (row: T) => string;
		noDataMessage?: string;
		pageSize?: number;
		sortable?: string[];
		filterable?: string[];
		filters?: Record<string, (string | number)[]>;
		initSort?: InitSort;
		tableSelectActions?: Snippet<[Record<string, T>]>;
		validateSelect?: (row: T) => boolean;
		disabledSelectMessage?: string;
		sectionedBy?: string;
		sectionPrimaryTitle?: string;
		sectionSecondaryTitle?: string;
		disablePortal?: boolean;
	}

	const {
		actions,
		classes,
		headers,
		headerClasses,
		data,
		fields,
		onClickRow,
		onClearAllFilters,
		onFilter,
		onRenderColumn,
		onRenderSubrowContent,
		onSort,
		pageSize,
		noDataMessage = 'No data',
		setRowClasses,
		sortable,
		filterable,
		initSort,
		filters,
		tableSelectActions,
		validateSelect,
		disabledSelectMessage,
		sectionedBy,
		sectionPrimaryTitle,
		sectionSecondaryTitle,
		disablePortal
	}: Props<T> = $props();

	let page = $state(0);
	let total = $derived(data.length);

	let sortableFields = $derived(new Set(sortable));
	let filterableFields = $derived(new Set(filterable));
	let sortedBy = $derived<{ property: string; order: 'asc' | 'desc' } | undefined>(
		initSort ?? (sortable?.[0] ? { property: sortable[0], order: 'asc' } : undefined)
	);

	let filteredBy = $derived<Record<string, (string | number)[]> | undefined>(filters);
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
								acc[property].add((typeof v === 'string' ? v : v.toString()).trim());
							}
						});
					} else if (typeof value === 'string' || typeof value === 'number') {
						acc[property].add((typeof value === 'string' ? value : value.toString()).trim());
					}
				}
				return acc;
			},
			{} as Record<string, Set<string | number>>
		);
	});

	let selected = $state<Record<string, T>>({});
	let dataTableRef: HTMLTableElement | null = $state(null);
	let headerTableRef: HTMLTableSectionElement | null = $state(null);
	let headerScrollRef: HTMLDivElement | null = $state(null);
	let bodyScrollRef: HTMLDivElement | null = $state(null);
	let wrapperRef: HTMLDivElement | null = $state(null);
	let columnWidths = $state<number[]>([]);
	let stickyTop = $state(0);

	let autoHiddenFieldIndices = new SvelteSet<number>();
	let userHiddenFieldIndices = $state<SvelteSet<number> | null>(null);

	// User preferences take priority over auto-hidden from resizing
	let hiddenFieldIndices = $derived(userHiddenFieldIndices ?? autoHiddenFieldIndices);
	let visibleFields = $derived(fields.filter((_, index) => !hiddenFieldIndices.has(index)));

	function handleColumnVisibilityChange(hiddenIndices: Set<number>) {
		const newSet = new SvelteSet<number>();
		hiddenIndices.forEach((i) => newSet.add(i));
		userHiddenFieldIndices = newSet;
	}

	function handleColumnVisibilityReset() {
		userHiddenFieldIndices = null;
		measureColumnWidths();
	}

	let tableData = $derived.by(() => {
		let updatedTableData = data;

		if (sortedBy) {
			updatedTableData = data.sort((a, b) => {
				if (tableSelectActions && validateSelect && sortedBy?.property === 'selectable') {
					const aSelectable = validateSelect(a);
					const bSelectable = validateSelect(b);

					// First sort by selectability (selectable items first)
					if (aSelectable !== bSelectable) {
						return aSelectable ? -1 : 1;
					}
				}

				// Then sort by the specified property
				let aValue = a[sortedBy!.property as keyof T];
				let bValue = b[sortedBy!.property as keyof T];

				if (sortedBy?.property === 'created') {
					const aDate = new Date(aValue as string);
					const bDate = new Date(bValue as string);
					return sortedBy!.order === 'asc'
						? aDate.getTime() - bDate.getTime()
						: bDate.getTime() - aDate.getTime();
				}

				if (Array.isArray(aValue) && Array.isArray(bValue)) {
					// use first value in array to sort
					aValue = aValue[0];
					bValue = bValue[0];
				}

				if (typeof aValue === 'number' && typeof bValue === 'number') {
					return sortedBy!.order === 'asc' ? aValue - bValue : bValue - aValue;
				}

				if (typeof aValue === 'string' && typeof bValue === 'string') {
					return sortedBy!.order === 'asc'
						? aValue.localeCompare(bValue)
						: bValue.localeCompare(aValue);
				}

				if (typeof aValue === 'boolean' && typeof bValue === 'boolean') {
					// If both are the same, sort alphabetically by first field
					if (aValue === bValue && fields.length > 0) {
						const firstFieldA = a[fields[0] as keyof T];
						const firstFieldB = b[fields[0] as keyof T];
						if (typeof firstFieldA === 'string' && typeof firstFieldB === 'string') {
							return firstFieldA.localeCompare(firstFieldB);
						}
					}
					return sortedBy!.order === 'asc' ? (aValue ? 1 : -1) : bValue ? 1 : -1;
				}

				return 0;
			});
		}

		updatedTableData =
			filteredBy && Object.keys(filteredBy).length > 0
				? updatedTableData.filter((d) =>
						Object.keys(filteredBy || {}).every((property) => {
							if (property === 'selectable') {
								return validateSelect ? validateSelect(d) : true;
							}

							const value = d[property as keyof T];
							if (Array.isArray(value)) {
								return value.some((v) => filteredBy?.[property]?.includes(v.toString().trim()));
							} else if (typeof value === 'string' || typeof value === 'number') {
								return filteredBy?.[property]?.includes(value.toString().trim());
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

		sortedBy = { ...sortedBy };

		onSort?.(property, sortedBy.order);
	}

	function handleFilter(property: string, values: string[]) {
		if (!filterable?.includes(property)) return;
		if (values.length === 0) {
			delete filteredBy?.[property];
			filteredBy = { ...filteredBy };
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

	export function clearSelectAll() {
		selected = {};
	}

	function calculateConstrainedWidths(naturalWidths: number[], availableWidth: number): number[] {
		const totalNaturalWidth = naturalWidths.reduce((sum, width) => sum + width, 0);

		// If total width fits within available space, return natural widths
		if (totalNaturalWidth <= availableWidth) {
			return naturalWidths;
		}

		const minWidths = naturalWidths.map((width, index) =>
			index === 0 && tableSelectActions ? 57 : Math.max(width * 0.3, 100)
		);
		const totalMinWidth = minWidths.reduce((sum, width) => sum + width, 0);

		if (totalMinWidth > availableWidth) {
			return naturalWidths;
		}

		const excessWidth = totalNaturalWidth - availableWidth;
		const reducibleWidth = totalNaturalWidth - totalMinWidth;
		const scaleFactor = Math.max(0, (reducibleWidth - excessWidth) / reducibleWidth);

		// scaling
		return naturalWidths.map((width, index) => {
			const scaledWidth = width * scaleFactor;
			return Math.max(scaledWidth, minWidths[index]);
		});
	}

	// If there is no data, measure using the header cells instead of the first row's cells
	function getTableCells(): HTMLTableCellElement[] | null {
		let firstRow = dataTableRef?.querySelector('tbody tr:not([data-section-header])');
		const cells = firstRow
			? (firstRow?.querySelectorAll('td') ?? dataTableRef?.querySelectorAll('tr th'))
			: headerTableRef?.querySelectorAll('th');
		return cells ? (Array.from(cells) as HTMLTableCellElement[]) : null;
	}

	function measureCellWidth(cell: HTMLTableCellElement): number {
		if (cell.tagName === 'TD') {
			const contentDiv = cell.querySelector('div');
			return contentDiv ? contentDiv.scrollWidth : cell.scrollWidth;
		}
		return cell.scrollWidth;
	}

	function calculateFieldPadding(fieldIndex: number): number {
		const property = fields[fieldIndex];
		let padding = 32; // base cell padding

		if (filterableFields.has(property)) {
			padding += 12; // filter icon and gap
		}

		if (sortableFields.has(property)) {
			padding += 20; // sort icon and gap
		}

		return padding;
	}

	function measureNaturalWidths(cells: HTMLTableCellElement[]): number[] {
		const naturalWidths: number[] = [];
		const isHeaderCells = cells[0]?.tagName === 'TH';
		const selectColOffset = tableSelectActions ? 1 : 0;
		const actionsOffset = actions ? 1 : 0;

		cells.forEach((cell, index) => {
			let width = measureCellWidth(cell);

			const isFieldColumn =
				index >= selectColOffset &&
				index < cells.length - actionsOffset &&
				index < selectColOffset + fields.length;
			if (isFieldColumn) {
				const fieldIndex = index - selectColOffset;
				if (isHeaderCells) {
					width += 32; // base cell padding only for headers
				} else {
					width += calculateFieldPadding(fieldIndex);
				}
			}

			naturalWidths.push(width);
		});

		return naturalWidths;
	}

	function getAvailableWidth(): number {
		const parentContainer = dataTableRef?.closest('.default-scrollbar-thin') as HTMLElement;
		return parentContainer ? parentContainer.clientWidth : 0;
	}

	function determineHiddenColumns(
		constrainedWidths: number[],
		availableWidth: number
	): SvelteSet<number> {
		let totalWidth = constrainedWidths.reduce((sum, w) => sum + w, 0);

		if (totalWidth <= availableWidth || availableWidth === 0) {
			return new SvelteSet();
		}

		const newHiddenIndices = new SvelteSet<number>();
		// to exclude actions from being hidden
		const selectColOffset = tableSelectActions ? 1 : 0;
		for (let i = fields.length - 1; i >= 1 && totalWidth > availableWidth; i--) {
			const colIndex = selectColOffset + i;
			newHiddenIndices.add(i);
			totalWidth -= constrainedWidths[colIndex] || 0;
		}

		return newHiddenIndices;
	}

	function buildVisibleNaturalWidths(
		naturalWidths: number[],
		hiddenIndices: Set<number>
	): number[] {
		const visibleNaturalWidths: number[] = [];
		const selectColOffset = tableSelectActions ? 1 : 0;

		if (tableSelectActions) {
			visibleNaturalWidths.push(naturalWidths[0]);
		}

		fields.forEach((_, i) => {
			if (!hiddenIndices.has(i)) {
				visibleNaturalWidths.push(naturalWidths[selectColOffset + i]);
			}
		});

		if (actions) {
			visibleNaturalWidths.push(naturalWidths[naturalWidths.length - 1]);
		}

		return visibleNaturalWidths;
	}

	function measureColumnWidths() {
		if (!dataTableRef) return;

		const previousWidths = columnWidths;
		const previousAutoHidden = new Set(autoHiddenFieldIndices);
		columnWidths = [];
		autoHiddenFieldIndices.clear();

		requestAnimationFrame(() => {
			const cells = getTableCells();

			if (!cells?.length && previousWidths.length) {
				columnWidths = previousWidths;
				previousAutoHidden.forEach((i) => autoHiddenFieldIndices.add(i));
				return;
			}

			if (!cells) return;

			const naturalWidths = measureNaturalWidths(cells);
			const availableWidth = getAvailableWidth();

			let constrainedWidths = calculateConstrainedWidths(naturalWidths, availableWidth);
			const newHiddenIndices = determineHiddenColumns(constrainedWidths, availableWidth);

			if (newHiddenIndices.size > 0) {
				newHiddenIndices.forEach((i) => autoHiddenFieldIndices.add(i));
				const effectiveHidden = userHiddenFieldIndices ?? newHiddenIndices;
				const visibleNaturalWidths = buildVisibleNaturalWidths(naturalWidths, effectiveHidden);
				constrainedWidths = calculateConstrainedWidths(visibleNaturalWidths, availableWidth);
			}

			columnWidths = constrainedWidths;
		});
	}

	onMount(() => {
		const parentContainer = dataTableRef?.closest('.default-scrollbar-thin') as HTMLElement;
		if (!parentContainer) return;

		let resizeTimeout: ReturnType<typeof setTimeout> | undefined;
		const debouncedMeasure = () => {
			clearTimeout(resizeTimeout);
			resizeTimeout = setTimeout(() => {
				measureColumnWidths();
			}, 100);
		};

		const resizeObserver = new ResizeObserver(debouncedMeasure);
		resizeObserver.observe(parentContainer);

		return () => {
			clearTimeout(resizeTimeout);
			resizeObserver.disconnect();
		};
	});

	onMount(() => {
		if (!headerScrollRef || !bodyScrollRef) return;

		const handleHeaderScroll = () => syncScroll(headerScrollRef!, bodyScrollRef!);
		const handleBodyScroll = () => syncScroll(bodyScrollRef!, headerScrollRef!);

		headerScrollRef.addEventListener('scroll', handleHeaderScroll);
		bodyScrollRef.addEventListener('scroll', handleBodyScroll);

		return () => {
			headerScrollRef?.removeEventListener('scroll', handleHeaderScroll);
			bodyScrollRef?.removeEventListener('scroll', handleBodyScroll);
		};
	});

	// Calculate sticky offset based on sticky elements above the table
	onMount(() => {
		if (!wrapperRef) return;
		stickyTop = calculateStickyTop(wrapperRef);
	});

	function findScrollContainer(element: HTMLElement): HTMLElement | null {
		let parent: HTMLElement | null = element.parentElement;
		while (parent) {
			const style = getComputedStyle(parent);
			if (style.overflowY === 'auto' || style.overflowY === 'scroll') {
				return parent;
			}
			parent = parent.parentElement;
		}
		return null;
	}

	function calculateStickyTop(wrapper: HTMLElement): number {
		const scrollContainer = findScrollContainer(wrapper);
		if (!scrollContainer) return 0;

		let maxStickyBottom = 0;

		// Traverse from wrapper up to scroll container, checking for sticky siblings
		let current: HTMLElement | null = wrapper;

		while (current && current !== scrollContainer) {
			const parent: HTMLElement | null = current.parentElement;
			if (!parent) break;

			// Check all siblings that come before current element
			for (const sibling of parent.children) {
				if (sibling === current) break;

				// Check if the sibling itself is sticky
				const siblingStyle = getComputedStyle(sibling);
				if (siblingStyle.position === 'sticky') {
					const top = parseFloat(siblingStyle.top) || 0;
					if (top >= 0 && top < 200) {
						maxStickyBottom = Math.max(
							maxStickyBottom,
							top + (sibling as HTMLElement).offsetHeight
						);
					}
				}

				// Also check for sticky descendants within the sibling
				const stickyDescendants = sibling.querySelectorAll('*');
				for (const desc of stickyDescendants) {
					const descStyle = getComputedStyle(desc);
					if (descStyle.position === 'sticky') {
						const top = parseFloat(descStyle.top) || 0;
						if (top >= 0 && top < 200) {
							maxStickyBottom = Math.max(maxStickyBottom, top + (desc as HTMLElement).offsetHeight);
						}
					}
				}
			}

			current = parent;
		}

		return maxStickyBottom;
	}

	let isScrolling = false;

	function syncScroll(source: HTMLDivElement, target: HTMLDivElement) {
		if (isScrolling) return;
		isScrolling = true;
		target.scrollLeft = source.scrollLeft;
		requestAnimationFrame(() => {
			isScrolling = false;
		});
	}

	$effect(() => {
		if (tableData.length && dataTableRef && headerTableRef) {
			// Use a small delay to ensure the table is fully rendered
			setTimeout(() => {
				measureColumnWidths();
			}, 0);
		}
	});
</script>

<div bind:this={wrapperRef}>
	<div
		class={twMerge('dark:bg-surface1 bg-surface2 sticky left-0 z-40 w-full', classes?.thead)}
		style="top: {stickyTop}px;"
	>
		{#if tableSelectActions && Object.keys(selected).length > 0}
			<div class="flex w-full items-center">
				<div class="flex-shrink-0 p-2">
					{@render selectAll()}
				</div>
				<div class="text-on-surface1 px-4 py-2 text-left text-sm font-semibold">
					{Object.keys(selected).length} of {totalSelectable} selected
				</div>
				<div class="flex grow items-center justify-end">
					{@render tableSelectActions(selected)}
				</div>
			</div>
		{:else}
			<div class="default-scrollbar-thin w-full overflow-x-auto" bind:this={headerScrollRef}>
				<table
					class="w-full border-collapse"
					style={columnWidths.length > 0 ? 'table-layout: fixed; width: 100%;' : ''}
				>
					{#if columnWidths.length > 0}
						<colgroup>
							{#if tableSelectActions}
								<col style="width: {columnWidths[0] || 57}px;" />
							{/if}
							{#each visibleFields as fieldName, index (fieldName)}
								<col
									style="width: {columnWidths[tableSelectActions ? index + 1 : index]
										? columnWidths[tableSelectActions ? index + 1 : index] + 'px'
										: 'auto'};"
								/>
							{/each}
							{#if actions}
								<col style="width: {columnWidths[columnWidths.length - 1] || 80}px;" />
							{/if}
						</colgroup>
					{/if}
					{@render header()}
				</table>
			</div>
		{/if}
	</div>
	<div
		class={twMerge(
			'dark:bg-surface2 default-scrollbar-thin bg-background relative overflow-hidden rounded-md shadow-sm',
			classes?.root
		)}
		bind:this={bodyScrollRef}
	>
		<table
			class="w-full border-collapse"
			bind:this={dataTableRef}
			style={columnWidths.length > 0 ? 'table-layout: fixed; width: 100%;' : ''}
		>
			{#if columnWidths.length > 0}
				<colgroup>
					{#if tableSelectActions}
						<col style="width: {columnWidths[0] || 57}px;" />
					{/if}
					{#each visibleFields as fieldName, index (fieldName)}
						<col
							style="width: {columnWidths[tableSelectActions ? index + 1 : index]
								? columnWidths[tableSelectActions ? index + 1 : index] + 'px'
								: 'auto'};"
						/>
					{/each}
					{#if actions}
						<col style="width: {columnWidths[columnWidths.length - 1] || 80}px;" />
					{/if}
				</colgroup>
			{/if}
			{#if tableData.length > 0}
				<tbody>
					{#if sectionedBy}
						{#key `${sortedBy?.property}-${sortedBy?.order}`}
							{@const sectionA = visibleItems.filter((d) => d[sectionedBy as keyof T])}
							{@const sectionB = visibleItems.filter((d) => !d[sectionedBy as keyof T])}

							{#if sectionA.length > 0}
								{#if sectionB.length > 0}
									<tr class="bg-surface3" data-section-header>
										<th
											colspan={visibleFields.length +
												(tableSelectActions ? 1 : 0) +
												(actions ? 1 : 0)}
											class="px-4 py-2 text-left text-xs font-semibold uppercase"
										>
											{sectionPrimaryTitle}
										</th>
									</tr>
								{/if}
								{#each sectionA as d (d.id)}
									{@render row(d)}
								{/each}
							{/if}
							{#if sectionB.length > 0}
								{#if sectionA.length > 0}
									<tr class="bg-surface3" data-section-header>
										<th
											colspan={visibleFields.length +
												(tableSelectActions ? 1 : 0) +
												(actions ? 1 : 0)}
											class="px-4 py-2 text-left text-xs font-semibold uppercase"
										>
											{sectionSecondaryTitle}
										</th>
									</tr>
								{/if}
								{#each sectionB as d (d.id)}
									{@render row(d)}
								{/each}
							{/if}
						{/key}
					{:else}
						{#each visibleItems as d (sortedBy ? `${d.id}-${sortedBy.property}-${sortedBy.order}` : d.id)}
							{@render row(d)}
						{/each}
					{/if}
				</tbody>
			{/if}
		</table>
	</div>
</div>
{#if tableData.length === 0}
	<div class="my-2 flex flex-col items-center justify-center gap-2">
		{#if Object.keys(filteredBy || {}).length > 0}
			<p class="text-on-surface1 text-sm font-light">No results found.</p>
			<button
				class="button text-sm"
				onclick={() => {
					filteredBy = undefined;
					onClearAllFilters?.();
				}}
			>
				Clear All Filters
			</button>
		{:else}
			<p class="text-on-surface1 text-sm font-light">{noDataMessage}</p>
		{/if}
	</div>
{/if}

{#if pageSize && tableData.length > pageSize}
	<div class="flex items-center justify-center gap-4">
		<button
			class="button-text flex items-center gap-1 text-xs"
			disabled={page === 0}
			onclick={() => page--}
		>
			<ChevronsLeft class="size-4" /> Previous
		</button>

		<p class="text-on-surface1 text-xs">
			{page + 1} of {Math.ceil(total / pageSize)}
		</p>

		<button
			class="button-text flex items-center gap-1 text-xs"
			disabled={page === Math.floor(total / pageSize)}
			onclick={() => page++}
		>
			Next <ChevronsRight class="size-4" />
		</button>
	</div>
{/if}

{#snippet selectAll()}
	<div class="flex items-center gap-1">
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
		{#if validateSelect}
			<DotDotDot class="text-on-surface1" {disablePortal}>
				{#snippet icon()}
					<ChevronDown class="size-4" />
				{/snippet}

				<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
					<button
						class="menu-button"
						onclick={() => {
							sortedBy = {
								property: 'selectable',
								order: 'asc'
							};
							onSort?.('selectable', 'asc');
						}}
					>
						Sort By Selectable Items
					</button>
					<button
						class="menu-button"
						onclick={async () => {
							if (filteredBy?.['selectable']) {
								delete filteredBy['selectable'];
								filteredBy = { ...filteredBy };
							} else {
								filteredBy = {
									...filteredBy,
									selectable: ['true']
								};
							}
							onFilter?.('selectable', ['true']);
						}}
					>
						{#if filteredBy?.['selectable']}
							Show All Items
						{:else}
							Show Only Selectable Items
						{/if}
					</button>
				</div>
			</DotDotDot>
		{/if}
	</div>
{/snippet}

{#snippet header(hidden?: boolean)}
	<thead
		class={twMerge(
			'dark:bg-surface1 bg-surface2 border-surface2 border-b',
			hidden && 'hidden',
			classes?.thead
		)}
		bind:this={headerTableRef}
	>
		<tr>
			{#if tableSelectActions}
				<th class="w-4 p-2">
					{@render selectAll()}
				</th>
			{/if}

			{#each visibleFields as property (property)}
				{@const headerClass = headerClasses?.find((hc) => hc.property === property)?.class}
				{@const headerConfig = headers?.find((h) => h.property === property)}
				{@const headerTitle = headerConfig?.title}
				{@const headerTooltip = headerConfig?.tooltip}
				<TableHeader
					sortable={sortableFields.has(property)}
					filterable={filterableFields.has(property)}
					filterOptions={filterValues[property] ? Array.from(filterValues[property]) : []}
					{headerClass}
					{headerTitle}
					{headerTooltip}
					{property}
					onFilter={handleFilter}
					onSort={handleSort}
					activeSort={sortedBy?.property === property}
					order={sortedBy?.order}
					presetFilters={filteredBy?.[property]}
					{disablePortal}
				/>
			{/each}
			{#if actions}
				{@const actionHeaderClass = headerClasses?.find((hc) => hc.property === 'actions')?.class}
				<th
					class={twMerge(
						'text-md text-on-surface1 float-right w-auto px-4 py-2 text-left font-medium',
						actionHeaderClass
					)}
				>
					<TableColumnFilter
						{fields}
						{headers}
						{hiddenFieldIndices}
						{disablePortal}
						onVisibilityChange={handleColumnVisibilityChange}
						onReset={handleColumnVisibilityReset}
						showReset={userHiddenFieldIndices !== null}
					/>
				</th>
			{/if}
		</tr>
	</thead>
{/snippet}

{#snippet row(d: T)}
	<tr
		class={twMerge(
			'border-surface2 dark:border-surface2 border-b shadow-xs transition-colors duration-300 last:border-b-0',
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
		{#each visibleFields as fieldName (fieldName)}
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
			<td colspan={visibleFields.length + (tableSelectActions ? 1 : 0) + (actions ? 1 : 0)}>
				{@render onRenderSubrowContent(d)}
			</td>
		</tr>
	{/if}
{/snippet}
