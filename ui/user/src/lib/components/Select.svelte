<script lang="ts" generics="T extends { id: string | number; label: string }">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { ChevronDown, X } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
		disabled?: boolean;
		options: T[];
		selected?: string | number;
		onSelect: (option: T) => void;
		class?: string;
		classes?: {
			root?: string;
			clear?: string;
			option?: string;
			buttonContent?: string;
		};
		position?: 'top' | 'bottom';
		onClear?: () => void;
		buttonStartContent?: Snippet;
	}

	const {
		id,
		disabled,
		options,
		onSelect,
		selected,
		class: klass,
		classes,
		position = 'bottom',
		onClear,
		buttonStartContent
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
	<div class="relative flex items-center">
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
			<span class={twMerge('text-md gap-1 truncate', onClear && 'pr-12', classes?.buttonContent)}>
				{#if buttonStartContent}
					{@render buttonStartContent()}
				{/if}
				{selectedOption?.label ?? ''}
			</span>
			<ChevronDown class="size-5 flex-shrink-0" />
		</button>
		{#if onClear}
			<button
				class={twMerge(
					'button absolute top-1/2 right-12 -translate-y-1/2 p-1 transition-colors duration-300',
					classes?.clear
				)}
				onclick={() => {
					onClear();
				}}
			>
				<X class="size-4" />
			</button>
		{/if}
	</div>
	<dialog
		use:clickOutside={[
			() => {
				popover?.close();
			},
			true
		]}
		bind:this={popover}
		class={twMerge(
			'default-scrollbar-thin absolute top-0 left-0 z-10 max-h-[300px] w-full overflow-y-auto rounded-sm',
			position === 'top' && 'translate-y-10',
			position === 'bottom' && '-translate-y-full'
		)}
	>
		{#each availableOptions as option (option.id)}
			<button
				class={twMerge(
					'dark:hover:bg-surface3 hover:bg-surface2 text-md w-full px-4 py-2 text-left',
					classes?.option
				)}
				onclick={(e) => {
					e.stopPropagation();
					onSelect(option);
					popover?.close();
				}}
			>
				{option.label}
			</button>
		{/each}
	</dialog>
</div>
