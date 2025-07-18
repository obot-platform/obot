<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Copy } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		text?: string;
		class?: string;
		tooltipText?: string;
		buttonText?: string;
		disabled?: boolean;
		classes?: {
			button?: string;
		};
		showTextLeft?: boolean;
	}

	let {
		text,
		class: clazz = '',
		tooltipText = 'Copy',
		buttonText,
		disabled,
		classes,
		showTextLeft
	}: Props = $props();
	let message = $state<string>(tooltipText);
	let buttonTextToShow = $state(buttonText);
	const COPIED_TEXT = 'Copied!';

	// Check if clipboard is available
	let clipboardUnavailable = $derived(typeof navigator === 'undefined' || !navigator.clipboard);

	// Combine explicit disabled prop with clipboard availability
	let isDisabled = $derived(disabled || clipboardUnavailable);

	function copy() {
		if (!text) return;
		if (!navigator.clipboard) return;

		navigator.clipboard.writeText(text);
		message = COPIED_TEXT;
		buttonTextToShow = COPIED_TEXT;
		setTimeout(() => {
			message = tooltipText;
		}, 750);
	}
</script>

{#if text}
	<button
		use:tooltip={message}
		onclick={() => copy()}
		disabled={isDisabled}
		onmouseenter={() => (buttonTextToShow = buttonText)}
		class={twMerge(
			buttonText &&
				'button-small flex items-center gap-1 rounded-full border border-blue-500 bg-transparent px-4 py-2 text-blue-500 hover:bg-blue-500 hover:text-white disabled:bg-transparent disabled:text-blue-500 disabled:opacity-50',
			classes?.button
		)}
	>
		{#if showTextLeft}
			{buttonTextToShow}
			<Copy class={twMerge('h-4 w-4', clazz)} />
		{:else}
			<Copy class={twMerge('h-4 w-4', clazz)} />
			{buttonTextToShow}
		{/if}
	</button>
{/if}
