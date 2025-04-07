<script lang="ts">
	import { SearchIcon } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onChange: (value: string) => void;
		class?: string;
		placeholder?: string;
		onMouseDown?: (e: MouseEvent) => void;
	}

	let { onChange, class: klass, placeholder = 'Search Obots...', onMouseDown }: Props = $props();
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
</script>

<div class="relative w-full">
	<input
		bind:this={input}
		type="text"
		{placeholder}
		class={twMerge(
			'peer bg-surface1 w-full rounded-xl border-none px-2.5 py-3 pl-12 ring-2 ring-transparent transition-all duration-200 hover:ring-2 hover:ring-blue-500 focus:w-full focus:ring-2 focus:ring-blue-500 focus:outline-hidden',
			klass
		)}
		oninput={search}
		onmousedown={onMouseDown}
	/>
	<button
		class="text-gray absolute top-1/2 left-4 -translate-y-1/2 peer-focus:text-blue-500"
		onclick={() => input?.focus()}
	>
		<SearchIcon />
	</button>
</div>
