<script module lang="ts">
	export type FilterOption = {
		label: string;
		id: string;
	};

	export type FilterInput = {
		label: string;
		property: FilterKey;
		selected?: string | number | null;
		default?: string | number | null;
		options: FilterOption[];
		readonly disabled: boolean;
	};
</script>

<script lang="ts">
	import { flip } from 'svelte/animate';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import Select, { type SelectProps } from '$lib/components/Select.svelte';
	import type { FilterKey } from './FiltersDrawer.svelte';

	interface Props {
		filter: FilterInput;
		optionsSort?: (a: FilterOption, b: FilterOption) => number;
		onSelect: SelectProps<{ id: string; label: string }>['onSelect'];
		onClearAll?: () => void;
		onReset?: () => void;
	}

	let {
		filter,
		optionsSort = (a: FilterOption, b: FilterOption) => a.label.localeCompare(b.label),
		onSelect,
		onClearAll,
		onReset
	}: Props = $props();

	let options = $derived([...(filter?.options ?? [])].sort(optionsSort));

	const value = $derived(
		filter.selected === null ? (filter.default ?? '') : (filter.selected ?? '')
	);

	const hasDefaultValue = $derived(!!filter.default);

	const shouldShowResetButton = $derived(
		hasDefaultValue && filter.selected !== null && filter.default !== filter.selected
	);
	const shouldShowClearButton = $derived(!!value);

	const actions = $derived(
		[
			shouldShowResetButton
				? {
						id: 'reset',
						label: 'Reset',
						onclick: () => onReset?.(),
						class: 'text-primary opacity-80 hover:opacity-90 active:opacity-100'
					}
				: undefined,
			shouldShowClearButton
				? {
						id: 'clear',
						label: ['Clear', value?.toString()?.includes?.(',') ? 'All' : '']
							.filter(Boolean)
							.join(' '),
						onclick: () => onClearAll?.(),
						class: 'opacity-50 hover:opacity-80 active:opacity-100'
					}
				: undefined
		].filter(Boolean) as {
			id: string;
			label: string;
			onclick: () => void;
			class: string;
		}[]
	);
</script>

<div
	class={twMerge(
		'mb-2 flex flex-col gap-1',
		(filter.disabled || !options.length) && 'pointer-events-none opacity-50'
	)}
>
	<div class="flex items-center justify-between">
		<label for={filter.property} class="text-md font-light">
			By {filter.label}
		</label>

		<flex class="flex gap-4">
			{#each actions as action (action.id)}
				<button
					class={twMerge(action.class, 'text-xs whitespace-nowrap transition-opacity duration-200')}
					onclick={action.onclick}
					in:slide={{ duration: 100, axis: 'x' }}
					out:slide={{ duration: 100, delay: 200, axis: 'x' }}
					animate:flip={{ duration: 200 }}
				>
					{action.label}
				</button>
			{/each}
		</flex>
	</div>

	<Select
		id={`filter-${filter.property}`}
		class="dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner"
		classes={{
			root: 'w-full',
			clear: 'hover:bg-surface3 bg-transparent'
		}}
		{options}
		bind:selected={
			() => value,
			(v) => {
				filter.selected = v;
			}
		}
		multiple
		{onSelect}
		position="top"
	/>
</div>
