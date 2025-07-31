<script module lang="ts">
	export interface SelectProps<T> {
		id?: string;
		disabled?: boolean;
		options: T[];
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
		onClear?: (option?: T, value?: string | number) => void;
		buttonStartContent?: Snippet;
	}
</script>

<script lang="ts" generics="T extends { id: string | number; label: string }">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { ChevronDown, X, Check } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let {
		id,
		disabled,
		options,
		onSelect,
		selected = $bindable(),
		multiple = false,
		class: klass,
		classes,
		position = 'bottom',
		onClear,
		buttonStartContent
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

	let search = $state('');

	let availableOptions = $derived(
		options.filter((option) => option.label.toLowerCase().includes(search.toLowerCase()))
	);

	let selectedOptions = $derived(
		selectedValues
			.map((selectedValue) => options.find((option) => option.id === selectedValue))
			.filter(Boolean) as T[]
	);

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
				'dark:bg-surface1 text-md flex min-h-10 w-full grow resize-none items-center justify-between gap-2 rounded-lg bg-white px-2 py-2 text-left shadow-sm',
				disabled && 'cursor-not-allowed opacity-50',
				klass
			)}
			placeholder="Enter a task"
			oninput={onInput}
			onclick={() => {
				if (popover?.open) {
					popover?.close();
				} else {
					popover?.show();
				}
			}}
		>
			<div class="flex flex-1 flex-wrap items-center justify-start">
				<div class="flex flex-wrap items-center justify-start gap-2 whitespace-break-spaces">
					{#if multiple}
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
									<div
										class={twMerge(
											'button rounded-xs p-0 transition-colors duration-300',
											classes?.clear
										)}
										role="button"
										tabindex="0"
										onclick={(ev) => {
											ev.preventDefault();
											ev.stopImmediatePropagation();

											const filteredValues = selectedValues.filter((d) => d !== selectedOption.id);

											selected = filteredValues.join(',');

											onClear?.(selectedOption, selected);
										}}
										onkeydown={() => {}}
									>
										<X class="size-4 " />
									</div>
								</div>
							</div>
						{/each}
					{:else}
						<div class="flex items-center gap-2">
							{#if buttonStartContent}
								{@render buttonStartContent()}
							{/if}
							<div>{selectedOptions[0]?.label ?? ''}</div>
						</div>
					{/if}
				</div>
			</div>

			<ChevronDown class="size-5 flex-shrink-0 self-start" />
		</button>

		{#if onClear}
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
			},
			true
		]}
		bind:this={popover}
		class={twMerge(
			'default-scrollbar-thin absolute top-0 left-0 z-10 max-h-[300px] w-full overflow-y-auto rounded-sm',
			position === 'top' && 'top-full translate-y-1',
			position === 'bottom' && '-translate-y-full'
		)}
	>
		{#if availableOptions.length === 0}
			<div class="px-4 py-2 font-light text-gray-400 dark:text-gray-600">No options available</div>
		{:else}
			{#each availableOptions as option (option.id)}
				{@const isSelected = selectedValues.some((d) => d === option.id)}

				<button
					class={twMerge(
						'dark:hover:bg-surface3 hover:bg-surface2 text-md flex w-full items-center px-4 py-2 text-left break-all transition-colors duration-100',
						isSelected && 'dark:bg-surface1 bg-surface2',
						classes?.option
					)}
					onclick={(e) => {
						e.stopPropagation();

						const key = option.id.toString();

						if (isSelected) {
							selected = selectedValues.filter((d) => d !== key).join(',');
						} else {
							selected = [key, ...selectedValues].join(',');
						}

						onSelect(option, selected);

						popover?.close();
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
