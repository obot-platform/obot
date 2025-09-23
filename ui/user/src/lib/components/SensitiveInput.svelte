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
	let isEditing = $state(false);

	function getMaskedValue(text: string): string {
		return text.replace(/[^\n\r]/g, '•');
	}

	function handleTextareaFocus() {
		if (!showSensitive) {
			isEditing = true;
		}
	}

	function handleTextareaBlur() {
		if (!showSensitive) {
			isEditing = false;
		}
	}

	function handleTextareaInput(event: Event) {
		const input = event.target as HTMLInputElement | HTMLTextAreaElement;
		value = input.value;
		oninput?.();
	}

	function handleInput(event: Event) {
		const input = event.target as HTMLInputElement;
		value = input.value;
		oninput?.();
	}

	function toggleVisibility(e: MouseEvent) {
		e.preventDefault();
		showSensitive = !showSensitive;
	}
</script>

<div class="relative flex grow items-center">
	{#if textarea}
		<textarea
			bind:this={textareaElement}
			data-1p-ignore
			id={name}
			{name}
			class={twMerge(
				'text-input-filled w-full pr-10',
				error && 'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
			)}
			class:text-red-500={error}
			value={showSensitive || isEditing ? value : getMaskedValue(value || '')}
			onfocus={handleTextareaFocus}
			onblur={handleTextareaBlur}
			oninput={handleTextareaInput}
			{disabled}
		></textarea>
	{:else}
		<input
			data-1p-ignore
			id={name}
			{name}
			class={twMerge(
				'text-input-filled w-full pr-10',
				error && 'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
			)}
			class:text-red-500={error}
			{value}
			type={showSensitive ? 'text' : 'password'}
			oninput={handleInput}
			autocomplete="new-password"
			{disabled}
		/>
	{/if}

	<button
		type="button"
		class="absolute top-1/2 right-4 z-10 -translate-y-1/2 cursor-pointer"
		class:text-red-500={error}
		onclick={toggleVisibility}
		use:tooltip={{ disablePortal: true, text: showSensitive ? 'Hide' : 'Reveal' }}
	>
		{#if showSensitive}
			<EyeOff class="size-4" />
		{:else}
			<Eye class="size-4" />
		{/if}
	</button>
</div>
