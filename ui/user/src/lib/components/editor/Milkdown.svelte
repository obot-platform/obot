<script lang="ts">
	import { Crepe } from '@milkdown/crepe';

	import { commandsCtx } from '@milkdown/kit/core';
	import '@milkdown/crepe/theme/common/style.css';
	import '@milkdown/crepe/theme/frame.css';
	import { listener, listenerCtx } from '@milkdown/kit/plugin/listener';
	import type { InvokeInput } from '$lib/services';
	import type { EditorState } from '@milkdown/prose/state';
	import type { EditorView } from '@milkdown/prose/view';
	import { CircleHelp, MessageSquareText } from 'lucide-svelte/icons';
	import { tick } from 'svelte';
	import Input from '$lib/components/messages/Input.svelte';
	import { Bold, Italic, Strikethrough } from 'lucide-svelte';
	import { TooltipProvider } from '@milkdown/plugin-tooltip';
	import { tooltipFactory } from '@milkdown/plugin-tooltip';
	import { type Ctx } from '@milkdown/ctx';
	import { toggleStrongCommand, toggleEmphasisCommand } from '@milkdown/kit/preset/commonmark';
	import { toggleStrikethroughCommand } from '@milkdown/kit/preset/gfm';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { replaceAll } from '@milkdown/utils';
	import { debounce } from 'es-toolkit';

	interface Props {
		file: EditorItem;
		contents?: string;
		onInvoke?: (invoke: InvokeInput) => void | Promise<void>;
		onFileChanged?: (name: string, contents: string) => void;
		items: EditorItem[];
		class?: string;
		overrideContent?: string;
	}

	let {
		file,
		contents,
		onFileChanged,
		onInvoke,
		items,
		class: klass,
		overrideContent
	}: Props = $props();

	let ttDiv: HTMLDivElement | undefined = $state();
	let provider: TooltipProvider | undefined = $derived.by(() => {
		if (!ttDiv) {
			return;
		}
		const provider = new TooltipProvider({
			content: ttDiv
		});
		provider.onShow = () => {
			ttVisible = true;
		};
		provider.onHide = hide;
		return provider;
	});
	let ttVisible = $state(false);
	let ttImprove = $state(false);
	const tooltip = tooltipFactory('assistant-tooltip');
	let input: ReturnType<typeof Input> | undefined = $state();
	let focused = $state(false);
	let crepe: Crepe | undefined = $state();
	let editorCtx: Ctx;
	let editorView: EditorView | undefined = $state();
	let isEditing = $state(false);
	let lastSetValue = $state(contents ?? '');

	function hide() {
		ttVisible = false;
		ttImprove = false;
	}

	$effect(() => {
		if (!isEditing && overrideContent) {
			setValue(overrideContent, true);
		}

		if (!isEditing && !overrideContent && contents) {
			setValue(contents);
		}
	});

	async function setValue(value: string, isTemporary: boolean = false) {
		if (!crepe) return;
		crepe.editor.action(replaceAll(value));
		if (!isTemporary && value !== lastSetValue) {
			lastSetValue = value;
		}
	}

	async function onSubmit(input: InvokeInput) {
		input.improve = {
			filename: file.name,
			selection: getSelection()
		};
		await onInvoke?.(input);
		hide();
	}

	function getSelection(): string {
		if (!editorView) {
			return '';
		}
		return editorView.state.doc.textBetween(
			editorView.state.selection.from,
			editorView.state.selection.to,
			' '
		);
	}

	async function onExplain() {
		onInvoke?.({
			explain: {
				filename: file.name,
				selection: getSelection()
			}
		});
	}

	const debouncedOnFileChanged = debounce((changedContents: string) => {
		isEditing = false;
		if (onFileChanged && lastSetValue !== changedContents) {
			onFileChanged(file.name, changedContents);
		}
	}, 500);

	async function onBold() {
		editorCtx?.get(commandsCtx)?.call(toggleStrongCommand.key);
	}

	async function onItalic() {
		editorCtx?.get(commandsCtx)?.call(toggleEmphasisCommand.key);
	}

	async function onStrikethrough() {
		editorCtx?.get(commandsCtx)?.call(toggleStrikethroughCommand.key);
	}

	function ttUpdate(updatedView: EditorView, prevState: EditorState) {
		editorView = updatedView;
		provider?.update(updatedView, prevState);
	}

	function ttDestroy() {
		provider?.destroy();
		ttVisible = false;
		ttImprove = false;
	}

	function editor(node: HTMLElement) {
		const value = (contents || file?.file?.contents) ?? '';

		crepe = new Crepe({
			root: node,
			defaultValue: value,
			features: {
				[Crepe.Feature.Toolbar]: false
			}
		});

		crepe.editor
			.config((ctx) => {
				editorCtx = ctx;

				const listener = ctx.get(listenerCtx);
				listener.markdownUpdated((_ctx, markdown, prevMarkdown) => {
					if (overrideContent || focused) return;

					isEditing = true;
					if (markdown === prevMarkdown) {
						return;
					}

					debouncedOnFileChanged(markdown);
				});

				ctx.set(tooltip.key, {
					view: () => {
						return {
							update: ttUpdate,
							destroy: ttDestroy
						};
					}
				});
			})
			.use(listener)
			.use(tooltip);

		crepe.create();

		return {
			destroy: () => {
				crepe?.destroy();
				crepe = undefined;
				ttDiv = undefined;
			}
		};
	}
