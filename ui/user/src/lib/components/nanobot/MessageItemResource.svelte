<script lang="ts">
	import { getFileIcon } from '$lib/components/nanobot/MessageAttachments.svelte';
	import { formatFileSize } from '$lib/format';
	import { m } from '$lib/i18n';
	import type { ChatMessageItemResource } from '$lib/services/nanobot/types';
	import { isCancellationError } from '$lib/services/nanobot/utils';
	import PDF from './PDF.svelte';
	import { CircleAlert, TriangleAlert } from '@lucide/svelte';

	interface Props {
		item: ChatMessageItemResource;
	}

	let { item }: Props = $props();

	let isError = $derived(item.resource.mimeType === 'application/vnd.nanobot.error+json');
	let modal = $state<HTMLDialogElement>();

	function openModal() {
		modal?.showModal();
	}

	function isTextType(mimeType: string): boolean {
		return (
			mimeType.startsWith('text/') ||
			mimeType.includes('json') ||
			mimeType.includes('xml') ||
			mimeType === 'application/javascript' ||
			mimeType === 'application/typescript'
		);
	}

	function isPdfType(mimeType: string): boolean {
		return mimeType === 'application/pdf';
	}

	function isCancelledError(text?: string): boolean {
		return isCancellationError(text);
	}

	function getResourceDisplayName(): string {
		// Use title or name if available, otherwise generate from MIME type
		if (item.resource.title) return item.resource.title;
		if (item.resource.name) return item.resource.name;

		// Generate friendly name from MIME type
		const mimeType = item.resource.mimeType;
		if (mimeType === 'application/json') return m.chat_resource_json_data();
		if (mimeType === 'application/xml') return m.chat_resource_xml_document();
		if (mimeType === 'application/pdf') return m.chat_resource_pdf_document();
		if (mimeType.startsWith('text/')) return m.chat_resource_text_document();
		if (mimeType.startsWith('image/')) return m.chat_resource_image();
		if (mimeType.includes('json')) return m.chat_resource_json_resource();
		if (mimeType.includes('html')) return m.chat_resource_html_document();
		if (mimeType.includes('csv')) return m.chat_resource_csv_data();
		if (mimeType.includes('markdown')) return m.chat_resource_markdown();

		// Fallback to MIME type
		return mimeType;
	}

	function getDecodedText(): string {
		if (!item.resource.blob) return '';
		try {
			// Unicode-safe base64 decoding
			const binaryString = atob(item.resource.blob);
			const bytes = Uint8Array.from(binaryString, (c) => c.charCodeAt(0));
			const str = new TextDecoder('utf-8').decode(bytes);
			try {
				return JSON.stringify(JSON.parse(str), null, 2);
			} catch {
				return str;
			}
		} catch {
			return m.chat_error_decoding_content();
		}
	}
</script>

{#if isError && isCancelledError(item.resource.text)}
	<div class="my-4 flex items-center gap-1 text-xs italic">
		<CircleAlert class="size-3" />
		{m.chat_message_aborted()}
	</div>
{:else if isError}
	<div class="border-error/20 bg-error/10 mt-3 mb-3 rounded-lg border p-3">
		<div class="mb-2 flex items-center gap-2 text-sm">
			<TriangleAlert class="text-error h-4 w-4" />
			<span class="text-error font-medium">{m.common_error()}</span>
		</div>
		{#if item.resource.text}
			<pre
				class="bg-base-100 text-error mt-2 rounded p-2 text-xs break-all whitespace-pre-wrap">{item
					.resource.text}</pre>
		{/if}
	</div>
{:else}
	<!-- Enhanced resource card -->
	<div class="card-compact card border-base-200/50 max-w-sm border shadow-md">
		<div class="card-body">
			<div class="flex items-start gap-3">
				<!-- Large icon -->
				<div class="shrink-0">
					<div class="bg-primary/10 flex h-12 w-12 items-center justify-center rounded-xl text-2xl">
						{getFileIcon(item.resource.mimeType)}
					</div>
				</div>

				<!-- Content -->
				<div class="min-w-0 flex-1">
					<h4 class="text-base-content truncate text-sm font-semibold">
						{getResourceDisplayName()}
					</h4>

					<!-- Description if available -->
					{#if item.resource.description}
						<p
							class="text-base-content/60 mt-1 text-xs"
							style="display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;"
						>
							{item.resource.description}
						</p>
					{/if}

					<!-- Metadata row -->
					<div class="mt-2 flex items-center gap-2">
						<span class="badge badge-ghost badge-xs">{item.resource.mimeType}</span>
						{#if item.resource.size}
							<span class="text-muted-content text-xs">
								{formatFileSize(item.resource.size)}
							</span>
						{/if}
					</div>
				</div>
			</div>

			<!-- Card actions -->
			<div class="card-actions mt-3 justify-end">
				<button type="button" class="btn btn-sm btn-primary" onclick={openModal}>
					{m.chat_view_content()}
				</button>
			</div>
		</div>
	</div>

	<!-- DaisyUI Dialog Modal -->
	<dialog bind:this={modal} class="modal modal-bottom sm:modal-middle">
		<div class="modal-box max-h-[80vh] max-w-4xl overflow-hidden">
			<div class="mb-4 flex items-center justify-between">
				<h3 class="flex items-center gap-3 text-lg font-bold">
					<div class="bg-primary/10 flex h-8 w-8 items-center justify-center rounded-lg">
						<span class="text-xl">{getFileIcon(item.resource.mimeType)}</span>
					</div>
					<div>
						<div class="text-base-content">{getResourceDisplayName()}</div>
						{#if item.resource.description}
							<div class="text-base-content/60 text-sm font-normal">
								{item.resource.description}
							</div>
						{/if}
					</div>
				</h3>
			</div>

			<div class="mb-4 flex items-center gap-2">
				<span class="badge badge-sm badge-primary">{item.resource.mimeType}</span>
				{#if item.resource.size}
					<span class="badge badge-ghost badge-sm">{formatFileSize(item.resource.size)}</span>
				{/if}
				{#if item.resource.annotations?.lastModified}
					<span class="text-muted-content text-xs">
						{m.chat_modified_date({
							date: new Date(item.resource.annotations.lastModified).toLocaleDateString()
						})}
					</span>
				{/if}
			</div>

			<div class="max-h-96 overflow-auto">
				{#if isTextType(item.resource.mimeType) && item.resource.blob}
					<div class="mockup-code">
						<pre><code>{getDecodedText()}</code></pre>
					</div>
				{:else if isPdfType(item.resource.mimeType) && item.resource.blob}
					<PDF
						base64={item.resource.blob}
						classes={{ iframe: 'border-base-300 h-96 w-full rounded border' }}
					/>
				{:else}
					<div class="py-8 text-center">
						<div class="mb-4 text-6xl">{getFileIcon(item.resource.mimeType)}</div>
						<p class="text-base-content/60">{m.chat_preview_not_available()}</p>
						{#if item.resource.blob}
							<p class="text-muted-content mt-2 text-sm">
								{m.chat_resource_data_cannot_preview()}
							</p>
						{:else}
							<p class="text-muted-content mt-2 text-sm">{m.chat_no_resource_data()}</p>
						{/if}
					</div>
				{/if}
			</div>

			<div class="modal-action">
				<form method="dialog">
					<button class="btn">{m.core_close()}</button>
				</form>
			</div>
		</div>
	</dialog>
{/if}
