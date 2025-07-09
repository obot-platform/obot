<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { ChevronDown } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Option {
		id: string | number;
		label: string;
	}

	interface Props {
		id?: string;
		disabled?: boolean;
		options: Option[];
		selected?: string | number;
		onSelect: (option: Option) => void;
		class?: string;
		classes?: {
			root?: string;
		};
		position?: 'top' | 'bottom';
	}

	const {
		id,
		disabled,
		options,
		onSelect,
		selected,
		class: klass,
		classes,
		position = 'bottom'
	}: Props = $props();

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

<div class={twMerge('relative', classes?.root)}>
	<button
		{id}
		{disabled}
		class={twMerge(
			'dark:bg-surface1 text-md flex min-h-10 w-full grow resize-none items-center justify-between rounded-lg bg-white px-4 py-2 text-left shadow-sm',
			disabled && 'cursor-not-allowed opacity-50',
			klass
		)}
		placeholder="Enter a task"
		oninput={onInput}
		onmousedown={() => {
			if (popover?.open) {
				popover?.close();
			} else {
				popover?.show();
			}
		}}
	>
		<span class="text-md grow truncate">{selectedOption?.label ?? ''}</span>
		<ChevronDown class="size-5 flex-shrink-0" />
	</button>
	<dialog
		use:clickOutside={() => popover?.close()}
		bind:this={popover}
		class={twMerge(
			'default-scrollbar-thin absolute top-0 left-0 z-10 max-h-[300px] w-full overflow-y-auto rounded-sm',
			position === 'top' && 'translate-y-10',
			position === 'bottom' && '-translate-y-full'
		)}
	>
		{#each availableOptions as option (option.id)}
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
