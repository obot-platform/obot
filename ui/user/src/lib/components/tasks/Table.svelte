<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		header: string[];
		placeholders?: string[];
		rows: string[][];
		buttons?: Snippet<[string[]]>;
		onCellBlur?: (value: string, row: number, col: number) => void | Promise<void>;
		editable?: boolean;
	}

	let { header, placeholders, rows, buttons, editable, onCellBlur }: Props = $props();
</script>

{#snippet drawCell(value: string, row: number, col: number)}
	{#if editable}
		<input
			type="text"
			{value}
			class="w-full bg-gray-50 dark:bg-gray-950"
			onblur={(e) => {
				if (e.target instanceof HTMLInputElement) {
					onCellBlur?.(e.target.value, row, col);
				}
			}}
			placeholder={placeholders?.[col] ?? ''}
			onkeydown={(e) => {
				if (e.key === 'Enter' && e.target instanceof HTMLInputElement) {
					e.target.blur();
				}
			}}
		/>
	{:else}
		{value}
	{/if}
{/snippet}

<table class="w-full text-left">
	<thead class="font-semibold">
		<tr>
			{#each header as key}
				<th class="">
					{key}
				</th>
			{/each}
		</tr>
	</thead>
	<tbody>
		{#each rows as row, r}
			<tr class="group">
				{#each row as cell, c}
					{#if c < header.length - 1}
						<td>
							{@render drawCell(cell, r, c)}
						</td>
					{:else if c === header.length - 1}
						<td class="flex items-center">
							{@render drawCell(cell, r, c)}
							{#if buttons}
								{@render buttons(row)}
							{/if}
						</td>
					{/if}
				{/each}
			</tr>
		{/each}
	</tbody>
</table>

<style lang="postcss">
</style>
