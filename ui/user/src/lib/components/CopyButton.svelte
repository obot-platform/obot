<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Check, Copy } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
		text?: string;
		class?: string;
		tooltipText?: string;
		buttonText?: string;
		disabled?: boolean;
		classes?: {
			button?: string;
		};
		showTextLeft?: boolean;
		noButtonText?: boolean;
	}

	let {
		id,
		text,
		class: clazz = '',
		tooltipText = 'Copy',
		buttonText,
		disabled,
		classes,
		showTextLeft,
		noButtonText
	}: Props = $props();
	let message = $state<string>(untrack(() => tooltipText));
	let buttonTextToShow = $state(untrack(() => buttonText));
	let copied = $state(false);
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

	export async function copy() {
		if (!text) return;

		let success: boolean;

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
			copied = true;
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
		{id}
		use:tooltip={disabled ? undefined : message}
		onclick={() => copy()}
		{disabled}
		onmouseenter={() => {
			buttonTextToShow = buttonText;
			copied = false;
		}}
		class={twMerge(
			buttonText && 'btn btn-soft btn-primary',
			'flex gap-1 text-xs items-center',
			classes?.button
		)}
		type="button"
	>
		{#if noButtonText}
			{#if copied}
				<Check class="size-4" />
			{:else}
				<Copy class="size-4" />
			{/if}
		{:else if showTextLeft}
			{buttonTextToShow}
			<Copy class={twMerge('size-4', clazz)} />
		{:else}
			<Copy class={twMerge('size-4', clazz)} />
			{buttonTextToShow}
		{/if}
	</button>
{/if}