</script>

<div
	class={klass}
	use:editor
	onfocusin={() => (focused = true)}
	onfocusout={() => (focused = false)}
></div>

<div
	class="bg-surface1 absolute flex rounded-3xl shadow-lg"
	bind:this={ttDiv}
	class:hidden={!ttVisible}
>
	<button
		class="milkdown-action-btn rounded-s-3xl border-none"
		onclick={onBold}
		class:hidden={ttImprove}
	>
		<Bold class="h-5 w-5" />
	</button>
	<button class="milkdown-action-btn" onclick={onItalic} class:hidden={ttImprove}>
		<Italic class="h-5 w-5" />
	</button>
	<button class="milkdown-action-btn" onclick={onStrikethrough} class:hidden={ttImprove}>
		<Strikethrough class="h-5 w-5" />
	</button>
	<button class="milkdown-action-btn" onclick={onExplain} class:hidden={ttImprove}>
		<span class="text-sm">Explain</span>
		<CircleHelp class="h-5 w-5" />
	</button>
	<button
		class="milkdown-action-btn rounded-e-3xl"
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

<style lang="postcss">
	:global {
		.milkdown-action-btn {
			align-items: center;
			display: flex;
			gap: 0.5rem;
			padding: 1rem;
			padding-inline-start: 1.25rem;
			&:hover {
				background-color: var(--color-gray-100);
			}

			.dark &:hover {
				background-color: var(--color-gray-900);
			}
		}

		.milkdown {
			& milkdown-slash-menu {
				border-radius: 1.5rem; /* rounded-3xl */
				border: none;
				box-shadow: 0 10px 15px -3px rgb(0 0 0 / 0.1); /* shadow-lg */
				outline: none;
			}

			& milkdown-slash-menu .tab-group ul li {
				font-size: 1rem; /* text-base */
				font-weight: 400; /* font-normal */
			}

			& milkdown-slash-menu .menu-groups .menu-group li > span {
				font-weight: 400; /* font-normal */
			}
		}

		.milkdown {
			--crepe-color-background: transparent;
			--crepe-font-title:
				'Poppins', 'ui-sans-serif', 'system-ui', '-apple-system', 'system-ui', 'Segoe UI', 'Roboto',
				'Helvetica Neue', 'Arial', 'Noto Sans', 'sans-serif', 'Apple Color Emoji', 'Segoe UI Emoji',
				'Segoe UI Symbol', 'Noto Color Emoji';
			--crepe-font-default:
				'Poppins', 'ui-sans-serif', 'system-ui', '-apple-system', 'system-ui', 'Segoe UI', 'Roboto',
				'Helvetica Neue', 'Arial', 'Noto Sans', 'sans-serif', 'Apple Color Emoji', 'Segoe UI Emoji',
				'Segoe UI Symbol', 'Noto Color Emoji';
			--crepe-font-code:
				'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'Liberation Mono',
				'Courier New', 'monospace';
		}

		.milkdown .ProseMirror {
			padding: 0 1rem 1rem 1rem; /* px-4 pt-0 pb-4 */
			@media (min-width: 768px) {
				padding-left: 0;
				padding-right: 0; /* md:px-0 */
			}

			& h1 {
				margin-top: 1rem;
				margin-bottom: 1rem; /* my-4 */
				font-size: 1.5rem; /* text-2xl */
				font-weight: 700; /* font-bold */
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& h2 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1.25rem; /* text-xl */
				font-weight: 700;
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& h3,
			& h4 {
				margin-top: 1rem;
				margin-bottom: 1rem;
				font-size: 1rem; /* text-base */
				font-weight: 700;
				color: black;
				.dark & {
					color: var(--color-gray-100);
				}
			}

			& p {
				margin-bottom: 1rem;
				font-size: var(--text-md);
				color: var(--color-gray-900);
				.dark & {
					color: var(--color-gray-100);
				}
			}
		}

		.milkdown .operation-item {
			display: none;
		}

		.dark .milkdown {
			--crepe-color-background: transparent;
			--crepe-color-on-background: #e6e6e6;
			--crepe-color-surface: #121212;
			--crepe-color-surface-low: #1c1c1c;
			--crepe-color-on-surface: #d1d1d1;
			--crepe-color-on-surface-variant: #a9a9a9;
			--crepe-color-outline: #757575;
			--crepe-color-primary: #b5b5b5;
			--crepe-color-secondary: #4d4d4d;
			--crepe-color-on-secondary: #d6d6d6;
			--crepe-color-inverse: #e5e5e5;
			--crepe-color-on-inverse: #2a2a2a;
			--crepe-color-inline-code: #ff6666;
			--crepe-color-error: #ff6666;
			--crepe-color-hover: #232323;
			--crepe-color-selected: #2f2f2f;
			--crepe-color-inline-area: #2b2b2b;
			--crepe-shadow-1:
				0px 1px 2px 0px rgba(255, 255, 255, 0.3), 0px 1px 3px 1px rgba(255, 255, 255, 0.15);
			--crepe-shadow-2:
				0px 1px 2px 0px rgba(255, 255, 255, 0.3), 0px 2px 6px 2px rgba(255, 255, 255, 0.15);
		}
	}
</style>
