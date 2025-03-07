<script lang="ts">
	import { type InvokeInput } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea.js';
	import { ArrowUp, LoaderCircle } from 'lucide-svelte';
	import { onMount, type Snippet, tick } from 'svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		onFocus?: () => void;
		onSubmit?: (input: InvokeInput) => void | Promise<void>;
		onAbort?: () => Promise<void>;
		children?: Snippet;
		placeholder?: string;
		readonly?: boolean;
		pending?: boolean;
		items?: EditorItem[];
	}

	let {
		onFocus,
		onSubmit,
		onAbort,
		children,
		readonly,
		pending,
		placeholder = 'Your message...',
		items = $bindable([])
	}: Props = $props();

	let value = $state('');
	let chat: HTMLTextAreaElement;

	export function focus() {
		chat.focus();
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
		chat.dispatchEvent(new Event('resize'));
	}

	async function onKey(e: KeyboardEvent) {
		if (e.key !== 'Enter' || e.shiftKey) {
			return;
		}
		e.preventDefault();
		if (readonly || pending) {
			return;
		}
		await submit();
	}

	onMount(() => {
		focus();
	});
</script>

<div class="w-full max-w-[700px]">
	<label for="chat" class="sr-only">Your messages</label>
	<div
		class="relative flex flex-col items-center rounded-3xl bg-surface1 focus-within:shadow-md focus-within:ring-1 focus-within:ring-blue
"
	>
		<div class="flex w-full items-center px-6 py-4">
			<textarea
				use:autoHeight
				id="chat"
				rows="1"
				bind:value
				readonly={readonly || pending}
				onkeydown={onKey}
				bind:this={chat}
				onfocus={onFocus}
				class="grow resize-none border-none bg-surface1 outline-none"
				{placeholder}
			></textarea>
			<button
				type="submit"
				onclick={() => submit()}
				class="button-colors absolute bottom-2 right-2 rounded-full
				p-2
				text-blue
				hover:border-none"
			>
				{#if readonly}
					<div class="m-1.5 h-3 w-3 place-self-center rounded-sm bg-blue"></div>
				{:else if pending}
					<LoaderCircle class="animate-spin" />
				{:else}
					<ArrowUp />
				{/if}
				<span class="sr-only">Send message</span>
			</button>
		</div>
		{@render children?.()}
	</div>
</div>
