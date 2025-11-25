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
		autoHeight?: boolean;
		maxHeight?: string;
	}

	let {
		value = $bindable(''),
		class: klass,
		disabled,
		placeholder,
		rows = 6,
		autoHeight = false,
		maxHeight
	}: Props = $props();

	let lastSetValue = '';
	let focused = $state(false);
	let cmView: CMEditorView | undefined = $state();
	let setDarkMode: boolean;
	let reload: () => void;

	const getBasicSetup = () => [
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
		CMEditorView.lineWrapping,
		// Add custom class to scope styles
		CMEditorView.editorAttributes.of({ class: 'yaml-editor' })
	];

	const getHeightConfig = () => {
		if (autoHeight) {
			return [
				CMEditorView.editorAttributes.of({ class: 'yaml-editor-auto-height' }),
				CMEditorView.theme({
					'&': {
						height: 'auto',
						minHeight: `${rows * 1.5}rem`,
						'--yaml-editor-min-height': `${rows * 1.5}rem`
					},
					'.cm-scroller': {
						overflow: 'auto',
						minHeight: `${rows * 1.5}rem`,
						maxHeight: maxHeight || 'none'
					}
				})
			];
		}
		return [];
	};

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
					getBasicSetup(),
					darkMode.isDark ? githubDark : githubLight,
					updater,
					yaml(),
					...getHeightConfig(),
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
		'text-input-filled border-surface3 dark:bg-background overflow-hidden p-0 transition-colors',
		focused && !disabled && 'ring-primary ring-2 outline-none',
		disabled && 'disabled opacity-60',
		klass
	)}
	style={autoHeight
		? `min-height: ${rows * 1.5}rem;${maxHeight ? ` max-height: ${maxHeight};` : ''}`
		: `height: ${rows * 1.5}rem; min-height: ${rows * 1.5}rem;`}
	role="textbox"
	tabindex="-1"
	onclick={() => {
		if (cmView && !disabled && !cmView.hasFocus) {
			cmView.focus();
		}
	}}
	onkeydown={(e) => {
		if (e.key === 'Enter' && cmView && !disabled && !cmView.hasFocus) {
			e.preventDefault();
			cmView.focus();
		}
	}}
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
		.cm-editor.yaml-editor:not(.yaml-editor-auto-height) {
			height: 100% !important;
		}
		.cm-editor.yaml-editor .cm-scroller {
			overflow: auto;
		}
		.cm-editor.yaml-editor.cm-focused {
			outline-style: none !important;
		}
		.cm-editor.yaml-editor-auto-height .cm-gutters {
			min-height: var(--yaml-editor-min-height) !important;
		}
	}
</style>
