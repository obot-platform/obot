<script lang="ts">
	import { onMount, type Snippet, tick } from 'svelte';
	import { ArrowUp, LoaderCircle } from 'lucide-svelte';

	import { type InvokeInput } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	import PlaintextEditor from './PlaintextEditor.svelte';

	interface Props {
		id?: string;
		onFocus?: () => void;
		onSubmit?: (input: InvokeInput) => void | Promise<void>;
		onAbort?: () => Promise<void>;
		onChange?: (value: string) => void;
		onInputChange?: (value: string) => void;
		onArrowKeys?: (direction: 'up' | 'down') => void;
		children?: Snippet;
		placeholder?: string;
		readonly?: boolean;
		pending?: boolean;
		initialValue?: string;
		items?: EditorItem[];
		inputPopover?: Snippet<[string]>;
	}

	let {
		id,
		onFocus,
		onSubmit,
		onAbort,
		onChange,
		onInputChange,
		onArrowKeys,
		children,
		readonly,
		pending,
		placeholder = 'Your message...',
		initialValue,
		items = $bindable([]),
		inputPopover
	}: Props = $props();

	let value = $state(initialValue || '');
	let chat: HTMLDivElement | undefined = $state<HTMLDivElement>();
	let editor: PlaintextEditor | undefined = $state();

	// Public method to focus the editor
	export function focus() {
		return editor?.focus();
	}

	$effect(() => {
		if (!initialValue || (initialValue && initialValue !== value)) {
			onInputChange?.(value);
		}
	});

	export function getValue() {
		return value;
	}

	async function submit() {
		let input: InvokeInput = {
			prompt: value,
			changedFiles: {}
		};

		for (const file of items) {
			if (file && file.file?.modified && !file.file?.taskID) {
				if (!input.changedFiles) {
					input.changedFiles = {};
				}
				input.changedFiles[file.name] = file.file.buffer;
			}
		}

		if (readonly || pending) {
			await onAbort?.();
			return;
		} else {
			await onSubmit?.(input);
		}

		if (input.changedFiles) {
			for (const file of items) {
				if (input.changedFiles[file.name] && file.file) {
					file.file.contents = input.changedFiles[file.name];
					file.file.modified = false;
					file.file.buffer = '';
				}
			}
		}

		value = '';
		await tick();
		chat?.dispatchEvent(new Event('resize'));
	}

	async function onKey(e: KeyboardEvent) {
		if (onArrowKeys && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
			onArrowKeys(e.key === 'ArrowUp' ? 'up' : 'down');
			return;
		}

		if (e.key !== 'Enter' || e.shiftKey) {
			onChange?.(value);
			return;
		}

		e.preventDefault();
		if (readonly || pending) {
			return;
		}

		await submit();
	}

	export function clear() {
		value = '';
	}

	export function setValue(newValue: string) {
		value = newValue;
	}

	onMount(() => {
		focus();
	});
</script>

{#snippet submitButton()}
	<button
		type="submit"
		onclick={() => submit()}
		class="button-colors text-blue rounded-full p-2 transition-all duration-100 hover:border-none"
	>
		{#if readonly}
			<div class="m-1.5 h-3 w-3 place-self-center rounded-xs bg-white"></div>
		{:else if pending}
			<LoaderCircle class="animate-spin" />
		{:else}
			<ArrowUp />
		{/if}
		<span class="sr-only">Send message</span>
	</button>
{/snippet}

<div
	class=" focus-within:ring-blue bg-surface1 mt-4 flex h-fit max-h-[80svh] overflow-hidden rounded-2xl focus-within:shadow-md focus-within:ring-1"
>
	<!-- <div class="max-h-full min-h-full w-full flex-1 overflow-y-auto"></div> -->

	<div class="flex min-h-full w-full flex-col" {id}>
		<label for="chat" class="sr-only">Your messages</label>
		<div class="chat-grid relative flex flex-1 flex-col items-center overflow-hidden">
			<div class="flex h-full items-end overflow-hidden">
				<div class="scrollable relative flex max-h-full w-full flex-1 gap-4 overflow-y-auto p-2">
					{#if inputPopover}
						{@render inputPopover(value)}
					{/if}

					<PlaintextEditor
						bind:this={editor}
						bind:value
						{placeholder}
						onfocus={onFocus}
						onkeydown={onKey}
					></PlaintextEditor>

					{#if !children}
						{@render submitButton()}
					{/if}
				</div>
			</div>

			{#if children}
				<div class="chat-footer pointer-events-none z-1 flex w-full justify-between px-2 pb-2">
					<div class="pointer-events-auto flex flex-1">
						{@render children?.()}
						<div class="grow"></div>
						{@render submitButton()}
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.chat-grid {
		display: grid;
		grid-template-columns: 1fr;
		grid-template-rows: 1fr auto;
	}

	.chat-footer {
		background: linear-gradient(to bottom, rgb(0 0 0 / 0), var(--surface1) 40%);
	}
</style>
