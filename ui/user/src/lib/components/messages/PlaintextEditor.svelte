<script lang="ts">
	import { type Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	import { Editor, rootCtx, editorViewCtx } from '@milkdown/kit/core';
	import { Plugin } from '@milkdown/prose/state';
	import type { EditorView } from '@milkdown/prose/view';

	import { plaintext } from './preset';

	import '@milkdown/kit/prose/view/style/prosemirror.css';
	import { $prose as mlkprose } from '@milkdown/utils';

	type Props = {
		class?: string;
		value?: string;
		placeholder?: string | Snippet;
		onfocus?: () => void;
		onkeydown?: (event: KeyboardEvent) => void;
	};

	let editor: Editor | undefined = $state();

	let {
		value = $bindable(),
		placeholder,
		class: klass = '',
		onkeydown,
		onfocus,
		...restProps
	}: Props = $props();

	// Editor value updated internally; used to detect changes from the outside
	let _value = $state();

	$effect(() => {
		if (!editor) return;

		// Check if we have a change from the outside
		if (_value !== value) {
			if (value) {
				// If value is defined; updated editor content
				setTextContent(value);
			} else {
				// Otherwise clear the editor
				clear();
			}
		}
	});

	// Public method to focus the editor
	export function focus() {
		if (editor) {
			editor.action((ctx) => {
				const view = ctx.get(rootCtx) as HTMLElement;
				if (view) {
					view.focus();
				}
			});
		}
	}

	// Public method to set the editor content programmatically
	function setTextContent(value: string | undefined) {
		if (!editor) return;

		editor.action((ctx) => {
			// const schema = (ctx.get(editorStateCtx) as EditorState).schema;
			const view = ctx.get(editorViewCtx) as EditorView;

			if (!view || !view.state) return;

			// Only update if value is different
			const currentText = view.state.doc.textContent.trim();
			if (currentText === (value?.trim() ?? '')) return;

			const { schema, tr } = view.state;
			const paragraph = schema.nodes.paragraph.createAndFill(
				null,
				value ? schema.text(value) : null
			);

			if (paragraph) {
				const newDoc = schema.topNodeType.createAndFill(null, paragraph);
				tr.replaceWith(0, view.state.doc.content.size, newDoc!);
				view.dispatch(tr);
			}
		});
	}

	export function clear() {
		if (!editor) return;

		editor.action((ctx) => {
			const view = ctx.get(editorViewCtx) as EditorView;
			if (!view) return;

			const { schema, tr, doc } = view.state;
			const paragraph = schema.nodes.paragraph.createAndFill();

			if (paragraph) {
				// Replace the entire document with a single empty paragraph
				tr.replaceWith(0, doc.content.size, paragraph);
				view.dispatch(tr);
			}
		});
	}

	const mlkEventPluging = mlkprose(() => {
		return new Plugin({
			props: {
				handleKeyDown(_, event) {
					if (event.key === 'Enter' && !event.shiftKey) {
						event.preventDefault();
						return true;
					}
					return false;
				}
			}
		});
	});

	const mlkTextChangePlugin = mlkprose(() => {
		return new Plugin({
			appendTransaction(transactions, oldState, newState) {
				if (transactions.some((tr) => tr.docChanged)) {
					_value = value = newState.doc.textBetween(1, newState.doc.content.size - 1, '\n', '\0');
				}

				return null;
			}
		});
	});
</script>

<div
	class={twMerge(
		'plaintext-editor text-md relative w-full flex-1 grow resize-none p-2 leading-8 outline-none',
		klass
	)}
>
	<div
		{@attach (node) => {
			Editor.make()
				.config((ctx) => {
					ctx.set(rootCtx, node!);
				})
				.use(mlkEventPluging)
				.use(mlkTextChangePlugin)
				.use(plaintext)
				.create()
				.then((instance) => {
					editor = instance;
				})
				.catch((error) => {
					console.error('Failed to create editor:', error);
				});
		}}
		role="textbox"
		tabindex="0"
		id="chat"
		class="w-full flex-1 grow resize-none outline-0"
		{onkeydown}
		{onfocus}
		{...restProps}
	></div>

	{#if !value && placeholder}
		<div
			class="placeholder pointer-events-none absolute inset-0 z-0 p-2 opacity-50"
			in:fade={{ duration: 100 }}
			out:fade={{ duration: 1000 / 60 }}
		>
			{#if typeof placeholder === 'string'}
				<div>{placeholder}</div>
			{:else}
				{@render placeholder()}
			{/if}
		</div>
	{/if}
</div>

<style>
	:global(.ProseMirror.editor) {
		outline: none;
	}
</style>
