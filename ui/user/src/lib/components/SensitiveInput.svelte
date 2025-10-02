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
	}

	let { name, value = $bindable(''), error, oninput, textarea, disabled }: Props = $props();
	let showSensitive = $state(false);
	let textareaElement = $state<HTMLTextAreaElement>();

	function getMaskedValue(text: string): string {
		return text.replace(/[^\n\r]/g, '•');
	}

	function handleInput(event: Event) {
		const input = event.target as HTMLInputElement;
		value = input.value;
		oninput?.();
	}

	function toggleVisibility(e: MouseEvent) {
		e.preventDefault();
		showSensitive = !showSensitive;

		if (showSensitive) {
			textareaElement?.focus();
		}
	}
</script>

<div class="relative flex grow items-center">
	{#if textarea}
		<textarea
			bind:this={textareaElement}
			data-1p-ignore
			id={name}
			{name}
			{disabled}
			class={twMerge(
				'text-input-filled base w-full pr-10 font-mono',
				error && 'border-red-500 bg-red-500/20 text-red-500 ring-red-500 focus:ring-1',
				!showSensitive && 'hide'
			)}
			bind:value={
				() => value,
				(v) => {
					value = v;
					oninput?.();
				}
			}
		></textarea>

		{#if !showSensitive}
			<!-- Invisible textarea to allow copying the real value -->
			<textarea
				class={twMerge(
					'text-input-filled layer-1 pointer-events-none absolute inset-0 w-full bg-transparent pr-10 font-mono'
				)}
				value={getMaskedValue(value || '')}
			></textarea>
		{/if}
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
