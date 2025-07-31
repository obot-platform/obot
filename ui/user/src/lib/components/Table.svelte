<script lang="ts" generics="T extends { id: string | number }">
	import { ChevronDown, ChevronsLeft, ChevronsRight, ChevronUp } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props<T> {
		actions?: Snippet<[T]>;
		classes?: {
			root?: string;
		};
		headers?: { title: string; property: string }[];
		headerClasses?: { property: string; class: string }[];
		fields: string[];
		data: T[];
		onSelectRow?: (row: T) => void;
		onRenderColumn?: Snippet<[string, T]>;
		setRowClasses?: (row: T) => string;
		noDataMessage?: string;
		pageSize?: number;
		sortable?: string[];
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
		sortable
	}: Props<T> = $props();

	let page = $state(0);
	let total = $state(data.length);

	let sortableFields = $derived(new Set(sortable));
	let sortedBy = $state<{ property: string; order: 'asc' | 'desc' }>();
	let sortedData = $state<T[]>([]);

	$effect(() => {
		sortedData = sortedBy
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
	});
</script>

{#if sortedData.length > 0}
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
						<th
							class={twMerge(
								'text-md group px-4 py-2 text-left font-medium text-gray-500 capitalize',
								sortableFields.has(property) && 'cursor-pointer',
								headerClass
							)}
							onclick={() => {
								if (!sortable?.includes(property)) return;
								if (!sortedBy || sortedBy.property !== property) {
									sortedBy = { property, order: 'desc' };
								} else {
									sortedBy.order = sortedBy.order === 'asc' ? 'desc' : 'asc';
								}
							}}
						>
							<span class="flex grow items-center justify-between gap-4">
								{headerTitle ?? property}
								{#if sortable?.includes(property)}
									<button class="opacity-0 group-hover:opacity-100">
										{#if sortedBy && sortedBy.property === property}
											{#if sortedBy.order === 'desc'}
												<ChevronUp class="size-4" />
											{:else}
												<ChevronDown class="size-4" />
											{/if}
										{:else}
											<ChevronDown class="size-4" />
										{/if}
									</button>
								{/if}
							</span>
						</th>
					{/each}
					{#if actions}
						{@const actionHeaderClass = headerClasses?.find(
							(hc) => hc.property === 'actions'
						)?.class}
						<th
							class={twMerge(
								'text-md float-right w-auto px-4 py-2 text-left font-medium text-gray-500',
								actionHeaderClass
							)}
						></th>
					{/if}
				</tr>
			</thead>
			<tbody>
				{#each pageSize ? sortedData.slice(page * pageSize, (page + 1) * pageSize) : sortedData as d (sortedBy ? `${d.id}-${sortedBy.property}-${sortedBy.order}` : d.id)}
					{@render row(d)}
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<div class="my-2 flex items-center justify-center">
		<p class="text-sm font-light text-gray-400 dark:text-gray-600">{noDataMessage}</p>
	</div>
{/if}

{#if pageSize && sortedData.length > pageSize}
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
		onclick={() => onSelectRow?.(d)}
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
