<script lang="ts">
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import type { ChatMessageItemResourceLink, ResourceContents } from '$lib/services/nanobot/types';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';
	import PDF from './PDF.svelte';

	interface Props {
		item: ChatMessageItemResourceLink;
		onReadResource?: (uri: string) => Promise<{ contents: ResourceContents[] }>;
	}

	let { item, onReadResource }: Props = $props();

	let modal = $state<HTMLDialogElement>();
	let fetchedResource = $state<ResourceContents | null>(null);
	let loading = $state(false);
	let loadError = $state<string | null>(null);
	let isMissing = $state(false);

	const displayName = $derived(item.name || getNameFromURI(item.uri));
	const inferredMimeType = $derived(inferMimeType(item.uri));
	const previewMimeType = $derived(
		inferredMimeType !== 'application/octet-stream'
			? inferredMimeType
			: fetchedResource?.mimeType || item.mimeType || inferredMimeType
	);
	const previewBlob = $derived(fetchedResource?.blob || '');
	const previewText = $derived(fetchedResource?.text || '');

	const extensionMimeTypes: Record<string, string> = {
		txt: 'text/plain',
		md: 'text/markdown',
		markdown: 'text/markdown',
		json: 'application/json',
		yaml: 'application/yaml',
		yml: 'application/yaml',
		xml: 'application/xml',
		html: 'text/html',
		htm: 'text/html',
		csv: 'text/csv',
		js: 'application/javascript',
		ts: 'application/typescript',
		py: 'text/x-python',
		go: 'text/x-go',
		sh: 'application/x-sh',
		log: 'text/plain',
		pdf: 'application/pdf',
		png: 'image/png',
		jpg: 'image/jpeg',
		jpeg: 'image/jpeg',
		gif: 'image/gif',
		webp: 'image/webp',
		svg: 'image/svg+xml'
	};

	function handleClick() {
		openModal();
	}

	async function openModal() {
		modal?.showModal();
		if (fetchedResource || loading) return;
		if (!onReadResource) {
			loadError = 'Resource reading is not available.';
			return;
		}

		loading = true;
		loadError = null;

		try {
			const result = await onReadResource(item.uri);
			const content = result.contents?.find((c) => c.uri === item.uri) || result.contents?.[0];
			if (!content) {
				loadError = 'No content available for this resource.';
				isMissing = true;
				return;
			}
			fetchedResource = content;
			isMissing = false;
		} catch (e) {
			const errorMessage = e instanceof Error ? e.message : String(e);
			loadError = errorMessage;
			if (isMissingResourceError(errorMessage)) {
				isMissing = true;
			}
		} finally {
			loading = false;
		}
	}

	function getNameFromURI(uri: string): string {
		try {
			const url = new URL(uri);
			const decodedPath = decodeURIComponent(url.pathname || '');
			const fromPath = decodedPath.split('/').filter(Boolean).pop();
			if (fromPath) return fromPath;
		} catch {
			// ignore URL parse failures and fall back to raw URI parsing
		}

		const sanitized = uri.split('#')[0].split('?')[0];
		const fromRaw = sanitized.split('/').filter(Boolean).pop();
		return fromRaw || uri;
	}

	function inferMimeType(uri: string): string {
		const name = getNameFromURI(uri).toLowerCase();
		const ext = name.includes('.') ? name.split('.').pop() : '';
		if (!ext) return 'application/octet-stream';
		return extensionMimeTypes[ext] || 'application/octet-stream';
	}

	function isMissingResourceError(message: string): boolean {
		const lower = message.toLowerCase();
		return (
			lower.includes('not found') ||
			lower.includes('does not exist') ||
			lower.includes('missing') ||
			lower.includes('404')
		);
	}

	function isTextType(mimeType: string): boolean {
		return (
			mimeType.startsWith('text/') ||
			mimeType.includes('json') ||
			mimeType.includes('xml') ||
			mimeType.includes('yaml') ||
			mimeType === 'application/javascript' ||
			mimeType === 'application/typescript'
		);
	}

	function isMarkdownType(mimeType: string): boolean {
		return mimeType === 'text/markdown';
	}

	function isPdfType(mimeType: string): boolean {
		return mimeType === 'application/pdf';
	}

	function getDecodedText(blob?: string, text?: string): string {
		if (text) {
			try {
				return JSON.stringify(JSON.parse(text), null, 2);
			} catch {
				return text;
			}
		}
		if (!blob) return '';
		try {
			const binaryString = atob(blob);
			const bytes = Uint8Array.from(binaryString, (c) => c.charCodeAt(0));
			const str = new TextDecoder('utf-8').decode(bytes);
			try {
				return JSON.stringify(JSON.parse(str), null, 2);
			} catch {
				return str;
			}
		} catch {
			return 'Error decoding content';
		}
	}
