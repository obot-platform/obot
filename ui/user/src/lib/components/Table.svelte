<script lang="ts" generics="T extends { id: string | number }">
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props<T> {
		actions?: Snippet<[T]>;
		headers?: { title: string; property: string }[];
		headerClasses?: { property: string; class: string }[];
		fields: string[];
		data: T[];
		onSelectRow?: (row: T) => void;
		onRenderColumn?: Snippet<[string, T]>;
		noDataMessage?: string;
	}

	const {
		actions,
		headers,
		headerClasses,
		data,
		fields,
		onSelectRow,
		onRenderColumn,
		noDataMessage = 'No data'
	}: Props<T> = $props();
</script>

{#if data.length > 0}
	<div class="dark:bg-surface2 w-full overflow-hidden rounded-md bg-white shadow-sm">
		<table class="w-full border-collapse">
			<thead class="dark:bg-surface1 bg-surface2">
				<tr>
					{#each fields as property}
						{@const headerClass = headerClasses?.find((hc) => hc.property === property)?.class}
						{@const headerTitle = headers?.find((h) => h.property === property)?.title}
						<th
							class={twMerge(
								'text-md px-4 py-2 text-left font-medium text-gray-500 capitalize',
								headerClass
							)}>{headerTitle ?? property}</th
						>
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
				{#each data as d (d.id)}
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

{#snippet row(d: T)}
	<tr
		class={twMerge(
			'border-surface2 dark:border-surface2 border-t shadow-xs transition-colors duration-300',
			onSelectRow && ' hover:bg-surface1 dark:hover:bg-surface3 cursor-pointer'
		)}
		onclick={() => onSelectRow?.(d)}
	>
		{#each fields as fieldName, i}
			<td class="text-sm font-light">
				<div class="flex h-full w-full px-4 py-2">
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
