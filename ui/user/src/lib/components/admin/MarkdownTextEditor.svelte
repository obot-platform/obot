<script lang="ts">
	import { Crepe } from '@milkdown/crepe';
	import '@milkdown/crepe/theme/common/style.css';
	import '@milkdown/crepe/theme/frame.css';
	import { commandsCtx } from '@milkdown/kit/core';
	import { listener, listenerCtx } from '@milkdown/kit/plugin/listener';
	import type { EditorState } from '@milkdown/prose/state';
	import type { EditorView } from '@milkdown/prose/view';
	import { Bold, Italic, Strikethrough } from 'lucide-svelte';
	import { TooltipProvider } from '@milkdown/plugin-tooltip';
	import { tooltipFactory } from '@milkdown/plugin-tooltip';
	import { type Ctx } from '@milkdown/ctx';
	import { toggleStrongCommand, toggleEmphasisCommand } from '@milkdown/kit/preset/commonmark';
	import { toggleStrikethroughCommand } from '@milkdown/kit/preset/gfm';

	interface Props {
		class?: string;
		value?: string;
		onUpdate?: (markdown: string) => void;
		onCancel?: () => void;
		initialFocus?: boolean;
	}

	let { value = $bindable(''), class: klass, onUpdate, onCancel, initialFocus }: Props = $props();

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
	const tooltip = tooltipFactory('assistant-tooltip');
	let lastSetValue = '';
	let crepe: Crepe | undefined = $state();
	let editorCtx: Ctx;

	function hide() {
		ttVisible = false;
	}

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
		provider?.update(updatedView, prevState);
	}

	function ttDestroy() {
		provider?.destroy();
		ttVisible = false;
	}

	function editor(node: HTMLElement) {
		lastSetValue = value ?? '';

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
					if (markdown === prevMarkdown || markdown === lastSetValue) {
						return;
					}

					value = markdown;
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

		// Focus the editor if initialFocus is true
		if (initialFocus) {
			// Use setTimeout to ensure the editor is fully rendered
			setTimeout(() => {
				const editorElement = node.querySelector('.ProseMirror') as HTMLElement;
				if (editorElement) {
					editorElement.focus();
				}
			}, 10);
		}

		return {
			destroy: () => {
				crepe?.destroy();
				crepe = undefined;
				lastSetValue = '';
				ttDiv = undefined;
			}
		};
	}
</script>

<div
	class={klass}
	use:editor
	onfocusout={() => {
		if (value !== lastSetValue) {
			onUpdate?.(value);
		} else {
			onCancel?.();
		}
	}}
></div>

<div
	class="bg-surface1 absolute flex overflow-hidden rounded-3xl shadow-lg"
	bind:this={ttDiv}
	class:hidden={!ttVisible}
>
	<button class="milkdown-action-btn rounded-s-3xl border-none" onclick={onBold}>
		<Bold class="h-5 w-5" />
	</button>
	<button class="milkdown-action-btn" onclick={onItalic}>
		<Italic class="h-5 w-5" />
	</button>
	<button class="milkdown-action-btn" onclick={onStrikethrough}>
		<Strikethrough class="h-5 w-5" />
	</button>
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
			padding: 0; /* px-4 pt-0 pb-4 */
			& h1,
			& h2,
			& h3,
			& h4,
			& p {
				padding-top: 0;
				padding-bottom: 0;

				margin-top: 0;
				margin-bottom: 1rem;
				line-height: initial;
				&:first-child {
					margin-top: 0;
				}
				&:last-child {
					margin-bottom: 0;
				}
			}

			& h1 {
				font-size: 1.5rem; /* text-2xl */
				font-weight: 700; /* font-bold */
			}

			& h2 {
				font-size: 1.25rem; /* text-xl */
				font-weight: 700;
			}

			& h3,
			& h4 {
				font-size: 1rem; /* text-base */
				font-weight: 700;
			}

			& p {
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

				& li {
					margin-bottom: 0.5rem;
				}

				& ::marker {
					color: var(--color-gray-500);
				}
			}

			& ul {
				margin: 1rem 0;
				list-style-type: disc;
				padding-left: 1rem;

				& li {
					margin-bottom: 0.5rem;
				}

				& ::marker {
					color: var(--color-gray-500);
				}
			}

			& img {
				justify-self: center;
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

		/* Override default placeholder text */
		.milkdown .crepe-placeholder::before,
		.milkdown [data-placeholder]::before,
		.milkdown .ProseMirror[data-placeholder]:empty::before {
			content: 'Add description here...' !important;
		}

		/* Additional selectors to catch different placeholder implementations */
		.milkdown .ProseMirror:empty::before {
			content: 'Add description here...' !important;
			color: var(--color-gray-400);
			pointer-events: none;
			position: absolute;
			top: 0;
			left: 0;
		}

		.dark .milkdown .ProseMirror:empty::before {
			color: var(--color-gray-600);
		}
	}
</style>
