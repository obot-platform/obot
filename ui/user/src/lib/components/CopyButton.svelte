<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Copy } from 'lucide-svelte';
	import { untrack } from 'svelte';
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
	let message = $state<string>(untrack(() => tooltipText));
	let buttonTextToShow = $state(untrack(() => buttonText));
	const COPIED_TEXT = 'Copied!';

	function fallbackCopy(textToCopy: string): boolean {
		const previousActiveElement = document.activeElement;
		const textArea = document.createElement('textarea');
		textArea.value = textToCopy;

		textArea.style.position = 'fixed';
		textArea.style.left = '-9999px';
		textArea.style.top = '0';
		document.body.appendChild(textArea);

		textArea.focus();
		textArea.select();

		try {
			// is deprecated but still works for those without navigator.clipboard context
			return document.execCommand('copy');
		} catch {
			return false;
		} finally {
			document.body.removeChild(textArea);
			(previousActiveElement as HTMLElement)?.focus?.();
		}
	}

	async function copy() {
		if (!text) return;

		let success = false;

		if (navigator.clipboard) {
			try {
				await navigator.clipboard.writeText(text);
				success = true;
			} catch {
				success = fallbackCopy(text);
			}
		} else {
			success = fallbackCopy(text);
		}

		if (success) {
			message = COPIED_TEXT;
			buttonTextToShow = COPIED_TEXT;
			setTimeout(() => {
				message = tooltipText;
			}, 750);
		}
	}

	export function clearButtonText() {
		buttonTextToShow = buttonText;
	}
</script>

{#if text}
	<button
		use:tooltip={message}
		onclick={() => copy()}
		{disabled}
		onmouseenter={() => (buttonTextToShow = buttonText)}
		class={twMerge(
			buttonText &&
				'button-small border-primary text-primary hover:bg-primary disabled:text-primary flex items-center gap-1 rounded-full border bg-transparent px-4 py-2 hover:text-white disabled:bg-transparent disabled:opacity-50',
			classes?.button
		)}
		type="button"
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
