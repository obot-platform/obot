<script lang="ts">
	import { type InvokeInput } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea.js';
	import { ArrowUp, LoaderCircle } from 'lucide-svelte';
	import { onMount, type Snippet, tick } from 'svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
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
		id,
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
	let chat: HTMLTextAreaElement | undefined = $state<HTMLTextAreaElement>();

	export function focus() {
		chat?.focus();
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

<div class="w-full px-5" {id}>
	<label for="chat" class="sr-only">Your messages</label>
	<div
		class="bg-surface1 focus-within:ring-blue relative flex flex-col items-center rounded-2xl focus-within:shadow-md focus-within:ring-1"
	>
		<div class="flex h-fit w-full items-center gap-4 p-2">
			<textarea
				use:autoHeight
				id="chat"
				rows="1"
				bind:value
				onkeydown={onKey}
				bind:this={chat}
				onfocus={onFocus}
				class={twMerge(
					'bg-surface1 text-md grow resize-none rounded-xl border-none p-3 pr-20 outline-hidden'
				)}
				{placeholder}
			></textarea>
			{#if !children}
				{@render submitButton()}
			{/if}
		</div>
		{#if children}
			<div class="flex w-full justify-between p-2 pt-0">
				{@render children?.()}
				<div class="grow"></div>
				{@render submitButton()}
			</div>
		{/if}
	</div>
</div>