</script>

<button
	type="button"
	class="mb-2 inline-flex max-w-full items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition-colors {isMissing
		? 'border-error/30 bg-error/10 text-error hover:bg-error/20'
		: 'border-base-300 bg-base-200 hover:bg-base-300'}"
	onclick={handleClick}
	title={displayName}
>
	<FileItem uri={item.uri} compact />
	<span class="truncate">{displayName}</span>
	{#if isMissing}
		<span class="badge badge-error badge-xs">Missing</span>
	{/if}
</button>

<!-- Preview modal -->
<dialog bind:this={modal} class="modal modal-bottom sm:modal-middle">
	<div class="modal-box max-h-[80vh] max-w-4xl overflow-hidden">
		<div class="mb-4 flex items-center justify-between">
			<h3 class="flex min-w-0 items-center gap-3 text-lg font-bold">
				<div class="bg-primary/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg">
					<FileItem uri={item.uri} compact />
				</div>
				<div class="min-w-0">
					<div class="text-base-content truncate">{displayName}</div>
					<div class="text-base-content/60 truncate text-sm font-normal">{item.uri}</div>
				</div>
			</h3>
		</div>

		<div class="mb-4 flex items-center gap-2">
			<span class="badge badge-primary badge-sm">{previewMimeType}</span>
		</div>

		<div class="max-h-96 overflow-auto">
			{#if loading}
				<div class="flex items-center gap-2 py-8">
					<span class="loading loading-sm loading-spinner"></span>
					<span>Loading preview...</span>
				</div>
			{:else if loadError}
				<div class="alert alert-error">
					<span>{loadError}</span>
				</div>
			{:else if previewMimeType && isMarkdownType(previewMimeType) && (previewBlob || previewText)}
				<div class="prose max-w-none">
					{@html toHTMLFromMarkdown(getDecodedText(previewBlob, previewText))}
				</div>
			{:else if previewMimeType && isTextType(previewMimeType) && (previewBlob || previewText)}
				<div class="mockup-code">
					<pre><code>{getDecodedText(previewBlob, previewText)}</code></pre>
				</div>
			{:else if previewMimeType && isPdfType(previewMimeType) && (previewBlob || previewText)}
				<PDF
					base64={previewBlob || previewText}
					classes={{ iframe: 'border-base-300 h-96 w-full rounded border' }}
				/>
			{:else if previewMimeType?.startsWith('image/') && isSafeImageMimeType(previewMimeType) && previewBlob}
				<div class="flex justify-center">
					<img
						src="data:{previewMimeType};base64,{previewBlob}"
						alt={displayName}
						class="max-h-96 rounded"
					/>
				</div>
			{:else}
				<div class="py-8 text-center">
					<p class="text-base-content/60">Preview not available for this resource type</p>
					<p class="text-base-content/40 mt-2 text-sm break-all">{item.uri}</p>
				</div>
			{/if}
		</div>

		<div class="modal-action">
			<form method="dialog">
				<button class="btn">Close</button>
			</form>
		</div>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button>close</button>
	</form>
</dialog>
