<script lang="ts">
	import '@milkdown/crepe/theme/common/style.css';
	import '@milkdown/crepe/theme/frame.css';
	import { twMerge } from 'tailwind-merge';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';

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
	import { javascript } from '@codemirror/lang-javascript';
	import { go } from '@codemirror/lang-go';
	import { cpp } from '@codemirror/lang-cpp';
	import { css } from '@codemirror/lang-css';
	import { html } from '@codemirror/lang-html';
	import { sql } from '@codemirror/lang-sql';
	import { vue } from '@codemirror/lang-vue';
	import { sass } from '@codemirror/lang-sass';
	import { rust } from '@codemirror/lang-rust';
	import { java } from '@codemirror/lang-java';
	import { yaml } from '@codemirror/lang-yaml';
	import type { LanguageSupport } from '@codemirror/language';
	import { EditorState as CMEditorState } from '@codemirror/state';
	import { EditorView as CMEditorView } from '@codemirror/view';
	import { githubLight, githubDark } from '@uiw/codemirror-theme-github';
	import { darkMode } from '$lib/stores';
	import { onDestroy, untrack } from 'svelte';

	const LANGUAGE_MAP: Record<string, () => LanguageSupport> = {
		java,
		go,
		c: cpp,
		h: cpp,
		hpp: cpp,
		cpp,
		css,
		html,
		htm: html,
		sql,
		vue,
		sass,
		scss: sass,
		rs: rust,
		rust,
		yml: yaml,
		yaml
	};

	function getLanguageSupport(filename: string | undefined): LanguageSupport[] {
		if (!filename) return [markdown()];
		const ext = filename.split('.').pop()?.toLowerCase() ?? '';
		if (ext === 'txt') return [];
		const lang = LANGUAGE_MAP[ext] ?? javascript;
		return [lang()];
	}

	interface Props {
		value?: string;
		filename?: string;
		class?: string;
		classes?: {
			input?: string;
		};
		disabled?: boolean;
		placeholder?: string;
		disablePreview?: boolean;
		typewriterOnAutonomous?: boolean;
		typewriterSpeed?: number;
		overrideContent?: string;
	}

	let {
		value = $bindable(''),
		filename,
		class: klass,
		classes,
		disabled,
		placeholder,
		disablePreview,
		typewriterOnAutonomous = false,
		typewriterSpeed = 0,
		overrideContent
	}: Props = $props();

	let lastSetValue = '';
	let focused = $state(false);
	let showPreview = $state(false);

	let currentAnimation: AbortController | null = null;

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
		CMEditorView.editorAttributes.of({ class: 'raw-editor' })
	])();

	$effect(() => {
		if (overrideContent) {
			animateTemporaryValue(overrideContent);
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
	let prevFilename = $state(untrack(() => filename));

	$effect(() => {
		if (cmView && prevDisabled !== disabled) {
			prevDisabled = disabled;
			reload();
		}
	});

	$effect(() => {
		if (cmView && prevFilename !== filename) {
			prevFilename = filename;
			reload();
		}
	});

	onDestroy(() => {
		if (currentAnimation) {
			currentAnimation.abort();
			currentAnimation = null;
		}
	});

	async function animateTemporaryValue(changedValue: string) {
		if (currentAnimation) {
			currentAnimation.abort();
			currentAnimation = null;
		}

		if (typewriterOnAutonomous && cmView) {
			const currentDoc = cmView.state.doc.toString();
			const newLength = changedValue.length;

			// Create new abort controller for this animation
			currentAnimation = new AbortController();
			const signal = currentAnimation.signal;

			// Find the common prefix length to determine where to start animating
			let commonPrefixLength = 0;
			const minLength = Math.min(currentDoc.length, newLength);
			while (
				commonPrefixLength < minLength &&
				currentDoc[commonPrefixLength] === changedValue[commonPrefixLength]
			) {
				commonPrefixLength++;
			}

			// Start animation from where the content differs
			const startIndex = commonPrefixLength;

			// If there's new content to animate, do the typewriter effect
			if (startIndex < newLength) {
				// Build up the content incrementally
				let currentContent = currentDoc.substring(0, startIndex);

				for (let i = startIndex; i < newLength; i++) {
					// Check if animation was cancelled
					if (signal.aborted) {
						return;
					}
					await new Promise((resolve) => setTimeout(resolve, typewriterSpeed));

					// Check again after the timeout
					if (signal.aborted) {
						return;
					}

					// Add the next character
					currentContent += changedValue[i];

					// Update the entire document with the new content
					if (cmView) {
						cmView.dispatch(
							cmView.state.update({
								changes: { from: 0, to: cmView.state.doc.length, insert: currentContent }
							})
						);
					}
				}
			}
		}
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
			const langSupport = getLanguageSupport(filename);
			const newState = CMEditorState.create({
				doc: state.doc,
				extensions: [
					basicSetup,
					darkMode.isDark ? githubDark : githubLight,
					updater,
					...langSupport,
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
		disabled && 'disabled',
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
		.cm-editor.raw-editor {
			font-size: var(--text-md);
			background-color: transparent;
			height: 100%;
			.cm-gutters {
				display: none;
			}
		}
		.cm-editor.raw-editor .cm-scroller {
			height: inherit;
			-ms-overflow-style: none; /* IE and Edge */
			scrollbar-width: none; /* Firefox */
			overflow: unset !important;
		}
		.cm-editor.raw-editor .cm-scroller::-webkit-scrollbar {
			display: none;
		}
		.cm-editor.raw-editor.cm-focused {
			outline-style: none !important;
		}

		/* Hide cursor when disabled but keep selection */
		.disabled .cm-editor.raw-editor .cm-cursor {
			display: none !important;
		}

		/* Gray styling for existing characters during animation */
		.text-gray-400 {
			color: #9ca3af !important;
		}
		.opacity-60 {
			opacity: 0.6 !important;
		}
	}
</style>
