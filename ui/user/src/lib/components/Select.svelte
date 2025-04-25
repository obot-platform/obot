<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { ChevronDown } from 'lucide-svelte';

	interface Option {
		id: string | number;
		label: string;
		[key: string]: any;
	}

	interface Props {
		options: Option[];
		selected?: string;
		onSelect: (option: Option) => void;
	}

	const { options, onSelect, selected }: Props = $props();

	let search = $state('');
	let availableOptions = $derived(
		options.filter((option) => option.label.toLowerCase().includes(search.toLowerCase()))
	);

	let selectedOption = $derived(options.find((option) => option.id === selected));

	let popover = $state<HTMLDialogElement>();

	function onInput(e: Event) {
		search = (e.target as HTMLInputElement).value;
	}
</script>

<div class="relative">
	<button
		class="dark:bg-surface1 text-md flex min-h-10 w-full grow resize-none items-center justify-between rounded-lg bg-white px-4 py-2 shadow-sm"
		placeholder="Enter a task"
		oninput={onInput}
		onmousedown={() => popover?.show()}
	>
		<span class="text-md">{selectedOption?.label ?? ''}</span>
		<ChevronDown class="size-5" />
	</button>
	<dialog
		use:clickOutside={() => popover?.close()}
		bind:this={popover}
		class="absolute top-0 left-0 z-10 w-full translate-y-10 rounded-sm"
	>
		{#each availableOptions as option}
			<button
				class="hover:bg-surface2 text-md w-full px-4 py-2 text-left"
				onclick={() => {
					onSelect(option);
					popover?.close();
				}}
			>
				{option.label}
			</button>
		{/each}
	</dialog>
</div>
