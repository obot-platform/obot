<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';
	import CornerCopyButton from './CornerCopyButton.svelte';

	interface Props {
		content: unknown;
		collapseLongText?: boolean;
	}

	let { content, collapseLongText = false }: Props = $props();

	const longTextCharacterLimit = 2000;
	const longTextLineLimit = 20;
	const textPreviewCharacterLimit = 600;
	const textPreviewLineLimit = 8;

	function record(value: unknown): Record<string, unknown> | undefined {
		return typeof value === 'object' && value !== null
			? (value as Record<string, unknown>)
			: undefined;
	}

	function safeExternalURL(value: unknown): string | undefined {
		if (typeof value !== 'string') return undefined;
		try {
			const url = new URL(value);
			return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : undefined;
		} catch {
			return undefined;
		}
	}

	function isLongText(value: string): boolean {
		return value.length > longTextCharacterLimit || value.split(/\r?\n/).length > longTextLineLimit;
	}

	function textPreview(value: string): string {
		return value
			.split(/\r?\n/)
			.slice(0, textPreviewLineLimit)
			.join('\n')
			.slice(0, textPreviewCharacterLimit);
	}

	let item = $derived(record(content));
	let type = $derived(typeof item?.type === 'string' ? item.type : undefined);
	let mimeType = $derived(typeof item?.mimeType === 'string' ? item.mimeType : undefined);
	let resource = $derived(record(item?.resource));
	let resourceMimeType = $derived(
		typeof resource?.mimeType === 'string' ? resource.mimeType : undefined
	);
	let externalURL = $derived(safeExternalURL(item?.uri));
	let collapsedText = $derived(
		type === 'text' && typeof item?.text === 'string' && collapseLongText && isLongText(item.text)
			? item.text
			: undefined
	);
	let collapsedTextPreview = $derived(collapsedText ? textPreview(collapsedText) : undefined);
	// The bordered content box. Shared so CornerCopyButton can *be* the box where a copy
	// control is offered, anchoring it to the box corner rather than to short content.
	const BOX = 'border-base-300 dark:border-base-400 rounded-lg border p-3';
	const safeAudioTypes = new Set([
		'audio/mpeg',
		'audio/mp3',
		'audio/wav',
		'audio/ogg',
		'audio/webm'
	]);
</script>

{#if type === 'text' && typeof item?.text === 'string'}
	<CornerCopyButton text={item.text} label="Copy text" class={BOX}>
		{#if collapsedText && collapsedTextPreview}
			<pre
				class="max-h-48 overflow-hidden pr-10 text-sm whitespace-pre-wrap wrap-break-word"
				aria-label="Text preview">{collapsedTextPreview}<span aria-hidden="true">…</span></pre>
			<details class="mt-3">
				<summary class="cursor-pointer text-sm font-medium">Show full text</summary>
				<pre
					class="mt-2 max-h-96 overflow-auto text-sm whitespace-pre-wrap wrap-break-word"
					aria-label="Full text">{collapsedText}</pre>
			</details>
		{:else}
			<pre
				class="overflow-auto pr-10 text-sm whitespace-pre-wrap wrap-break-word"
				aria-label="Text content">{item.text}</pre>
		{/if}
	</CornerCopyButton>
{:else if type === 'image' && typeof item?.data === 'string' && mimeType}
	<div class={BOX}>
		{#if isSafeImageMimeType(mimeType)}
			<img
				src={`data:${mimeType};base64,${item.data}`}
				alt="MCP server result"
				class="max-h-96 max-w-full rounded object-contain"
			/>
		{:else}
			<p class="text-sm text-muted-content">Unsupported image type: {mimeType}</p>
		{/if}
	</div>
{:else if type === 'audio' && typeof item?.data === 'string' && mimeType}
	<div class={BOX}>
		{#if safeAudioTypes.has(mimeType.toLowerCase())}
			<audio controls class="w-full">
				<source src={`data:${mimeType};base64,${item.data}`} type={mimeType} />
			</audio>
		{:else}
			<p class="text-sm text-muted-content">Unsupported audio type: {mimeType}</p>
		{/if}
	</div>
{:else if type === 'resource' && resource}
	{#if typeof resource.text === 'string'}
		<CornerCopyButton text={resource.text} label="Copy text" class={`${BOX} space-y-2`}>
			<p class="pr-10 text-xs font-medium break-all">
				{String(resource.uri ?? 'Embedded resource')}
			</p>
			<pre class="overflow-auto text-sm whitespace-pre-wrap wrap-break-word">{resource.text}</pre>
		</CornerCopyButton>
	{:else}
		<div class={`${BOX} space-y-2`}>
			<p class="text-xs font-medium break-all">{String(resource.uri ?? 'Embedded resource')}</p>
			{#if typeof resource.blob === 'string' && resourceMimeType && isSafeImageMimeType(resourceMimeType)}
				<img
					src={`data:${resourceMimeType};base64,${resource.blob}`}
					alt="Embedded MCP resource"
					class="max-h-96 max-w-full rounded object-contain"
				/>
			{:else}
				<p class="text-sm text-muted-content">
					Binary or unsupported embedded content ({resourceMimeType || 'unknown type'})
				</p>
			{/if}
		</div>
	{/if}
{:else if type === 'resource_link' && typeof item?.uri === 'string'}
	<div class={BOX}>
		{#if externalURL}
			<!-- eslint-disable svelte/no-navigation-without-resolve -- validated external resource URL -->
			<a
				class="link link-primary break-all"
				href={externalURL}
				target="_blank"
				rel="noopener noreferrer">{typeof item.name === 'string' ? item.name : item.uri}</a
			>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		{:else}
			<p class="break-all text-sm">{typeof item.name === 'string' ? item.name : item.uri}</p>
			<p class="mt-1 text-xs text-muted-content break-all">{item.uri}</p>
		{/if}
		{#if typeof item.description === 'string'}
			<p class="mt-1 text-sm text-muted-content">{item.description}</p>
		{/if}
	</div>
{:else}
	<div class={BOX}>
		<p class="mb-2 text-sm text-muted-content">Unsupported content. Raw metadata is shown.</p>
		<JsonPreview value={content} ariaLabel="Unsupported MCP content" />
	</div>
{/if}
