<script lang="ts">
	import { Paperclip, Send } from 'lucide-svelte';
	import MessageAttachments from './MessageAttachments.svelte';
	import MessageResources from './MessageResources.svelte';
	import type MessageSlashPromptsType from './MessageSlashPrompts.svelte';
	import MessageSlashPrompts from './MessageSlashPrompts.svelte';
	import type {
		Agent,
		Attachment,
		ChatMessage,
		ChatResult,
		Prompt,
		Resource,
		UploadedFile,
		UploadingFile
	} from '$lib/services/nanobot/types';

	interface Props {
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onPrompt?: (promptName: string) => void;
		onFileUpload?: (file: File, opts?: { controller?: AbortController }) => Promise<Attachment>;
		cancelUpload?: (fileId: string) => void;
		uploadingFiles?: UploadingFile[];
		uploadedFiles?: UploadedFile[];
		placeholder?: string;
		disabled?: boolean;
		supportedMimeTypes?: string[];
		prompts?: Prompt[];
		resources?: Resource[];
		messages?: ChatMessage[];
		agents?: Agent[];
		selectedAgentId?: string;
		onAgentChange?: (agentId: string) => void;
	}

	let {
		onSend,
		onFileUpload,
		onPrompt,
		placeholder = 'Type a message...',
		disabled = false,
		uploadingFiles = [],
		uploadedFiles = [],
		cancelUpload,
		prompts = [],
		resources = [],
		messages = [],
		agents = [],
		selectedAgentId = '',
		onAgentChange,
		supportedMimeTypes = [
			'image/*',
			'text/plain',
			'application/pdf',
			'application/json',
			'text/csv'
		]
	}: Props = $props();

	let message = $state('');
	let fileInput: HTMLInputElement;
	let textareaRef: HTMLTextAreaElement;
	let slashInput: MessageSlashPromptsType;
	let isUploading = $state(false);

	let selectedResources = $state<Resource[]>([]);
	const showAgentDropdown = $derived(agents.length > 1);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (message.trim() && onSend) {
			textareaRef?.focus();
			onSend(message.trim(), selectedResources);
			message = '';
			selectedResources = [];
		}
	}

	function removeSelectedResource(resource: Resource) {
		selectedResources = selectedResources.filter((r) => r.uri !== resource.uri);
	}

	function toggleResource(resource: Resource) {
		const isSelected = selectedResources.some((r) => r.uri === resource.uri);
		if (isSelected) {
			selectedResources = selectedResources.filter((r) => r.uri !== resource.uri);
		} else {
			selectedResources = [...selectedResources, resource];
		}
	}

	function handleAttach() {
		fileInput?.click();
	}

	async function handleFileSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];

		if (!file || !onFileUpload) return;

		const controller = new AbortController();

		isUploading = true;

		try {
			await onFileUpload(file, { controller });
		} finally {
			isUploading = false;
			target.value = '';
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (slashInput.handleKeydown(e)) {
			return;
		}

		if (e.key === 'Escape') {
			if (message.trim().startsWith('/')) {
				message = '';
			}
		}

		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			if (disabled || isUploading) {
				return;
			}
			handleSubmit(e);
		}
	}

	function autoResize() {
		if (!textareaRef) return;

		textareaRef.style.height = '0';

		const newHeight = Math.min(Math.max(textareaRef.scrollHeight, 40), 128); // min 40px (2.5rem), max 128px (8rem)
		textareaRef.style.height = `${newHeight}px`;
	}

	// Auto-resize when message changes
	$effect(() => {
		void message;
		if (textareaRef) {
			autoResize();
		}
	});
</script>

<div class="p-0 md:p-4">
	<MessageSlashPrompts
		bind:this={slashInput}
		{prompts}
		{message}
		onPrompt={(p) => {
			message = '';
			onPrompt?.(p);
		}}
	/>

	<!-- Hidden file input -->
	<input
		bind:this={fileInput}
		type="file"
		accept={supportedMimeTypes.join(',')}
		onchange={handleFileSelect}
		class="hidden"
		aria-label="File upload"
	/>

	<form onsubmit={handleSubmit}>
		<div
			class="rounded-t-selector border-base-200 bg-base-100 focus-within:border-primary md:rounded-selector space-y-3 border-2 p-3 transition-colors"
		>
			<!-- Top row: Full-width input -->
			<textarea
				bind:value={message}
				onkeydown={handleKeydown}
				oninput={autoResize}
				{placeholder}
				class="placeholder:text-base-content/50 max-h-32 min-h-[2.5rem] w-full resize-none bg-transparent p-1 text-sm leading-6 outline-none"
				rows="1"
				bind:this={textareaRef}
			></textarea>

			<!-- Bottom row: Agent select on left (if multiple agents), buttons on right -->
			<div
				class="flex items-center {uploadedFiles.length > 0 ||
				uploadingFiles.length > 0 ||
				selectedResources.length > 0 ||
				showAgentDropdown
					? 'justify-between'
					: 'justify-end'}"
			>
				<!-- Agent selector -->
				{#if showAgentDropdown}
					<select
						class="select select-ghost select-sm w-48"
						disabled={disabled || isUploading}
						value={selectedAgentId}
						onchange={(e) => onAgentChange?.(e.currentTarget.value)}
					>
						{#each agents as agent (agent.id)}
							<option value={agent.id}>
								{agent.name}{agent.current ? ' (default)' : ''}
							</option>
						{/each}
					</select>
				{/if}

				<MessageAttachments
					{selectedResources}
					{uploadedFiles}
					{uploadingFiles}
					{removeSelectedResource}
					{cancelUpload}
				/>

				<!-- Action buttons -->
				<div class="flex gap-2">
					<!-- Attach button -->
					<button
						type="button"
						onclick={handleAttach}
						class="btn btn-ghost btn-sm h-9 w-9 rounded-full p-0"
						disabled={disabled || isUploading}
						aria-label="Attach file"
					>
						{#if isUploading}
							<span class="loading loading-xs loading-spinner"></span>
						{:else}
							<Paperclip class="h-4 w-4" />
						{/if}
					</button>

					<MessageResources
						{disabled}
						{resources}
						{selectedResources}
						{toggleResource}
						{messages}
					/>

					<!-- Submit button -->
					<button
						type="submit"
						class="btn btn-sm btn-primary h-9 w-9 rounded-full p-0"
						disabled={disabled || isUploading || !message.trim()}
						aria-label="Send message"
					>
						{#if disabled && !isUploading}
							<span class="loading loading-xs loading-spinner"></span>
						{:else}
							<Send class="h-4 w-4" />
						{/if}
					</button>
				</div>
			</div>
		</div>
	</form>
</div>
