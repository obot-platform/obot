<script lang="ts">
	import { SearchIcon } from 'lucide-svelte';
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

	function search(e: Event) {
		const value = (e.target as HTMLInputElement).value;

		// Clear previous timeout
		if (searchTimeout) clearTimeout(searchTimeout);

		// Set new timeout for debounced search
		searchTimeout = setTimeout(() => {
			onChange(value);
		}, 300);
	}

	export function clear() {
		if (input) {
			input.value = '';
		}
		onChange('');
	}
</script>

<div class="relative w-full" {...restProps}>
	<input
		bind:this={input}
		{value}
		type="text"
		{placeholder}
		class={twMerge(
			'bg-surface1 peer hover:ring-primary focus:ring-primary w-full rounded-lg px-2.5 py-3 pl-12 ring-2 ring-transparent transition-all duration-200 hover:ring-2 focus:w-full focus:ring-2 focus:outline-hidden',
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
