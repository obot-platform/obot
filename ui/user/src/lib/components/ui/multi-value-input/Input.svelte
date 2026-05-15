<script module lang="ts">
	type ClearEventHandler =
		| (() => void)
		| ((ev: Event) => void)
		| ((ev: Event, value: string) => void);

	export interface SelectProps {
		id?: string;
		value?: string | string[];
		labels?: Record<string, string>;
		disabled?: boolean;
		readonly?: boolean;
		class?: string;
		classes?: {
			chip?: string;
			clearButton?: string;
		};
		placeholder?: string;
		onclear?: ClearEventHandler;
	}
</script>

<script lang="ts">
	import { X } from 'lucide-svelte';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let {
		id,
		disabled,
		readonly,
		value = $bindable<string | string[]>(''),
		labels,
		class: klass,
		classes,
		placeholder,
		onclear
	}: SelectProps = $props();

	let values = $derived.by(() => {
		if (!value) return [];
		if (Array.isArray(value)) {
			return value.map((v) => v.trim()).filter(Boolean);
		}
		return value
			.trim()
			.split(',')
			.map((v) => v.trim())
			.filter(Boolean);
	});

	let input = $state<HTMLInputElement>();
	let text = $state('');

	function assignValues(nextValues: string[]) {
		value = Array.isArray(value) ? nextValues : nextValues.join(',');
	}
</script>

<div
	{id}
	class={twMerge(
		'dark:bg-base-200 text-md bg-base-100 flex min-h-10 w-full grow resize-none flex-wrap items-center gap-2 rounded-lg px-2 py-2 text-left shadow-inner',
		disabled && 'pointer-events-none cursor-default opacity-50',
		readonly && 'pointer-events-none',
		klass
	)}
>
	{#if values.length}
		<div class="flex flex-wrap items-center justify-start gap-2 whitespace-break-spaces">
			{#each values as v (v)}
				<div
					class={twMerge(
						'text-md bg-base-400/50 dark:bg-base-300 inline-flex items-center gap-1 rounded-sm px-1',
						classes?.chip
					)}
					in:fade={{ duration: 100 }}
					out:fade={{ duration: 0 }}
					animate:flip={{ duration: 100 }}
				>
					<div class="flex flex-1 break-all">
						{labels?.[v] ?? v ?? ''}
					</div>

					<div class="flex h-[22.5px] items-center place-self-start">
						<button
							class={twMerge(
								'btn btn-circle btn-secondary size-4 btn-xs border-transparent text-muted-content hover:text-base-content',
								classes?.clearButton
							)}
							{disabled}
							onclick={(ev) => {
								ev.preventDefault();
								ev.stopPropagation();

								assignValues(values.filter((d) => d !== v));
							}}
						>
							<X class="size-3" />
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<input
		class="grow bg-transparent text-current placeholder:text-current placeholder:opacity-50 focus:ring-0 focus:outline-none"
		{placeholder}
		bind:this={input}
		bind:value={text}
		type="text"
		{disabled}
		{readonly}
		onkeydown={(ev) => {
			if (ev.defaultPrevented) {
				return;
			}

			switch (ev.key) {
				case 'Backspace': {
					if (ev.key === 'Backspace') {
						// Remove the last selected value
						if (values.length === 0) break;
						if (text.length) break;

						assignValues(values.slice(0, -1));
					}

					break;
				}
				case ',':
				case 'Enter': {
					ev.preventDefault();
					ev.stopPropagation();

					const trimmedText = text?.trim();

					if (!trimmedText) break;

					if (values.includes(trimmedText)) {
						text = '';
						break;
					}

					assignValues([...values, trimmedText]);

					text = '';

					break;
				}
			}
		}}
	/>

	{#if values.length || text.length}
		<button
			transition:fade={{ duration: 100 }}
			class={twMerge(
				'bg-base-400/50 hover:bg-base-400/70 active:bg-base-400/80 rounded-sm p-0.5 transition-colors duration-300',
				classes?.clearButton
			)}
			type="button"
			onclick={(ev) => {
				onclear?.(ev, '');

				if (ev.defaultPrevented) return;

				if (text.length) {
					text = '';
					return;
				}

				assignValues([]);
			}}
		>
			<X class="size-4" />
		</button>
	{/if}
</div>
