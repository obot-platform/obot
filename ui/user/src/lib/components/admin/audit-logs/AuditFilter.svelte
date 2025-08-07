<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import Select, { type SelectProps } from '$lib/components/Select.svelte';
	import type { FilterInput } from './AuditFilters.svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		filter: FilterInput;
		onSelect: SelectProps<{ id: string; label: string }>['onSelect'];
		onClearAll?: () => void;
	}

	let { filter, onSelect, onClearAll }: Props = $props();

	let options = $derived(filter.options ?? []);
</script>

<div class={twMerge('mb-2 flex flex-col gap-1', !options.length && 'opacity-50')}>
	<div class="flex items-center justify-between">
		<label for={filter.property} class="text-md font-light">
			By {filter.label}
		</label>

		{#if filter.selected}
			<button
				class="text-xs opacity-50 transition-opacity duration-200 hover:opacity-80 active:opacity-100"
				onclick={() => onClearAll?.()}
				in:fade={{ duration: 200 }}
				out:fade={{ duration: 100, delay: 200 }}
			>
				{#if filter.selected.toString()?.includes?.(',')}
					Clear All
				{:else}
					Clear
				{/if}
			</button>
		{/if}
	</div>

	<Select
		class="dark:border-surface3 bg-surface1 border border-transparent shadow-inner dark:bg-black"
		classes={{
			root: 'w-full',
			clear: 'hover:bg-surface3 bg-transparent'
		}}
		{options}
		bind:selected={filter.selected}
		multiple
		position="top"
		{onSelect}
	/>
</div>
