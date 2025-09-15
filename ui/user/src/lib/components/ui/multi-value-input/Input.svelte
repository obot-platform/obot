<script module lang="ts">
	type ClearEventHandler =
		| (() => void)
		| ((ev: Event) => void)
		| ((ev: Event, value: string) => void);

	export interface SelectProps {
		id?: string;
		value?: string;
		disabled?: boolean;
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
	import { Plus, X } from 'lucide-svelte';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let {
		id,
		disabled,
		value = $bindable(''),
		class: klass,
		classes,
		placeholder,
		onclear
	}: SelectProps = $props();

	let values = $derived.by(() => {
		if (!value) return [];
		return value
			.trim()
			.split(',')
			.map((v) => v.trim())
			.filter(Boolean);
	});

	let input = $state<HTMLInputElement>();
	let text = $state('');

	const actions = $derived.by(() => {
		const array = [];

		array.push(addButton);

		if (values.length || text.length) {
			array.push(clearButton);
		}

		return array;
	});
</script>

<div
	{id}
	class={twMerge(
		'dark:bg-surface1 text-md bg-surface-1 flex min-h-10 w-full grow resize-none items-center gap-2 rounded-lg px-2 py-2 text-left shadow-inner',
		disabled && 'pointer-events-none cursor-not-allowed opacity-50',
		klass
	)}
>
	{#if values.length}
		<div class="flex flex-wrap items-center justify-start gap-2 whitespace-break-spaces">
			{#each values as v (v)}
				<div
					class={twMerge(
						'text-md bg-surface3/50 dark:bg-surface2 inline-flex items-center gap-1 rounded-sm px-1',
						classes?.chip
					)}
					in:fade={{ duration: 100 }}
					out:fade={{ duration: 0 }}
					animate:flip={{ duration: 100 }}
				>
					<div class="flex flex-1 break-all">
						{v ?? ''}
					</div>

					<div class="flex h-[22.5px] items-center place-self-start">
						<button
							class={twMerge(
								'button rounded-xs p-0 transition-colors duration-300',
								classes?.clearButton
							)}
							{disabled}
							onclick={(ev) => {
								ev.preventDefault();
								ev.stopPropagation();

								value = values.filter((d) => d !== v).join(',');
							}}
						>
							<X class="size-4 " />
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<input
		class="grow bg-inherit focus:ring-0 focus:outline-none"
		{placeholder}
		bind:this={input}
		bind:value={text}
		type="text"
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

						value = values.slice(0, -1).join(',');
					}

					break;
				}
				case 'Enter': {
					ev.preventDefault();
					ev.stopPropagation();

					const trimmedText = text?.trim();

					if (!trimmedText) break;

					if (values.includes(trimmedText)) {
						text = '';
						break;
					}

					value = [...values, trimmedText].join(',');

					text = '';

					break;
				}
			}
		}}
	/>

	{#each actions as snp (snp)}
		<div animate:flip={{ duration: 100 }}>
			{@render snp()}
		</div>
	{/each}
</div>

{#snippet addButton()}
	<button
		class={twMerge(
			'bg-surface3/50 hover:bg-surface3/70 active:bg-surface3/80 rounded-sm p-1 transition-colors duration-300',
			classes?.clearButton
		)}
		type="button"
		onclick={(ev) => {
			onclear?.(ev, '');

			if (ev.defaultPrevented) return;

			const trimmedText = text?.trim();

			if (!trimmedText) return;

			value = [...values, trimmedText].join(',');

			text = '';
		}}
	>
		<Plus class="size-4" />
	</button>
{/snippet}

{#snippet clearButton()}
	<button
		class={twMerge(
			'bg-surface3/50 hover:bg-surface3/70 active:bg-surface3/80 rounded-sm p-1 transition-colors duration-300',
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

			value = '';
		}}
	>
		<X class="size-4" />
	</button>
{/snippet}
