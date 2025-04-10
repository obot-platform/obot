<script lang="ts">
	import {
		lineNumbers,
		highlightActiveLineGutter,
		highlightSpecialChars,
		drawSelection,
		dropCursor,
		keymap
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
	import { EditorState, type Transaction } from '@codemirror/state';
	import { EditorView } from '@codemirror/view';
	import { type LanguageSupport } from '@codemirror/language';
	import type { Tooltip, TooltipView } from '@codemirror/view';
	import { showTooltip } from '@codemirror/view';
	import { StateField } from '@codemirror/state';
	import { githubLight, githubDark } from '@uiw/codemirror-theme-github';

	import { MessageSquareText, CircleHelp } from 'lucide-svelte/icons';
	import { darkMode } from '$lib/stores';
	import Input from '$lib/components/messages/Input.svelte';
	import type { InvokeInput } from '$lib/services';
	import { tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	const cursorTooltipField = StateField.define<readonly Tooltip[]>({
		create: createTooltips,
		update: updateExplainToolTip,
		provide: (f) => showTooltip.computeN([f], (state) => state.field(f))
	});

	let explain = $state<HTMLElement>();

	interface Props {
		file: EditorItem;
		onInvoke?: (invoke: InvokeInput) => void | Promise<void>;
		onFileChanged?: (name: string, contents: string) => void;
		class?: string;
		items: EditorItem[];
	}

	let { file, onInvoke, onFileChanged, class: klass = '', items }: Props = $props();
	let lastSetValue = '';
	let focused = $state(false);
	let ttState: EditorState | undefined = $state();
	let ttVisible = $state(false);
	let ttImprove = $state(false);
	let view: EditorView | undefined = $state();
	let setDarkMode: boolean;
	let reloadDarkMode: () => void;
	let input = $state<ReturnType<typeof Input>>();

	const basicSetup = (() => [
		lineNumbers(),
		highlightActiveLineGutter(),
		highlightSpecialChars(),
		history(),
		foldGutter(),
		drawSelection(),
		dropCursor(),
		EditorState.allowMultipleSelections.of(true),
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
		cursorTooltipField
	])();

	$effect(() => {
		if (file?.file?.contents) {
			setValue(file.file.contents);
		}
		if (!focused) {
			// hide()
		}
	});

	$effect(() => {
		if (setDarkMode !== darkMode.isDark) {
			reloadDarkMode();
		}
	});

	function onExplain() {
		if (ttState) {
			const selection = ttState.selection.ranges[0];
			if (selection.from != selection.to) {
				onInvoke?.({
					explain: {
						filename: file.name,
						selection: ttState.sliceDoc(selection.from, selection.to).toString()
					}
				});
			}
		}
		hide();
	}

	function hide() {
		ttVisible = false;
		ttImprove = false;
	}

	async function onSubmit(input: InvokeInput) {
		if (ttState) {
			input.improve = {
				filename: file.name,
				selection: ttState
					.sliceDoc(ttState.selection.ranges[0].from, ttState.selection.ranges[0].to)
					.toString()
			};
			await onInvoke?.(input);
		}
		hide();
	}

	function setValue(newContent: string) {
		if (lastSetValue === newContent || !view) {
			return;
		}

		lastSetValue = newContent;
		view.dispatch(
			view.state.update({
				changes: { from: 0, to: view.state.doc.length, insert: newContent }
			})
		);
	}

	function editor(targetElement: HTMLElement) {
		const ext = file.name.split('.').pop() || 'js';
		const languages = {
			java: java,
			go: go,
			c: cpp,
			h: cpp,
			hpp: cpp,
			cpp: cpp,
			css: css,
			html: html,
			sql: sql,
			vue: vue,
			sass: sass,
			rs: rust
		} as Record<string, () => LanguageSupport>;

		const langExtension = ext === 'txt' ? () => [] : (languages[ext] ?? javascript);
		lastSetValue = file?.file?.contents ?? '';

		const updater = EditorView.updateListener.of((update) => {
			if (update.docChanged && focused) {
				onFileChanged?.(file.name, update.state.doc.toString());
			}
		});

		// initial state is never use because it's immediately replaced by the reloadDarkMode
		// below, so the reloadDarkMode is real constructor
		let state: EditorState = EditorState.create({
			doc: file?.file?.contents
		});

		view = new EditorView({
			parent: targetElement,
			state
		});

		reloadDarkMode = () => {
			const newState = EditorState.create({
				doc: state.doc,
				extensions: [
					basicSetup,
					darkMode.isDark ? githubDark : githubLight,
					updater,
					langExtension(),
					cursorTooltipField
				]
			});
			view?.setState(newState);
			state = newState;
			setDarkMode = darkMode.isDark;
		};
		reloadDarkMode();
	}

	function newExplainToolTip(state: EditorState): TooltipView {
		ttState = state;
		const selectionFrom = state.selection.ranges[0].from;
		const line = state.doc.lineAt(selectionFrom).number;

		return {
			dom: explain ?? document.createElement('div'),
			offset: {
				// even though strictSide is false, the tooltip is still
				// being positioned outside parent container
				// adjusting offset of the tooltip highlighting in first 4 lines
				x: line <= 4 ? 12 : 0,
				y: line <= 4 ? -52 : 12
			},
			mount() {
				ttVisible = true;
				ttImprove = false;
			},
			destroy: hide
		};
	}

	function updateExplainToolTip(tooltips: readonly Tooltip[], tr: Transaction): readonly Tooltip[] {
		if (tooltips.length !== 1) {
			return createTooltips(tr.state);
		}

		const obj = tooltips[0] as object;
		if ('state' in obj && obj.state instanceof EditorState) {
			if (createTooltips(tr.state).length === 0) {
				ttState = undefined;
				hide();
				return [];
			} else {
				obj.state = tr.state;
				ttState = tr.state;
			}
		}

		return tooltips;
	}

	function createTooltips(state: EditorState): readonly Tooltip[] {
		if (!onInvoke) {
			return [];
		}

		if (
			state.selection.ranges.length == 1 &&
			state.selection.ranges[0].from != state.selection.ranges[0].to
		) {
			const tooltip = {
				pos: state.selection.ranges[0].from,
				end: state.selection.ranges[0].to,
				above: true,
				strictSide: false,
				create: () => {
					return newExplainToolTip(state);
				},
				state: state,
				selection: state.selection.ranges[0]
			} as Tooltip;
			return [tooltip];
		}
		return [];
	}
</script>

<div
	use:editor
	onfocusin={() => (focused = true)}
	onfocusout={() => (focused = false)}
	class={twMerge('mx-2 mt-4 h-full border-l-2 border-gray-100 dark:border-gray-900', klass)}
></div>

<div class="absolute flex">
	<div class="bg-gray-70 flex rounded-3xl shadow-lg" bind:this={explain} class:hidden={!ttVisible}>
		<button
			class="flex items-center gap-2 rounded-s-3xl p-4 ps-5 hover:bg-gray-100 dark:bg-gray-950 dark:hover:bg-gray-900"
			onclick={onExplain}
			class:hidden={ttImprove}
		>
			<span class="text-sm">Explain</span>
			<CircleHelp class="h-5 w-5" />
		</button>
		<button
			class="flex items-center gap-2 rounded-e-3xl p-4 pe-5 hover:bg-gray-100 dark:bg-gray-950 dark:hover:bg-gray-900"
			onclick={async () => {
				ttImprove = true;
				await tick();
				input?.focus();
			}}
			class:hidden={ttImprove}
		>
			<span class="text-sm">Improve</span>
			<MessageSquareText class="h-5 w-5" />
		</button>
		<div class:hidden={!ttImprove} class="flex w-full max-w-[700px]">
			<Input {onSubmit} bind:this={input} placeholder="Instructions..." {items} />
		</div>
	</div>
</div>

<style lang="postcss">
	:global {
		.cm-editor {
			height: 100%;
		}
		.ͼ2 .cm-tooltip {
			border: none;
		}
		.ͼ14 {
			background-color: #000000 !important;
		}
		.cm-focused {
			outline-style: none !important;
		}
	}
</style>
