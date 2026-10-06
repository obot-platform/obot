<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import { m } from '$lib/i18n';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';
	import CornerCopyButton from './CornerCopyButton.svelte';
	import McpTextResult from './McpTextResult.svelte';

	interface Props {
		content: unknown;
		collapseLongText?: boolean;
	}

	let { content, collapseLongText = false }: Props = $props();

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

	let item = $derived(record(content));
	let type = $derived(typeof item?.type === 'string' ? item.type : undefined);
	let mimeType = $derived(typeof item?.mimeType === 'string' ? item.mimeType : undefined);
	let resource = $derived(record(item?.resource));
	let resourceMimeType = $derived(
		typeof resource?.mimeType === 'string' ? resource.mimeType : undefined
	);
	let externalURL = $derived(safeExternalURL(item?.uri));
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
	{#if collapseLongText}
		{#key item.text}
			<McpTextResult text={item.text} />
		{/key}
	{:else}
		<CornerCopyButton text={item.text} label={m.mcps_tester_copy_text()} class={BOX}>
			<pre
				class="overflow-auto pr-10 text-sm whitespace-pre-wrap wrap-break-word"
				aria-label={m.mcps_tester_text_content()}>{item.text}</pre>
		</CornerCopyButton>
	{/if}
{:else if type === 'image' && typeof item?.data === 'string' && mimeType}
	<div class={BOX}>
		{#if isSafeImageMimeType(mimeType)}
			<img
				src={`data:${mimeType};base64,${item.data}`}
				alt={m.mcps_tester_server_result_alt()}
				class="max-h-96 max-w-full rounded object-contain"
			/>
		{:else}
			<p class="text-sm text-muted-content">
				{m.mcps_tester_unsupported_image_type({ type: mimeType })}
			</p>
		{/if}
	</div>
{:else if type === 'audio' && typeof item?.data === 'string' && mimeType}
	<div class={BOX}>
		{#if safeAudioTypes.has(mimeType.toLowerCase())}
			<audio controls class="w-full">
				<source src={`data:${mimeType};base64,${item.data}`} type={mimeType} />
			</audio>
		{:else}
			<p class="text-sm text-muted-content">
				{m.mcps_tester_unsupported_audio_type({ type: mimeType })}
			</p>
		{/if}
	</div>
{:else if type === 'resource' && resource}
	{#if typeof resource.text === 'string'}
		<CornerCopyButton
			text={resource.text}
			label={m.mcps_tester_copy_text()}
			class={`${BOX} space-y-2`}
		>
			<p class="pr-10 text-xs font-medium break-all">
				{String(resource.uri ?? m.mcps_tester_embedded_resource())}
			</p>
			<pre class="overflow-auto text-sm whitespace-pre-wrap wrap-break-word">{resource.text}</pre>
		</CornerCopyButton>
	{:else}
		<div class={`${BOX} space-y-2`}>
			<p class="text-xs font-medium break-all">
				{String(resource.uri ?? m.mcps_tester_embedded_resource())}
			</p>
			{#if typeof resource.blob === 'string' && resourceMimeType && isSafeImageMimeType(resourceMimeType)}
				<img
					src={`data:${resourceMimeType};base64,${resource.blob}`}
					alt={m.mcps_tester_embedded_resource_alt()}
					class="max-h-96 max-w-full rounded object-contain"
				/>
			{:else}
				<p class="text-sm text-muted-content">
					{m.mcps_tester_unsupported_embedded({
						type: resourceMimeType || m.mcps_tester_unknown_type()
					})}
				</p>
			{/if}
		</div>
	{/if}
{:else if type === 'resource_link' && typeof item?.uri === 'string'}
	<div class={BOX}>
		{#if externalURL}
			<a
				class="link link-primary break-all"
				href={externalURL}
				target="_blank"
				rel="external noopener noreferrer">{typeof item.name === 'string' ? item.name : item.uri}</a
			>
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
		<p class="mb-2 text-sm text-muted-content">
			{m.mcps_tester_unsupported_content()}
		</p>
		<JsonPreview value={content} ariaLabel={m.mcps_tester_unsupported_mcp_content()} />
	</div>
{/if}
