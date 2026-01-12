<script lang="ts">
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import { darkMode } from '$lib/stores';
	import {
		closeBrackets,
		autocompletion,
		closeBracketsKeymap,
		completionKeymap
	} from '@codemirror/autocomplete';
	import { history, defaultKeymap, historyKeymap } from '@codemirror/commands';
	import { markdown } from '@codemirror/lang-markdown';
	import {
		foldGutter,
		indentOnInput,
		syntaxHighlighting,
		defaultHighlightStyle,
		bracketMatching,
		foldKeymap
	} from '@codemirror/language';
	import { lintKeymap } from '@codemirror/lint';
	import { searchKeymap } from '@codemirror/search';
	import { EditorState as CMEditorState } from '@codemirror/state';
	import {
		lineNumbers,
		highlightActiveLineGutter,
		highlightSpecialChars,
		drawSelection,
		dropCursor,
		keymap,
		placeholder as cmPlaceholder,
		EditorView
	} from '@codemirror/view';
	import { EditorView as CMEditorView } from '@codemirror/view';
	import '@milkdown/crepe/theme/common/style.css';
	import '@milkdown/crepe/theme/frame.css';
	import { githubLight, githubDark } from '@uiw/codemirror-theme-github';
	import { onMount, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		value?: string;
		class?: string;
		classes?: {
			input?: string;
		};
		disabled?: boolean;
		placeholder?: string;
		disablePreview?: boolean;
	}

	let {
		value = $bindable(''),
		class: klass,
		classes,
		disabled,
		placeholder,
		disablePreview
	}: Props = $props();

	let lastSetValue = '';
	let focused = $state(false);
	let showPreview = $state(false);

	let cmView: CMEditorView | undefined = $state();
	let setDarkMode: boolean;
	let reload: () => void;

	// CodeMirror basic setup
	const basicSetup = (() => [
		// Enable line wrapping
		EditorView.lineWrapping,
		lineNumbers(),
		highlightActiveLineGutter(),
		highlightSpecialChars(),
		history(),
		foldGutter(),
		drawSelection(),
		dropCursor(),
		CMEditorState.allowMultipleSelections.of(true),
		indentOnInput(),
		syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
		bracketMatching(),
		closeBrackets(),
		autocompletion(),
		keymap.of([
			...closeBracketsKeymap,
			...defaultKeymap,
			...searchKeymap,
			...historyKeymap,
			...foldKeymap,
			...completionKeymap,
			...lintKeymap
		]),
		// Add custom class to scope styles
		CMEditorView.editorAttributes.of({ class: 'markdown-input-editor' })
	])();

	onMount(() => {
		if (value) {
			setValue(value);
		}
	});

	// Effect to handle dark mode changes
	$effect(() => {
		if (setDarkMode !== darkMode.isDark) {
			reload();
		}
	});

	// Track previous disabled state to detect changes
	let prevDisabled = $state(untrack(() => disabled));

	$effect(() => {
		if (cmView && prevDisabled !== disabled) {
			prevDisabled = disabled;
			reload();
		}
	});

	async function setValue(value: string) {
		if (lastSetValue === value) {
			return;
		}

		cmView?.dispatch(
			cmView.state.update({
				changes: { from: 0, to: cmView?.state.doc.length, insert: value }
			})
		);
		lastSetValue = value;
	}

	// CodeMirror editor function
	function cmEditor(targetElement: HTMLElement) {
		lastSetValue = value;

		const updater = CMEditorView.updateListener.of((update) => {
			if (update.docChanged && focused && !disabled) {
				const newValue = update.state.doc.toString();
				if (newValue !== lastSetValue) {
					value = newValue;
					lastSetValue = newValue;
				}
			}
		});

		let state: CMEditorState = CMEditorState.create({
			doc: value
		});

		cmView = new CMEditorView({
			parent: targetElement,
			state
		});

		reload = () => {
			const newState = CMEditorState.create({
				doc: state.doc,
				extensions: [
					basicSetup,
					darkMode.isDark ? githubDark : githubLight,
					updater,
					markdown(),
					// Add placeholder if provided
					...(placeholder ? [cmPlaceholder(placeholder)] : []),
					// Make editor read-only when disabled
					disabled ? CMEditorState.readOnly.of(true) : CMEditorState.readOnly.of(false)
				]
			});
			cmView?.setState(newState);
			state = newState;
			setDarkMode = darkMode.isDark;
		};
		reload();

		return {
			destroy: () => {
				cmView?.destroy();
				cmView = undefined;
			}
		};
	}
</script>

<div
	class={twMerge(
		'text-input-filled border-surface3 dark:bg-background flex flex-col gap-0 overflow-hidden border p-0 transition-colors',
		focused && !disabled && !disablePreview && 'ring-primary ring-2 outline-none',
		disabled && 'disabled opacity-50',
		klass
	)}
>
	{#if !disablePreview}
		<div
			class="dark:border-surface3 dark:bg-surface2 text-on-surface1 flex items-center border-b text-sm font-light"
		>
			<button
				class={twMerge(
					'px-4 py-2',
					!showPreview &&
						'dark:border-surface3 bg-background text-on-background relative z-10 translate-y-[1px] border-r font-medium'
				)}
				onclick={() => {
					showPreview = false;
					// Focus the editor after it becomes visible
					setTimeout(() => {
						if (cmView && !disabled) {
							cmView.focus();
						}
					}, 0);
				}}>Write</button
			>
			<button
				class={twMerge(
					'px-4 py-2',
					showPreview &&
						'dark:border-surface3 bg-background text-on-background relative z-10 translate-y-[1px] border-x font-medium'
				)}
				onclick={() => (showPreview = true)}>Preview</button
			>
		</div>
	{/if}
	{#if !disablePreview && showPreview}
		<div
			class="milkdown-content default-scrollbar-thin bg-background max-h-[650px] min-h-48 overflow-y-auto p-4"
		>
			{@html toHTMLFromMarkdownWithNewTabLinks(value)}
		</div>
	{:else}
		<div
			class={twMerge(
				'default-scrollbar-thin bg-background max-h-[650px] min-h-48 overflow-y-auto p-4 ',
				classes?.input
			)}
			use:cmEditor
			onfocusin={() => (focused = true)}
			onfocusout={() => (focused = false)}
		></div>
	{/if}
</div>

<style lang="postcss">
	:global {
		.cm-editor.markdown-input-editor {
			font-size: var(--text-md);
			background-color: transparent;
			height: 100%;
			.cm-gutters {
				display: none;
			}
		}
		.cm-editor.markdown-input-editor .cm-scroller {
			height: inherit;
			-ms-overflow-style: none; /* IE and Edge */
			scrollbar-width: none; /* Firefox */
			overflow: unset !important;
		}
		.cm-editor.markdown-input-editor .cm-scroller::-webkit-scrollbar {
			display: none;
		}
		.cm-editor.markdown-input-editor.cm-focused {
			outline-style: none !important;
		}

		/* Hide cursor when disabled but keep selection */
		.disabled .cm-editor.markdown-input-editor .cm-cursor {
			display: none !important;
		}
	}
</style>
