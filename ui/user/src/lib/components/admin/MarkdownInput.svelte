<script lang="ts">
	import '@milkdown/crepe/theme/common/style.css';
	import '@milkdown/crepe/theme/frame.css';
	import { twMerge } from 'tailwind-merge';
	import { onMount } from 'svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';

	import {
		lineNumbers,
		highlightActiveLineGutter,
		highlightSpecialChars,
		drawSelection,
		dropCursor,
		keymap,
		placeholder as cmPlaceholder
	} from '@codemirror/view';
	import {
		foldGutter,
		indentOnInput,
		syntaxHighlighting,
		defaultHighlightStyle,
		bracketMatching,
		foldKeymap
	} from '@codemirror/language';
	import { history, defaultKeymap, historyKeymap } from '@codemirror/commands';
	import { searchKeymap } from '@codemirror/search';
	import {
		closeBrackets,
		autocompletion,
		closeBracketsKeymap,
		completionKeymap
	} from '@codemirror/autocomplete';
	import { lintKeymap } from '@codemirror/lint';
	import { markdown } from '@codemirror/lang-markdown';
	import { EditorState as CMEditorState } from '@codemirror/state';
	import { EditorView as CMEditorView } from '@codemirror/view';
	import { githubLight, githubDark } from '@uiw/codemirror-theme-github';
	import { darkMode } from '$lib/stores';

	interface Props {
		value?: string;
		class?: string;
		disabled?: boolean;
		placeholder?: string;
	}

	let { value = $bindable(''), class: klass, disabled, placeholder }: Props = $props();

	let lastSetValue = '';
	let focused = $state(false);
	let showPreview = $state(false);

	let cmView: CMEditorView | undefined = $state();
	let setDarkMode: boolean;
	let reload: () => void;

	// CodeMirror basic setup
	const basicSetup = (() => [
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
		])
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
	let prevDisabled = $state(disabled);

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
		'text-input-filled border-surface3 flex flex-col gap-0 overflow-hidden border p-0 transition-colors dark:bg-black',
		focused && !disabled && 'ring-2 ring-blue-500 outline-none',
		disabled && 'disabled opacity-50',
		klass
	)}
>
	<div
		class="dark:border-surface3 dark:bg-surface2 flex items-center border-b text-sm font-light text-gray-500"
	>
		<button
			class={twMerge(
				'px-4 py-2',
				!showPreview &&
					'dark:border-surface3 relative z-10 translate-y-[1px] border-r bg-white font-medium text-black dark:bg-black dark:text-white'
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
					'dark:border-surface3 relative z-10 translate-y-[1px] border-x bg-white font-medium text-black dark:bg-black dark:text-white'
			)}
			onclick={() => (showPreview = true)}>Preview</button
		>
	</div>
	{#if showPreview}
		<div
			class="milkdown-content default-scrollbar-thin h-48 overflow-y-auto bg-white p-4 dark:bg-black"
		>
			{@html toHTMLFromMarkdown(value)}
		</div>
	{:else}
		<div
			class="default-scrollbar-thin h-48 max-h-49 overflow-y-auto bg-white p-4 dark:bg-black"
			use:cmEditor
			onfocusin={() => (focused = true)}
			onfocusout={() => (focused = false)}
		></div>
	{/if}
</div>

<style lang="postcss">
	:global {
		.cm-editor {
			font-size: var(--text-md);
			background-color: transparent;
			.cm-gutters {
				display: none;
			}
		}
		.cm-focused {
			outline-style: none !important;
		}

		/* Hide cursor when disabled but keep selection */
		.disabled .cm-editor .cm-cursor {
			display: none !important;
		}
		.milkdown-content {
			& h1,
			& h2,
			& h3,
			& h4,
			& p {
				&:first-child {
					margin-top: 0;
				}
				&:last-child {
					margin-bottom: 0;
				}
			}

			& h1 {
				margin-top: 1rem;
				margin-bottom: 1rem; /* my-4 */
				font-size: 1.5rem; /* text-2xl */
				font-weight: 700; /* font-bold */
			}

			& h2 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1.25rem; /* text-xl */
				font-weight: 700;
			}

			& h3,
			& h4 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1rem; /* text-base */
				font-weight: 700;
			}

			& p {
				margin-bottom: 1rem;
				font-size: var(--text-md);
			}

			& pre {
				padding: 0.5rem 1rem;
			}

			& a {
				color: var(--color-blue-500);
				text-decoration: underline;
				&:hover {
					color: var(--color-blue-600);
				}
			}

			& ol {
				margin: 1rem 0;
				list-style-type: decimal;
				padding-left: 1rem;
			}

			& ul {
				margin: 1rem 0;
				list-style-type: disc;
				padding-left: 1rem;
			}
		}
	}
</style>
