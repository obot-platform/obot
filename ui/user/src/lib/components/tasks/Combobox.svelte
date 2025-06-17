<script lang="ts">
	import { ChevronDown } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
	import type { HTMLInputTypeAttribute } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props {
		class?: string;
		value?: string;
		values: Record<string, string>;
		selected?: string;
		disabled?: boolean;
		type?: HTMLInputTypeAttribute | null | undefined;
		children?: Snippet<[]>;
		pre?: Snippet<[]>;
		post?: Snippet<[]>;
		onselect?: (value: string) => void | Promise<void>;
		onfocus?: (ev?: FocusEvent) => void;
		onblur?: (ev?: FocusEvent) => void;
		onkeydown?: (ev?: KeyboardEvent) => void;
		onclickout?: (event: MouseEvent) => void | null;
	}

	const popoverController = popover({
		placement: 'bottom-start'
	});

	const { ref, tooltip, toggle } = popoverController;

	let {
		value = $bindable(),
		values,
		selected,
		disabled = false,
		class: kclass = '',
		type = 'text',
		onselect,
		onfocus,
		onblur,
		onkeydown,
		onclickout,
		children,
		pre,
		post
	}: Props = $props();

	let inputElement = $state<HTMLElement>();

	$effect(() => {
		if (document.activeElement === inputElement) return;

		if (popoverController.open) {
			onfocus?.(undefined);
		} else {
			onblur?.();
		}
	});

	async function select(value: string) {
		await onselect?.(value);
		toggle();
	}
</script>

{#if disabled}
	{@const selectedValue = selected ?? value}
	<span
		class={twMerge(
			'combobox text-gray flex items-center justify-between gap-2 rounded-3xl p-3 px-4 capitalize dark:hover:bg-gray-900',
			kclass
		)}
	>
		{selectedValue ? values[selectedValue] : values[''] || ''}
		<ChevronDown class="text-gray" />
	</span>
{:else}
	<button
		use:ref
		use:clickOutside={(ev) => onclickout?.(ev as MouseEvent)}
		onclick={() => {
			toggle();
		}}
		class={twMerge(
			'combobox hover:bg-gray-70 relative flex items-center justify-between gap-2 rounded-3xl px-4 capitalize focus-within:outline dark:hover:bg-gray-900',
			kclass
		)}
	>
		{@render pre?.()}

		<input
			bind:this={inputElement}
			class="w-full min-w-0 flex-1 bg-transparent py-3 outline-none"
			{type}
			bind:value={
				() => value ?? '',
				(v) => {
					onselect?.(v);
				}
			}
			onclick={(ev) => {
				if (popoverController.open) {
					ev.stopPropagation();
				}
			}}
			{onfocus}
			{onblur}
			{onkeydown}
		/>

		{@render children?.()}

		{@render post?.()}

		<ChevronDown class="z-10" />
	</button>
	<div use:tooltip class="min-w-[150px] rounded-3xl bg-white shadow dark:bg-gray-900">
		<ul>
			{#each Object.keys(values) as key}
				{@const value = values[key]}
				{@const isSelected = key === (selected ?? value)}

				<li>
					<button
						class:bg-gray-70={isSelected}
						class:dark:bg-gray-800={isSelected}
						class="w-full px-6 py-2.5 text-start capitalize hover:bg-gray-100 dark:hover:bg-gray-800"
						onclick={() => select(key)}
					>
						{value}
					</button>
				</li>
			{/each}
		</ul>
	</div>
{/if}

<style lang="postcss">
	li:first-child button {
		border-top-left-radius: 1.5rem;
		border-top-right-radius: 1.5rem;
		padding-top: 1rem;
	}
	li:last-child button {
		border-bottom-left-radius: 1.5rem;
		border-bottom-right-radius: 1.5rem;
		padding-bottom: 1rem;
	}

	input::-webkit-outer-spin-button,
	input::-webkit-inner-spin-button {
		-webkit-appearance: none;
		margin: 0;
	}

	input[type='number'] {
		-moz-appearance: textfield;
	}
</style>
