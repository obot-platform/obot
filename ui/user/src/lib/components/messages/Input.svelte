<script lang="ts">
	import { type InvokeInput } from '$lib/services';
	import { editor } from '$lib/stores';
	import { autoHeight } from '$lib/actions/textarea.js';
	import { ArrowUp, LoaderCircle } from 'lucide-svelte';

	interface Props {
		onFocus?: () => void;
		onSubmit?: (input: InvokeInput) => void | Promise<void>;
		onAbort?: () => Promise<void>;
		placeholder?: string;
		readonly?: boolean;
		pending?: boolean;
	}

	let {
		onFocus,
		onSubmit,
		onAbort,
		readonly,
		pending,
		placeholder = 'Your message...'
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

		for (const file of editor) {
			if (file.modified && !file.taskID) {
				if (!input.changedFiles) {
					input.changedFiles = {};
				}
				input.changedFiles[file.name] = file.buffer;
			}
		}

		if (readonly || pending) {
			await onAbort?.();
			return;
		} else {
			await onSubmit?.(input);
		}

		if (input.changedFiles) {
			for (const file of editor) {
				if (input.changedFiles[file.name]) {
					file.contents = input.changedFiles[file.name];
					file.modified = false;
					file.buffer = '';
				}
			}
		}

		value = '';
	}

	async function onKey(e: KeyboardEvent) {
		if (e.key !== 'Enter' || e.shiftKey) {
			return;
		}
		e.preventDefault();
		await submit();
	}
</script>

<div class="w-full max-w-[700px]">
	<label for="chat" class="sr-only">Your messages</label>
	<div
		class="flex items-center rounded-3xl
	bg-gray-70
	!px-3
	focus-within:border-none
	focus-within:shadow-md
	focus-within:ring-1
	focus-within:ring-blue
	dark:border-none
	 dark:bg-gray-950"
	>
		<textarea
			use:autoHeight
			id="chat"
			rows="1"
			bind:value
			readonly={readonly || pending}
			onkeydown={onKey}
			bind:this={chat}
			onfocus={onFocus}
			class="peer
			!ml-4
			!mr-2
			w-full
			 resize-none bg-gray-70
			 !p-2.5 outline-none scrollbar-none dark:bg-gray-950"
			{placeholder}
		></textarea>
		<button
			type="submit"
			onclick={() => submit()}
			class="rounded-full bg-gray-70 !p-1
			text-blue
			hover:border-none
			hover:bg-gray-100
						 dark:bg-gray-950 dark:text-blue dark:hover:bg-gray-900"
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
</div>
