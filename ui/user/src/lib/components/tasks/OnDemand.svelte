<script lang="ts">
	import type { OnDemand } from '$lib/services';
	import Table from '$lib/components/tasks/Table.svelte';
	import { Plus } from '$lib/icons';
	import { Minus } from 'lucide-svelte';

	interface Props {
		onDemand: OnDemand;
		onChanged?: (onDemand: OnDemand) => void | Promise<void>;
		editMode?: boolean;
	}

	let { onDemand, onChanged, editMode = false }: Props = $props();
	let order: string[] = [];
	let rows = $derived.by(() => {
		const keys = Object.keys(onDemand.params ?? {});
		const result: string[][] = [];

		for (const key of order) {
			if (keys.includes(key)) {
				result.push([key, onDemand.params?.[key] ?? '']);
			}
		}

		for (const key in onDemand.params ?? {}) {
			if (!order.includes(key)) {
				order.push(key);
				result.push([key, onDemand.params?.[key] ?? '']);
			}
		}

		return result;
	});
</script>

{#snippet buttons(row: string[])}
	{#if editMode}
		<button
			class="icon-button hover:bg-gray-70"
			onclick={() => {
				const newParams = { ...(onDemand.params ?? {}) };
				delete newParams[row[0]];
				onChanged?.({
					...onDemand,
					params: newParams
				});
			}}
		>
			<Minus class="h-5 w-5" />
		</button>
		{#if row[0] !== ''}
			<button
				class="icon-button hover:bg-gray-70"
				onclick={() => {
					const newParams = { ...(onDemand.params ?? {}) };
					newParams[''] = '';
					onChanged?.({
						...onDemand,
						params: newParams
					});
				}}
			>
				<Plus class="h-5 w-5" />
			</button>
		{/if}
	{/if}
{/snippet}

<div class="flex flex-col gap-4">
	{#if Object.keys(onDemand.params ?? {}).length > 0}
		<h4 class="text-xl font-semibold">Arguments</h4>
		{#if editMode}
			<p class="text-sm text-gray">
				Reference these values in your steps using <span
					class="font-mono text-black dark:text-white">$VAR</span
				> syntax
			</p>
		{/if}
		<Table
			editable={editMode}
			header={['Name', 'Description']}
			placeholders={[
				'Enter name',
				'Add good description (essential to help AI form the correct values)...'
			]}
			{rows}
			{buttons}
			onCellBlur={(value, row, col) => {
				if (rows[row][col] !== value) {
					const newParams = { ...(onDemand.params ?? {}) };
					const oldKey = rows[row][0];
					const newKey = col === 0 ? value : rows[row][0];
					const newValue = col === 1 ? value : rows[row][1];

					const newOrder = rows.map((row) => row[0]);
					if (col === 0) {
						newOrder[row] = value;
					}
					order = newOrder;

					if (newKey !== oldKey) {
						delete newParams[oldKey];
					}
					newParams[newKey] = newValue;

					onChanged?.({
						...onDemand,
						params: newParams
					});
				}
			}}
		/>
	{/if}
</div>
