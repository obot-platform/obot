<script lang="ts">
	import type {
		Agent,
		Attachment,
		ChatResult,
		Prompt,
		Resource,
		UploadedFile,
		UploadingFile
	} from '$lib/services/nanobot/types';
	import MessageAttachments from './MessageAttachments.svelte';
	import type MessageSlashPromptsType from './MessageSlashPrompts.svelte';
	import MessageSlashPrompts from './MessageSlashPrompts.svelte';
	import { CircleAlert, Paperclip, Send, Square, X } from 'lucide-svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onPrompt?: (promptName: string) => void;
		onFileUpload?: (file: File, opts?: { controller?: AbortController }) => Promise<Attachment>;
		onCancel?: () => void;
		cancelUpload?: (fileId: string) => void;
		uploadingFiles?: UploadingFile[];
		uploadedFiles?: UploadedFile[];
		placeholder?: string;
		disabled?: boolean;
		supportedMimeTypes?: string[];
		prompts?: Prompt[];
		agents?: Agent[];
		selectedAgentId?: string;
		onAgentChange?: (agentId: string) => void;
	}

	let {
		onSend,
		onFileUpload,
		onPrompt,
		onCancel,
		placeholder = 'Type a message...',
		disabled = false,
		uploadingFiles = [],
		uploadedFiles = [],
		cancelUpload,
		prompts = [],
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
	let uploadErrors = $state<string[]>([]);

	let selectedResources = $state<Resource[]>([]);
	const showAgentDropdown = $derived(agents.length > 1);

	function mimeMatchesPattern(fileType: string, pattern: string): boolean {
		if (pattern.endsWith('/*')) {
			const base = pattern.slice(0, -2);
			return fileType.startsWith(`${base}/`);
		}
		return fileType === pattern;
	}

	function isAcceptedUpload(file: File): boolean {
		if (!file.type) return false;
		return supportedMimeTypes.some((p) => mimeMatchesPattern(file.type, p));
	}

	function clipboardFiles(e: ClipboardEvent): File[] {
		const cd = e.clipboardData;
		if (!cd) return [];
		const out: File[] = [];
		for (let i = 0; i < cd.items.length; i++) {
			const item = cd.items[i];
			if (item.kind === 'file') {
				const f = item.getAsFile();
				if (f) out.push(f);
			}
		}
		if (out.length === 0 && cd.files.length > 0) {
			out.push(...Array.from(cd.files));
		}
		return out;
	}

	async function uploadFile(file: File) {
		if (!onFileUpload) return;
		const controller = new AbortController();
		await onFileUpload(file, { controller });
	}

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

	async function handleFileSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];

		if (!file || !onFileUpload) return;

		isUploading = true;
		try {
			await uploadFile(file);
		} finally {
			isUploading = false;
			target.value = '';
		}
	}

	async function handlePaste(e: ClipboardEvent) {
		if (!onFileUpload || disabled || isUploading) return;

		const toUpload = clipboardFiles(e).filter(isAcceptedUpload);
		const invalidUploads = clipboardFiles(e).filter((f) => !isAcceptedUpload(f));
		if (invalidUploads.length > 0) {
			uploadErrors =
				invalidUploads.length === 1
					? [`${invalidUploads[0].name} is not a supported file type and cannot be uploaded.`]
					: [
							`${invalidUploads.map((f) => f.name).join(', ')} are not supported files types and cannot be uploaded.`
						];
		}
		if (toUpload.length === 0) {
			return;
		}

		e.preventDefault();
		isUploading = true;
		try {
			for (const file of toUpload) {
				await uploadFile(file);
			}
		} finally {
			isUploading = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		uploadErrors = [];

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
			class="border-base-200 bg-base-100 focus-within:border-primary rounded-selector space-y-3 border-2 p-3 transition-colors"
		>
			<!-- Top row: Full-width input -->
			<textarea
				bind:value={message}
				onkeydown={handleKeydown}
				onpaste={handlePaste}
				oninput={autoResize}
				{placeholder}
				class="placeholder:text-base-content/50 max-h-32 min-h-[2.5rem] w-full resize-none bg-transparent p-1 text-sm leading-6 outline-none"
				rows="1"
				bind:this={textareaRef}
			></textarea>

			{#if uploadErrors.length > 0}
				<div
					class="alert alert-error alert-soft flex justify-between px-2 py-1 text-xs"
					transition:slide={{ axis: 'y', duration: 150 }}
				>
					<div class="flex items-center gap-1">
						<CircleAlert class="text-error size-3 flex-shrink-0" />
						<div class="flex flex-col gap-1">
							{#each uploadErrors as error, i (i)}
								{error}
							{/each}
						</div>
					</div>

					<button
						class="btn btn-xs btn-error btn-circle size-4 p-0.5"
						onclick={() => (uploadErrors = [])}
					>
						<X class="size-3" />
					</button>
				</div>
			{/if}

			{#if showAgentDropdown}
				<MessageAttachments
					{selectedResources}
					{uploadedFiles}
					{uploadingFiles}
					{removeSelectedResource}
					{cancelUpload}
				/>
			{/if}

			<!-- Bottom row: Agent select on left (if multiple agents), buttons on right -->
			<div class="flex items-end justify-between">
				<!-- Agent selector -->
				<div class="flex items-center gap-2">
					<button
						type="button"
						class="btn btn-circle btn-ghost tooltip"
						data-tip="Upload a file"
						disabled={disabled || !onFileUpload || isUploading}
						onclick={() => fileInput?.click()}
						aria-label="Upload a file"
					>
						<Paperclip class="size-4" />
					</button>
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
					{:else}
						<MessageAttachments
							{selectedResources}
							{uploadedFiles}
							{uploadingFiles}
							{removeSelectedResource}
							{cancelUpload}
						/>
					{/if}
				</div>

				<!-- Action buttons -->
				<div class="flex gap-2">
					<!-- Submit button -->
					{#if onCancel && disabled}
						<button
							onclick={onCancel}
							class="btn btn-sm btn-primary h-9 w-9 rounded-full p-0"
							aria-label="Stop generating"
						>
							<Square class="size-4" />
						</button>
					{:else}
						<button
							type="submit"
							class="btn btn-sm btn-primary h-9 w-9 rounded-full p-0"
							disabled={disabled || isUploading || !message.trim()}
							aria-label="Send message"
						>
							{#if disabled && !isUploading}
								<span class="loading loading-xs loading-spinner"></span>
							{:else}
								<Send class="size-4" />
							{/if}
						</button>
					{/if}
				</div>
			</div>
		</div>
	</form>
</div>
