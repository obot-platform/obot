<script lang="ts">
	import { X } from 'lucide-svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { fade } from 'svelte/transition';
	import { createEventDispatcher } from 'svelte';

	interface Option {
		label: string;
		value: string;
	}

	interface Props {
		id?: string;
		options: Option[];
		value: Option[];
		creatable?: boolean;
		side?: 'top' | 'bottom';
		placeholder?: string;
		onChange?: (value: Option[]) => void;
	}

	const dispatch = createEventDispatcher<{
		change: Option[];
	}>();

	let {
		id,
		options = [],
		value = [],
		creatable = false,
		side = 'bottom',
		placeholder = 'Add items...',
		onChange
	}: Props = $props();

	let inputValue = $state('');
	let isOpen = $state(false);
	let dropdown: HTMLDivElement;
	let input: HTMLInputElement;

	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === 'Enter' && inputValue.trim()) {
			event.preventDefault();
			addValue(inputValue.trim());
			inputValue = '';
		} else if (event.key === 'Backspace' && !inputValue && value.length > 0) {
			removeValue(value[value.length - 1].value);
		}
	}

	function addValue(val: string) {
		if (!val || value.some((v) => v.value === val)) return;
		const newValue = [...value, { label: val, value: val }];
		dispatch('change', newValue);
		onChange?.(newValue);
		inputValue = '';
	}

	function removeValue(val: string) {
		const newValue = value.filter((v) => v.value !== val);
		dispatch('change', newValue);
		onChange?.(newValue);
	}

	function handleClickOutside() {
		isOpen = false;
	}

	function focusInput() {
		input?.focus();
	}
</script>

<div class="relative" bind:this={dropdown} use:clickOutside={handleClickOutside}>
	<div
		class="flex min-h-[38px] w-full flex-wrap items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-sm focus-within:border-blue-500 focus-within:ring-1 focus-within:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
		onclick={focusInput}
		onkeydown={(e) => e.key === 'Enter' && focusInput()}
		role="button"
		tabindex="0"
	>
		{#each value as item (item.label)}
			<div
				class="flex items-center gap-1 rounded bg-gray-100 px-2 py-0.5 text-sm dark:bg-gray-600"
				transition:fade
			>
				<span>{item.label}</span>
				<button
					type="button"
					class="icon-button min-h-auto min-w-auto p-0.5"
					onclick={() => removeValue(item.value)}
				>
					<X class="size-3" />
				</button>
			</div>
		{/each}
		<input
			bind:this={input}
			type="text"
			{id}
			class="flex-1 bg-transparent outline-none"
			placeholder={value.length === 0 ? placeholder : ''}
			bind:value={inputValue}
			onkeydown={handleKeyDown}
			onfocus={() => (isOpen = true)}
		/>
	</div>

	{#if isOpen && (creatable || options.length > 0)}
		<div
			class="absolute z-10 mt-1 max-h-60 w-full overflow-auto rounded-md border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-600 dark:bg-gray-700"
			class:top-0={side === 'bottom'}
			class:bottom-full={side === 'top'}
			class:mb-1={side === 'top'}
		>
			{#if creatable && inputValue && !options.some((o) => o.value === inputValue)}
				<button
					type="button"
					class="w-full px-3 py-1 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-600"
					onclick={() => addValue(inputValue)}
				>
					Add "{inputValue}"
				</button>
			{/if}
			{#each options.filter((o) => !value.some((v) => v.value === o.value)) as option (option.label)}
				<button
					type="button"
					class="w-full px-3 py-1 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-600"
					onclick={() => addValue(option.value)}
				>
					{option.label}
				</button>
			{/each}
		</div>
	{/if}
</div>
