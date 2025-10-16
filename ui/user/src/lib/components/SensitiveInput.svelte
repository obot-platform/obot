<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Eye, EyeOff } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		name: string;
		value?: string;
		error?: boolean;
		oninput?: () => void;
		textarea?: boolean;
		disabled?: boolean;
		growable?: boolean;
	}

	let {
		name,
		value = $bindable(''),
		error,
		oninput,
		textarea,
		disabled,
		growable
	}: Props = $props();
	let showSensitive = $state(false);
	let textareaElement = $state<HTMLElement>();
	let maskedTextarea = $state<HTMLElement>();

	function getMaskedValue(text: string): string {
		return text.replace(/[^\s]/g, '•').replace(/\n/g, '<br>');
	}

	function handleInput(ev: Event) {
		const input = ev.target as HTMLInputElement;
		value = input.value;
		oninput?.();
	}

	function toggleVisibility(ev: MouseEvent) {
		ev.preventDefault();
		showSensitive = !showSensitive;

		if (showSensitive) {
			textareaElement?.focus();
		}
	}
</script>

<div class="relative flex grow items-center">
	{#if textarea}
		<div class="relative flex min-h-[60px] w-full flex-col leading-5">
			{#if growable}
				<input type="text" {name} {disabled} {value} hidden />
				<div
					bind:this={textareaElement}
					data-1p-ignore
					id={name}
					contenteditable="plaintext-only"
					class={twMerge(
						'text-input-filled base min-h-full w-full flex-1 pr-10 font-mono',
						error && 'border-red-500 bg-red-500/20 text-red-500 ring-red-500 focus:ring-1',
						growable && 'resize-y',
						disabled && 'opacity-50',
						!showSensitive ? 'hide' : ''
					)}
					onscroll={(ev) => {
						if (!showSensitive && maskedTextarea) {
							maskedTextarea.scrollTop = ev.currentTarget.scrollTop;
							maskedTextarea.scrollLeft = ev.currentTarget.scrollLeft;
						}
					}}
					bind:innerText={
						() => value,
						(v) => {
							value = v;
							oninput?.();
						}
					}
				></div>
			{:else}
				<textarea
					bind:this={textareaElement}
					data-1p-ignore
					id={name}
					{name}
					{disabled}
					contenteditable={growable ? true : undefined}
					class={twMerge(
						'text-input-filled base min-h-full w-full flex-1 pr-10 font-mono',
						error && 'border-red-500 bg-red-500/20 text-red-500 ring-red-500 focus:ring-1',
						!showSensitive ? 'hide' : ''
					)}
					onscroll={(ev) => {
						if (!showSensitive && maskedTextarea) {
							maskedTextarea.scrollTop = ev.currentTarget.scrollTop;
							maskedTextarea.scrollLeft = ev.currentTarget.scrollLeft;
						}
					}}
					bind:value={
						() => value,
						(v) => {
							value = v;
							oninput?.();
						}
					}
				></textarea>
			{/if}

			{#if !showSensitive}
				<!-- Masked overlay textarea -->
				<div
					bind:this={maskedTextarea}
					tabindex="-1"
					class={twMerge(
						'text-input-filled layer-1 pointer-events-none absolute inset-0 w-full overflow-auto bg-transparent pr-10 font-mono break-words whitespace-pre-wrap'
					)}
				>
					{@html getMaskedValue(value)}
				</div>
			{/if}
		</div>
	{:else}
		<input
			data-1p-ignore
			id={name}
			{name}
			class={twMerge(
				'text-input-filled w-full pr-10',
				error && 'border-red-500 bg-red-500/20 text-red-500 ring-red-500 focus:ring-1'
			)}
			{value}
			type={showSensitive ? 'text' : 'password'}
			oninput={handleInput}
			autocomplete="new-password"
			{disabled}
		/>
	{/if}

	<div
		class="absolute top-1/2 right-4 z-10 grid -translate-y-1/2 grid-cols-1 grid-rows-1"
		use:tooltip={{ disablePortal: true, text: showSensitive ? 'Hide' : 'Reveal' }}
	>
		<button
			type="button"
			class="cursor-pointer transition-colors duration-150"
			class:text-red-500={error}
			onclick={toggleVisibility}
		>
			{#if showSensitive}
				<EyeOff class="size-4" />
			{:else}
				<Eye class="size-4" />
			{/if}
		</button>
	</div>
</div>

<style>
	.text-input-filled.base.hide {
		color: transparent;
		caret-color: var(--color-on-background);
	}
	.text-input-filled.base.hide::selection {
		background: highlight;
		color: transparent;
	}
</style>
