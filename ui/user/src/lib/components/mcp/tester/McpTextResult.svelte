<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { ChevronDown, ChevronUp, Maximize2, Minimize2 } from '@lucide/svelte';
	import { onDestroy, tick } from 'svelte';

	let { text }: { text: string } = $props();
	const id = $props.id();
	let expanded = $state(false);
	let formatJSON = $state(true);
	let fullscreen = $state(false);
	let dialog: HTMLDialogElement;
	let anchor: HTMLDivElement;
	let pre: HTMLPreElement;
	let fullscreenButton: HTMLButtonElement;
	let placeholderHeight = $state<number>();
	let inlineScrollTop = 0;
	let previousBodyOverflow = '';

	const formattedJSON = $derived.by(() => {
		// Keep initial rendering bounded. Large responses remain available as
		// original text through expansion, fullscreen, and copy.
		if (text.length > 100_000) return undefined;
		try {
			JSON.parse(text);
		} catch {
			return undefined;
		}
		// Change whitespace only: parsing and reserializing would round large numeric
		// IDs, discard duplicate keys, and alter escapes in a diagnostic response.
		const tokens = text.match(/"(?:\\.|[^"\\])*"|[^\s{}[\],:]+|[{}[\],:]/g) ?? [];
		let depth = 0;
		const output: string[] = [];
		let outputLength = 0;
		const newline = () => `\n${'  '.repeat(depth)}`;
		for (const [index, token] of tokens.entries()) {
			const start = output.length;
			if (token === '{' || token === '[') {
				depth++;
				output.push(token);
				if (tokens[index + 1] !== '}' && tokens[index + 1] !== ']') output.push(newline());
			} else if (token === '}' || token === ']') {
				depth--;
				if (tokens[index - 1] !== '{' && tokens[index - 1] !== '[') output.push(newline());
				output.push(token);
			} else if (token === ',') {
				output.push(token, newline());
			} else {
				output.push(token === ':' ? ': ' : token);
			}
			for (let i = start; i < output.length; i++) outputLength += output[i].length;
			// Deep nesting can produce far more indentation than input text.
			if (outputLength > 1_000_000) return undefined;
		}
		return output.join('');
	});
	const displayedText = $derived(formatJSON && formattedJSON !== undefined ? formattedJSON : text);
	// Keep the disclosure available when formatting changes, so the user's expansion
	// choice and the location of the controls remain predictable.
	const isLong = $derived(text.length > 2000 || (formattedJSON ?? text).split(/\r?\n/).length > 20);
	const preview = $derived(displayedText.slice(0, 600).split(/\r?\n/, 8).join('\n'));
	const showingPreview = $derived(isLong && !expanded && !fullscreen);
	const size = $derived(new TextEncoder().encode(text).length);

	async function toggleExpanded() {
		expanded = !expanded;
		await tick();
		if (pre) pre.scrollTop = 0;
	}

	async function openFullscreen() {
		previousBodyOverflow = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		inlineScrollTop = pre.scrollTop;
		placeholderHeight = anchor.getBoundingClientRect().height;
		fullscreen = true;
		await tick();
		if (!dialog?.isConnected) return;
		dialog.showModal();
		pre.scrollTop = inlineScrollTop;
		fullscreenButton.focus();
	}

	async function closeFullscreen() {
		document.body.style.overflow = previousBodyOverflow;
		fullscreen = false;
		placeholderHeight = undefined;
		await tick();
		if (pre) pre.scrollTop = inlineScrollTop;
		fullscreenButton?.focus({ preventScroll: true });
	}

	onDestroy(() => {
		if (fullscreen) document.body.style.overflow = previousBodyOverflow;
	});
</script>

{#snippet viewer()}
	<section
		aria-label="Text result"
		class="border-base-300 dark:border-base-400 bg-base-100 dark:bg-base-300 flex min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border"
		class:h-full={fullscreen}
	>
		<div
			class="border-base-300 dark:border-base-400 flex shrink-0 flex-wrap items-center gap-2 border-b p-3"
		>
			<span class="text-sm font-medium">Text result</span>
			<span class="text-xs text-muted-content"
				>{size < 1024 ? `${size} B` : `${(size / 1024).toFixed(1)} KB`}</span
			>
			<div class="ml-auto flex flex-wrap items-center gap-1">
				{#if formattedJSON !== undefined}
					<button
						type="button"
						class="btn btn-ghost btn-sm"
						aria-pressed={formatJSON}
						class:btn-active={formatJSON}
						onclick={() => (formatJSON = !formatJSON)}>Format JSON</button
					>
				{/if}
				<CopyButton
					{text}
					buttonText="Copy full text"
					tooltipText="Copy full text"
					classes={{ button: 'btn btn-ghost btn-sm' }}
				/>
				<button
					bind:this={fullscreenButton}
					type="button"
					class="btn btn-ghost btn-sm"
					aria-haspopup={fullscreen ? undefined : 'dialog'}
					onclick={() => (fullscreen ? dialog.close() : void openFullscreen())}
				>
					{#if fullscreen}<Minimize2 class="size-4" aria-hidden="true" />Close fullscreen
					{:else}<Maximize2 class="size-4" aria-hidden="true" />Fullscreen{/if}
				</button>
			</div>
		</div>
		<!-- The scrollable text must be focusable for keyboard scrolling. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<pre
			bind:this={pre}
			{id}
			tabindex={showingPreview ? undefined : 0}
			aria-label={showingPreview ? 'Text preview' : 'Full text'}
			class="m-0 min-h-0 p-3 text-sm whitespace-pre-wrap wrap-break-word {fullscreen
				? 'flex-1 overflow-auto overscroll-contain'
				: showingPreview
					? 'max-h-48 overflow-hidden'
					: 'max-h-96 overflow-auto overscroll-contain'}">{showingPreview
				? preview
				: displayedText}{#if showingPreview}<span aria-hidden="true">…</span>{/if}</pre>
		{#if isLong}
			<div
				class="border-base-300 dark:border-base-400 flex shrink-0 flex-wrap items-center justify-between gap-2 border-t p-3"
			>
				<span class="text-xs text-muted-content" role="status"
					>{showingPreview ? 'Preview · More content below' : 'Full text'}</span
				>
				{#if !fullscreen}
					<button
						type="button"
						class="btn btn-ghost btn-sm text-primary"
						aria-expanded={expanded}
						aria-controls={id}
						onclick={toggleExpanded}
					>
						{#if expanded}<ChevronUp class="size-4" aria-hidden="true" />Show less
						{:else}<ChevronDown class="size-4" aria-hidden="true" />Show full text{/if}
					</button>
				{/if}
			</div>
		{/if}
	</section>
{/snippet}

<div
	bind:this={anchor}
	class="min-w-0"
	style:height={placeholderHeight === undefined ? undefined : `${placeholderHeight}px`}
>
	{#if !fullscreen}{@render viewer()}{/if}
</div>
<dialog
	bind:this={dialog}
	aria-label="Fullscreen response"
	class="bg-base-100 dark:bg-base-300 text-base-content fixed inset-0 m-0 h-dvh max-h-none w-dvw max-w-none overflow-hidden border-0 p-0"
	onclose={closeFullscreen}
>
	{#if fullscreen}{@render viewer()}{/if}
</dialog>
