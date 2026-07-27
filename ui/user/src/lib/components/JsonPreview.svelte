<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { json } from '@codemirror/lang-json';
	import {
		defaultHighlightStyle,
		foldGutter,
		foldKeymap,
		syntaxHighlighting
	} from '@codemirror/language';
	import { Compartment, EditorState } from '@codemirror/state';
	import { EditorView, highlightSpecialChars, keymap, lineNumbers } from '@codemirror/view';
	import { Maximize2, Minimize2 } from '@lucide/svelte';
	import { githubDark, githubLight } from '@uiw/codemirror-theme-github';
	import { onDestroy, tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		value: unknown;
		class?: string;
		maxHeight?: string;
		ariaLabel?: string;
		maximizable?: boolean;
	}

	let {
		value,
		class: klass,
		maxHeight = '24rem',
		ariaLabel = 'JSON preview',
		maximizable = false
	}: Props = $props();

	let view: EditorView | undefined;
	let anchor: HTMLDivElement;
	let preview: HTMLDivElement;
	let fullscreenButton = $state<HTMLButtonElement>();
	let isMaximized = $state(false);
	let isTransitioning = $state(false);
	let placeholderHeight = $state<number>();
	let activeAnimation: Animation | undefined;
	let previousBodyOverflow = '';
	const theme = new Compartment();
	const language = new Compartment();
	const previewContent = $derived(formatContent(value));

	function formatContent(input: unknown): { text: string; isJSON: boolean } {
		if (typeof input === 'string') {
			try {
				const parsed = JSON.parse(input);
				return { text: JSON.stringify(parsed, null, 2) ?? input, isJSON: true };
			} catch {
				return { text: input, isJSON: false };
			}
		}

		try {
			const text = JSON.stringify(input, null, 2);
			return text === undefined ? { text: String(input), isJSON: false } : { text, isJSON: true };
		} catch {
			return { text: String(input), isJSON: false };
		}
	}

	function createEditor(node: HTMLElement) {
		view = new EditorView({
			parent: node,
			doc: previewContent.text,
			extensions: [
				lineNumbers(),
				foldGutter(),
				highlightSpecialChars(),
				syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
				language.of(previewContent.isJSON ? json() : []),
				keymap.of(foldKeymap),
				theme.of(darkMode.isDark ? githubDark : githubLight),
				EditorState.readOnly.of(true),
				EditorView.editable.of(false),
				EditorView.lineWrapping,
				EditorView.editorAttributes.of({ class: 'json-preview', 'aria-label': ariaLabel })
			]
		});

		return {
			destroy() {
				view?.destroy();
				view = undefined;
			}
		};
	}

	function animationDuration() {
		return window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 280;
	}

	async function animatePortal(from: DOMRect, to: DOMRect, direction: 'maximize' | 'minimize') {
		const translateX = from.left - to.left;
		const translateY = from.top - to.top;
		const scaleX = from.width / Math.max(to.width, 1);
		const scaleY = from.height / Math.max(to.height, 1);
		const collapsedTransform = `translate(${translateX}px, ${translateY}px) scale(${scaleX}, ${scaleY})`;
		const keyframes =
			direction === 'maximize'
				? [
						{ transform: collapsedTransform, borderRadius: '0.375rem' },
						{ transform: 'none', borderRadius: '0' }
					]
				: [
						{ transform: 'none', borderRadius: '0' },
						{ transform: collapsedTransform, borderRadius: '0.375rem' }
					];

		activeAnimation?.cancel();
		activeAnimation = preview.animate(keyframes, {
			duration: animationDuration(),
			easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
			fill: 'forwards'
		});
		await activeAnimation.finished.catch(() => undefined);
	}

	async function maximize() {
		if (!maximizable || isMaximized || isTransitioning) return;
		isTransitioning = true;

		const origin = preview.getBoundingClientRect();
		placeholderHeight = origin.height;
		previousBodyOverflow = document.body.style.overflow;
		document.body.style.overflow = 'hidden';

		isMaximized = true;
		preview.showPopover();
		await tick();
		await animatePortal(origin, preview.getBoundingClientRect(), 'maximize');
		activeAnimation?.cancel();
		activeAnimation = undefined;
		view?.requestMeasure();
		isTransitioning = false;
		fullscreenButton?.focus();
	}

	async function minimize() {
		if (!isMaximized || isTransitioning) return;
		isTransitioning = true;

		const fullscreenRect = preview.getBoundingClientRect();
		const origin = anchor.getBoundingClientRect();
		await animatePortal(origin, fullscreenRect, 'minimize');

		preview.hidePopover();
		isMaximized = false;
		document.body.style.overflow = previousBodyOverflow;
		placeholderHeight = undefined;
		activeAnimation?.cancel();
		activeAnimation = undefined;
		await tick();
		view?.requestMeasure();
		isTransitioning = false;
		fullscreenButton?.focus();
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape' || !isMaximized) return;
		event.preventDefault();
		event.stopPropagation();
		void minimize();
	}

	$effect(() => {
		const nextContent = previewContent;
		if (!view) return;

		view.dispatch({
			...(view.state.doc.toString() === nextContent.text
				? {}
				: { changes: { from: 0, to: view.state.doc.length, insert: nextContent.text } }),
			effects: language.reconfigure(nextContent.isJSON ? json() : [])
		});
	});

	$effect(() => {
		const nextTheme = darkMode.isDark ? githubDark : githubLight;
		view?.dispatch({ effects: theme.reconfigure(nextTheme) });
	});

	onDestroy(() => {
		activeAnimation?.cancel();
		if (isMaximized) {
			document.body.style.overflow = previousBodyOverflow;
		}
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<div
	bind:this={anchor}
	class="json-preview-anchor min-w-0"
	style:height={placeholderHeight === undefined ? undefined : `${placeholderHeight}px`}
>
	<div
		bind:this={preview}
		popover="manual"
		class={twMerge(
			'json-preview-root border-base-400 bg-base-100 dark:bg-base-300 relative overflow-hidden rounded-md border',
			isMaximized
				? 'json-preview-fullscreen fixed inset-0 z-9999 h-dvh max-h-none w-dvw max-w-none rounded-none border-0'
				: 'w-full',
			klass
		)}
		style:max-height={isMaximized ? 'none' : maxHeight}
	>
		<div use:createEditor></div>
		{#if maximizable}
			<button
				bind:this={fullscreenButton}
				type="button"
				class="btn btn-ghost btn-square btn-sm absolute top-2 right-2"
				onclick={() => (isMaximized ? void minimize() : void maximize())}
				disabled={isTransitioning}
				aria-label={isMaximized ? 'Minimize JSON preview' : 'Maximize JSON preview'}
				aria-pressed={isMaximized}
				title={isMaximized ? 'Minimize JSON preview' : 'Maximize JSON preview'}
			>
				{#if isMaximized}
					<Minimize2 class="size-4" />
				{:else}
					<Maximize2 class="size-4" />
				{/if}
			</button>
		{/if}
	</div>
</div>

<style lang="postcss">
	.json-preview-root[popover]:not(:popover-open) {
		display: block;
	}

	.json-preview-root[popover] {
		margin: 0;
		padding: 0;
	}

	.json-preview-root > div,
	.json-preview-root :global(.cm-editor.json-preview),
	.json-preview-root :global(.cm-editor.json-preview .cm-scroller) {
		max-height: inherit;
	}

	.json-preview-root :global(.cm-editor.json-preview) {
		background-color: transparent;
		font-size: var(--text-xs);
	}

	.json-preview-root :global(.cm-editor.json-preview .cm-scroller) {
		overflow: auto;
		font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
	}

	.json-preview-root :global(.cm-editor.json-preview .cm-gutters) {
		border-right-color: color-mix(in oklab, currentColor 12%, transparent);
		background-color: color-mix(in oklab, currentColor 4%, transparent);
	}

	.json-preview-root :global(.cm-editor.json-preview .cm-content) {
		padding-block: 0.75rem;
	}

	.json-preview-root :global(.cm-editor.json-preview .cm-line) {
		padding-inline: 0.75rem 3rem;
	}

	.json-preview-root :global(.cm-editor.json-preview.cm-focused) {
		outline: none;
	}

	.json-preview-root.json-preview-fullscreen > div,
	.json-preview-root.json-preview-fullscreen :global(.cm-editor.json-preview),
	.json-preview-root.json-preview-fullscreen :global(.cm-editor.json-preview .cm-scroller) {
		height: 100%;
		max-height: none;
	}
</style>
