<script lang="ts">
	import { SearchIcon } from '@lucide/svelte';
	import { onDestroy } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onChange: (value: string) => void;
		class?: string;
		placeholder?: string;
		onMouseDown?: (e: MouseEvent) => void;
		onMouseUp?: (e: MouseEvent) => void;
		compact?: boolean;
		value?: string;
	}

	let {
		onChange,
		class: klass,
		placeholder = 'Search Projects...',
		onMouseDown,
		onMouseUp,
		compact,
		value = '',
		...restProps
	}: Props = $props();
	let searchTimeout: ReturnType<typeof setTimeout>;
	let input = $state<HTMLInputElement | null>(null);
	// svelte-ignore state_referenced_locally
	let displayValue = $state(value);
	let editRevision = 0;
	let lastEmission: { value: string; revision: number } | undefined;

	$effect(() => {
		const nextValue = value;
		if (lastEmission?.value === nextValue) {
			const emission = lastEmission;
			lastEmission = undefined;
			if (editRevision > emission.revision) return;
		} else {
			lastEmission = undefined;
		}

		displayValue = nextValue;
		if (searchTimeout) clearTimeout(searchTimeout);
	});

	function search(e: Event) {
		const value = (e.target as HTMLInputElement).value;
		const revision = ++editRevision;

		// Clear previous timeout
		if (searchTimeout) clearTimeout(searchTimeout);

		// Set new timeout for debounced search
		searchTimeout = setTimeout(() => {
			lastEmission = { value, revision };
			onChange(value);
		}, 300);
	}

	export function clear() {
		if (searchTimeout) clearTimeout(searchTimeout);
		lastEmission = { value: '', revision: ++editRevision };
		displayValue = '';
		onChange('');
	}

	onDestroy(() => clearTimeout(searchTimeout));
</script>

<div class="relative w-full" {...restProps}>
	<input
		bind:this={input}
		bind:value={displayValue}
		type="text"
		{placeholder}
		class={twMerge(
			'bg-base-200 peer hover:ring-primary focus:ring-primary w-full rounded-sm px-2.5 py-3 pl-12 ring-2 ring-transparent transition-all duration-200 hover:ring-2 focus:w-full focus:ring-2 focus:outline-hidden',
			compact && 'py-2 pl-8',
			klass
		)}
		oninput={search}
		onmousedown={onMouseDown}
		onmouseup={onMouseUp}
	/>
	<button
		class={twMerge(
			'text-gray peer-focus:text-primary absolute top-1/2 left-4 -translate-y-1/2',
			compact && 'left-2.5'
		)}
		onclick={() => input?.focus()}
	>
		<SearchIcon class={twMerge(compact && 'size-4')} />
	</button>
</div>
