<script module lang="ts">
	export interface SelectProps<T> {
		id?: string;
		disabled?: boolean;
		readonly?: boolean;
		options: T[];
		query?: string;
		selected?: string | number;
		multiple?: boolean;
		onSelect?: (option: T, value?: string | number) => void;
		class?: string;
		classes?: {
			root?: string;
			clear?: string;
			option?: string;
			buttonContent?: string;
		};
		position?: 'top' | 'bottom';
		placeholder?: string;
		clearAllLabel?: string;
		onClear?: (option?: T, value?: string | number) => void;
		onClearAll?: () => void;
		buttonStartContent?: Snippet;
		onKeyDown?: (event: KeyboardEvent, params?: { query?: string; results?: T[] }) => void;
		searchable?: boolean;
	}
</script>

<script lang="ts" generics="T extends { id: string | number; label: string }">
	import { ChevronDown, X, Check } from 'lucide-svelte';
	import { type Snippet } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let {
		id = 'select',
		disabled,
		readonly,
		options,
		onSelect,
		selected = $bindable(),
		query = $bindable(),
		multiple = false,
		class: klass,
		classes,
		position = 'bottom',
		placeholder,
		clearAllLabel,
		onClear,
		onClearAll,
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

	let availableOptions = $derived(
		options.filter((option) => option.label.toLowerCase().includes(query?.toLowerCase() ?? ''))
	);

	let selectedOptions = $derived(
		selectedValues
			.filter(Boolean)
			.map((selectedValue) => options.find((option) => option.id === selectedValue))
			.filter(Boolean) as T[]
	);

	let popover = $state<HTMLDivElement>();

	function onInput(e: Event) {
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
		popover?.hidePopover();
	}
</script>

<div class={twMerge(classes?.root, (readonly || disabled) && 'pointer-events-none')}>
	<div class="relative flex w-full items-center">
		<div
			{id}
			role="combobox"
			aria-haspopup="listbox"
			aria-expanded={popover?.matches(':popover-open') ?? false}
			aria-controls={`${id}-popover`}
			class={twMerge(
				'dark:bg-surface1 text-md bg-background flex min-h-10 w-full grow cursor-pointer resize-none items-center gap-2 rounded-lg px-2 py-2 text-left shadow-sm',
				disabled && 'pointer-events-none cursor-default opacity-50',
				multiple && 'flex-wrap',
				klass
			)}
			style={`anchor-name: --${id}-anchor;`}
			tabindex={readonly || disabled ? -1 : 0}
			onclick={() => popover?.togglePopover()}
			onkeydown={(e) => {
				if (e.key === 'Enter' || e.key === ' ') {
					e.preventDefault();
					popover?.togglePopover();
				}
			}}
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
					{#if onClearAll}
						<button
							class={twMerge(
								'bg-surface3/50 dark:bg-surface2 hover:bg-surface3 dark:hover:bg-surface3 inline-flex rounded-sm px-1 text-sm transition-colors duration-300',
								classes?.buttonContent
							)}
							onclick={(ev) => {
								ev.preventDefault();
								ev.stopImmediatePropagation();
								onClearAll?.();
							}}
						>
							{clearAllLabel || 'Clear All'}
						</button>
					{/if}
				</div>
			{/if}

			{#if multiple}
				{#if !readonly}
					{@render searchInput()}
				{/if}
			{:else}
				{#if buttonStartContent}
					{@render buttonStartContent()}
				{/if}
				{#if !searchable}
					<div class="min-w-0 flex-1 items-center gap-2 truncate">
						{selectedOptions[0]?.label ?? ''}
					</div>
				{:else if !readonly}
					{@render searchInput()}
				{/if}
			{/if}

			<ChevronDown class="size-5 shrink-0 self-start" />
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

	<div
		bind:this={popover}
		popover
		id={`${id}-popover`}
		style={`position-anchor: --${id}-anchor; width: anchor-size(width); position-area: ${position}; position-try-fallbacks: flip-block;`}
		class={twMerge(
			'default-scrollbar-thin dropdown-menu max-h-[300px] overflow-y-auto',
			position === 'top' ? 'rounded-t-sm rounded-b-none' : 'rounded-t-none rounded-b-sm'
		)}
	>
		{#if availableOptions.length === 0}
			<div class="text-on-surface1 px-4 py-2 font-light">No options available</div>
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
					type="button"
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
	</div>
</div>

{#snippet searchInput()}
	<input
		class="min-w-0 flex-1 bg-inherit focus:ring-0 focus:outline-none"
		{placeholder}
		bind:this={input}
		bind:value={query}
		{disabled}
		{readonly}
		oninput={onInput}
		onkeydown={(e) => {
			onKeyDown?.(e, { query: query, results: availableOptions });

			if (e.defaultPrevented) {
				return;
			}

			if ((e.key === 'ArrowUp' || e.key === 'ArrowDown') && popover?.matches(':popover-open')) {
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
