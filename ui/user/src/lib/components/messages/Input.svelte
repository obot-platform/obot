<script lang="ts">
	import { type InvokeInput } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea.js';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { ArrowUp, Check, LoaderCircle, X } from 'lucide-svelte';
	import { onMount, type Snippet, tick } from 'svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onFocus?: () => void;
		onSubmit?: (input: InvokeInput) => void | Promise<void>;
		onAbort?: () => Promise<void>;
		children?: Snippet;
		placeholder?: string;
		readonly?: boolean;
		pending?: boolean;
		items?: EditorItem[];
		uploadState?: 'idle' | 'uploading' | 'success' | 'error';
	}

	let {
		onFocus,
		onSubmit,
		onAbort,
		children,
		readonly,
		pending,
		uploadState,
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

{#snippet submitButton()}
	<button
		type="submit"
		onclick={() => submit()}
		class="button-colors rounded-full p-2 text-blue transition-all duration-100 hover:border-none"
	>
		{#if readonly}
			<div class="m-1.5 h-3 w-3 place-self-center rounded-sm bg-white"></div>
		{:else if pending}
			<LoaderCircle class="animate-spin" />
		{:else}
			<ArrowUp />
		{/if}
		<span class="sr-only">Send message</span>
	</button>
{/snippet}

{#snippet uploadStatus()}
	{#if uploadState !== 'idle'}
		<button
			disabled
			class={twMerge(
				'me-2 flex items-center gap-2 rounded-full p-2 transition-colors duration-300',
				uploadState === 'uploading' && 'bg-blue-100 text-blue-500',
				uploadState === 'success' && 'bg-blue-100 text-blue-500',
				uploadState === 'error' && 'bg-red-200 text-red-500'
			)}
			transition:slide={{ axis: 'x' }}
		>
			{#if uploadState === 'uploading'}
				<LoaderCircle class="size-4 animate-spin" />
			{:else if uploadState === 'success'}
				<Check class="size-4" />
			{:else if uploadState === 'error'}
				<X class="size-4" />
			{/if}

			{#key uploadState}
				<span in:slide={{ axis: 'x' }} class="min-w-fit text-nowrap">
					{#if uploadState === 'uploading'}
						Uploading
					{:else if uploadState === 'success'}
						Upload Successful
					{:else if uploadState === 'error'}
						Upload Failed
					{/if}
				</span>
			{/key}
		</button>
	{/if}
{/snippet}

<div class="relative w-full px-5">
	<div
		class="relative flex flex-col items-center rounded-2xl bg-surface1 focus-within:shadow-md focus-within:ring-1 focus-within:ring-blue"
	>
		<div class="flex w-full items-center gap-4 p-2">
			<textarea
				use:autoHeight
				id="chat"
				rows="1"
				bind:value
				readonly={readonly || pending}
				onkeydown={onKey}
				bind:this={chat}
				onfocus={onFocus}
				class="grow resize-none rounded-xl border-none bg-transparent p-3 pr-20 text-md outline-none"
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
				{@render uploadStatus()}
				{@render submitButton()}
			</div>
		{/if}
	</div>
</div>
