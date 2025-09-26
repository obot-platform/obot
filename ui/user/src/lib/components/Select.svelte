<script module lang="ts">
	export interface SelectProps<T> {
		id?: string;
		disabled?: boolean;
		options: T[];
		query?: string;
		selected?: string | number;
		multiple?: boolean;
		onSelect: (option: T, value?: string | number) => void;
		class?: string;
		classes?: {
			root?: string;
			clear?: string;
			option?: string;
			buttonContent?: string;
		};
		position?: 'top' | 'bottom';
		placeholder?: string;
		onClear?: (option?: T, value?: string | number) => void;
		buttonStartContent?: Snippet;
		onKeyDown?: (event: KeyboardEvent, params?: { query?: string; results?: T[] }) => void;
		searchable?: boolean;
	}
</script>

<script lang="ts" generics="T extends { id: string | number; label: string }">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { computePosition, flip as flipMiddleware } from '@floating-ui/dom';
	import { ChevronDown, X, Check } from 'lucide-svelte';
	import { type Snippet } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let {
		id,
		disabled,
		options,
		onSelect,
		selected = $bindable(),
		query = $bindable(),
		multiple = false,
		class: klass,
		classes,
		position = 'bottom',
		placeholder,
		onClear,
		buttonStartContent,
		onKeyDown,
		searchable
	}: SelectProps<T> = $props();

	const selectedValues = $derived.by(() => {
		if (multiple) {
			if (typeof selected === 'string') {
				const values =
					selected
						.split(',')
						.map((d) => d.trim())
						.filter(Boolean) ?? [];
				return values;
			}

			if (typeof selected === 'number') {
				return [selected] as number[];
			}

			return [];
		}

		return [selected].filter(Boolean) as (string | number)[];
	});

	let input = $state<HTMLInputElement>();
	let optionHighlightIndex = $state(-1);
	let popoverPlacement = $state<{ x: number; y: number }>();

	let availableOptions = $derived(
		options.filter((option) => option.label.toLowerCase().includes(query?.toLowerCase() ?? ''))
	);

	let selectedOptions = $derived(
		selectedValues
			.filter(Boolean)
			.map((selectedValue) => options.find((option) => option.id === selectedValue))
			.filter(Boolean) as T[]
	);

	let ref = $state<HTMLDivElement>();
	let popover = $state<HTMLDialogElement>();

	async function showPopover() {
		if (!ref || !popover) return;
		popover?.show();
		const { x, y } = await computePosition(ref, popover, {
			placement: position === 'top' ? 'top-start' : 'bottom-start',
			middleware: [flipMiddleware()]
		});
		popoverPlacement = { x, y };
	}

	function onInput(e: Event) {
		if (!popover?.open) {
			showPopover();
			input?.focus();
		}

		optionHighlightIndex = -1;
		query = (e.target as HTMLInputElement).value;
	}

	function handleSelect(option: T) {
		const key = option.id.toString();
		const isSelected = selectedValues.some((d) => d === key);

		if (multiple) {
			if (isSelected) {
				selected = selectedValues.filter((d) => d !== key).join(',');
			} else {
				selected = [key, ...selectedValues].join(',');
			}
		} else if (!isSelected) {
			selected = key;
		}

		query = '';
		onSelect?.(option, selected);
		popover?.close();
	}
</script>

