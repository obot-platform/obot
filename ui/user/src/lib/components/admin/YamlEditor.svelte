<script lang="ts">
	import {
		lineNumbers,
		highlightActiveLineGutter,
		highlightSpecialChars,
		drawSelection,
		dropCursor,
		keymap,
		placeholder as cmPlaceholder,
		EditorView as CMEditorView
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
	import { yaml } from '@codemirror/lang-yaml';
	import { EditorState as CMEditorState } from '@codemirror/state';
	import { githubLight, githubDark } from '@uiw/codemirror-theme-github';
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		value?: string;
		class?: string;
		disabled?: boolean;
		placeholder?: string;
		rows?: number;
	}

	let { value = $bindable(''), class: klass, disabled, placeholder, rows = 6 }: Props = $props();

	let lastSetValue = '';
	let focused = $state(false);
	let cmView: CMEditorView | undefined = $state();
	let setDarkMode: boolean;
	let reload: () => void;

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
		]),
		CMEditorView.lineWrapping
	])();

	$effect(() => {
		if (setDarkMode !== darkMode.isDark) {
			reload?.();
		}
	});

	$effect(() => {
		if (cmView && value !== lastSetValue) {
			lastSetValue = value;
			cmView.dispatch(
				cmView.state.update({
					changes: { from: 0, to: cmView.state.doc.length, insert: value }
				})
			);
		}
	});

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
					yaml(),
					...(placeholder ? [cmPlaceholder(placeholder)] : []),
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
		'text-input-filled border-surface3 overflow-hidden p-0 transition-colors dark:bg-black',
		focused && !disabled && 'ring-2 ring-blue-500 outline-none',
		disabled && 'disabled opacity-60',
		klass
	)}
	style="height: {rows * 1.5}rem; min-height: {rows * 1.5}rem;"
>
	<div
		use:cmEditor
		onfocusin={() => (focused = true)}
		onfocusout={() => (focused = false)}
		class="h-full w-full"
	></div>
</div>

<style lang="postcss">
	:global {
		.cm-editor {
			height: 100% !important;
		}
		.cm-scroller {
			overflow: auto;
		}
		.cm-focused {
			outline-style: none !important;
		}
	}
</style>