<div class={classes?.root}>
	<div bind:this={ref} class="relative flex w-full items-center">
		<div
			{id}
			class={twMerge(
				'dark:bg-surface1 text-md flex min-h-10 w-full grow resize-none items-center gap-2 rounded-lg bg-white px-2 py-2 text-left shadow-sm',
				disabled && 'pointer-events-none cursor-not-allowed opacity-50',
				multiple && 'flex-wrap',
				klass
			)}
			onclick={() => {
				if (!popover?.open) {
					showPopover();
					input?.focus();
				}
			}}
			onkeydown={(e) => {
				if (e.key === 'Enter') {
					showPopover();
					input?.focus();
				}
			}}
			role="button"
			tabindex="0"
		>
			{#if multiple}
				<div class="flex flex-wrap items-center justify-start gap-2 whitespace-break-spaces">
					{#each selectedOptions as selectedOption (selectedOption.id)}
						<div
							class={twMerge(
								'text-md bg-surface3/50 dark:bg-surface2 inline-flex items-center gap-1 rounded-sm px-1',
								onClear && '',
								classes?.buttonContent
							)}
							in:fade={{ duration: 100 }}
							out:fade={{ duration: 0 }}
							animate:flip={{ duration: 100 }}
						>
							{#if buttonStartContent}
								{@render buttonStartContent()}
							{/if}

							<div class="flex flex-1 break-all">
								{selectedOption?.label ?? ''}
							</div>

							<div class="flex h-[22.5px] items-center place-self-start">
								<button
									class={twMerge(
										'button rounded-xs p-0 transition-colors duration-300',
										classes?.clear
									)}
									{disabled}
									onclick={(ev) => {
										ev.preventDefault();
										ev.stopImmediatePropagation();

										const filteredValues = selectedValues.filter((d) => d !== selectedOption.id);

										selected = filteredValues.join(',');

										onClear?.(selectedOption, selected);
									}}
								>
									<X class="size-4 " />
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}

			{#if multiple}
				{@render searchInput()}
			{:else}
				{#if buttonStartContent}
					{@render buttonStartContent()}
				{/if}
				{#if !searchable}
					<div class="w-full items-center gap-2 truncate">
						{selectedOptions[0]?.label ?? ''}
					</div>
				{:else}
					{@render searchInput()}
				{/if}
			{/if}

			<ChevronDown class="ml-auto size-5 flex-shrink-0 self-start" />
		</div>

		{#if onClear && !multiple}
			<button
				class={twMerge(
					'button absolute top-1/2 right-12 -translate-y-1/2 p-1 transition-colors duration-300',
					classes?.clear
				)}
				onclick={() => {
					onClear(undefined, '');
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
				optionHighlightIndex = -1;
			},
			true
		]}
		bind:this={popover}
		class={twMerge(
			'default-scrollbar-thin fixed top-auto right-auto bottom-auto left-auto z-10 max-h-[300px] overflow-y-auto rounded-sm'
		)}
		style={`top: ${popoverPlacement?.y ?? 0}px; left: ${popoverPlacement?.x ?? 0}px; width: ${ref?.clientWidth}px`}
	>
		{#if availableOptions.length === 0}
			<div class="px-4 py-2 font-light text-gray-400 dark:text-gray-600">No options available</div>
		{:else}
			{#each availableOptions as option, index (option.id)}
				{@const isSelected = selectedValues.some((d) => d === option.id)}
				{@const isHighlighted = optionHighlightIndex === index}

				<button
					class={twMerge(
						'dark:hover:bg-surface3/50 hover:bg-surface2/50 text-md flex w-full items-center px-4 py-2 text-left break-all transition-colors duration-100',
						isSelected &&
							'dark:bg-surface3/90 dark:hover:bg-surface3/50 bg-surface2/90 hover:bg-surface3/50',
						isHighlighted && 'dark:bg-surface3 bg-surface3',
						classes?.option
					)}
					onclick={(e) => {
						e.stopPropagation();
						handleSelect(option);

						optionHighlightIndex = -1;
					}}
				>
					<div>{option.label}</div>

					{#if multiple && isSelected}
						<Check class="ml-auto size-4" />
					{/if}
				</button>
			{/each}
		{/if}
	</dialog>
</div>

{#snippet searchInput()}
	<input
		class="grow bg-inherit focus:ring-0 focus:outline-none"
		{placeholder}
		bind:this={input}
		bind:value={query}
		oninput={onInput}
		onkeydown={(e) => {
			onKeyDown?.(e, { query: query, results: availableOptions });

			if (e.defaultPrevented) {
				return;
			}

			if ((e.key === 'ArrowUp' || e.key === 'ArrowDown') && popover?.open) {
				e.preventDefault();
				e.stopPropagation();

				if (e.key === 'ArrowDown') {
					optionHighlightIndex = Math.min(optionHighlightIndex + 1, availableOptions.length - 1);
				} else if (e.key === 'ArrowUp') {
					optionHighlightIndex = Math.max(optionHighlightIndex - 1, -1);
				}
			}

			if (
				multiple &&
				e.key === 'Backspace' &&
				selectedValues.length > 0 &&
				(query ?? '')?.length === 0
			) {
				selected = selectedValues.slice(0, -1).join(',');
			}

			if (e.key === 'Enter') {
				e.preventDefault();
				e.stopPropagation();
				const option = availableOptions[optionHighlightIndex];
				if (option) {
					handleSelect(option);
				}
			}
		}}
	/>
{/snippet}
