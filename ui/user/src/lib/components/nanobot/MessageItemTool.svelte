<script lang="ts">
	import { onMount } from 'svelte';
	import { Settings } from 'lucide-svelte';
	import type {
		Attachment,
		ChatResult,
		ChatMessageItemToolCall,
		ToolOutputItem
	} from '$lib/services/nanobot/types';
	import MessageItemUI from '$lib/components/nanobot/MessageItemUI.svelte';
	import { isUIResource } from '@mcp-ui/client';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { isSafeImageMimeType } from '$lib/services/nanobot/utils';

	// Dynamically import web component only in browser (SSR-safe)
	onMount(() => {
		import('@mcp-ui/client/ui-resource-renderer.wc.js');
	});

	interface Props {
		item: ChatMessageItemToolCall;
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
	}

	let { item, onSend }: Props = $props();
	let singleUIResource = $derived(
		item.output?.content &&
			item.output?.content?.filter((i) => {
				return isUIResource(i) && !i.resource?._meta?.['ai.nanobot.meta/workspace'];
			}).length === 1
	);

	function parseToolInput(input: string) {
		try {
			const parsed = JSON.parse(input);
			if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
				return { success: true, data: parsed };
			}
		} catch {
			// JSON parsing failed, fall back to string display
		}
		return { success: false, data: input };
	}

	function getStyle(
		item: ToolOutputItem,
		singleUIResource: boolean = false
	): Record<string, string> {
		if (singleUIResource) {
			return {};
		}
		if (isUIResource(item) && item.resource._meta?.['mcpui.dev/ui-preferred-frame-size']) {
			const coords = item.resource._meta['mcpui.dev/ui-preferred-frame-size'];
			if (Array.isArray(coords) && coords[0] && coords[1]) {
				return {
					width: `${coords[0]}`,
					height: `${coords[1]}`
				};
			} else if (coords && typeof coords === 'object' && 'height' in coords && 'width' in coords) {
				return {
					width: `${coords.width}`,
					height: `${coords.height}`
				};
			}
		}
		return {
			width: '300px',
			height: '400px'
		};
	}

	function parseToolOutput(output: string) {
		try {
			const parsed = JSON.parse(output);
			const formattedJson = JSON.stringify(parsed, null, 2);
			const highlightedJson = toHTMLFromMarkdown('```json\n' + formattedJson + '\n```');
			return { success: true, data: highlightedJson };
		} catch {
			// JSON parsing failed, fall back to string display
		}
		return { success: false, data: output };
	}
</script>

<div
	class="text border-base-100 bg-base-100 hover:collapse-arrow hover:border-base-300 collapse w-full border"
>
	<input type="checkbox" />
	<div class="collapse-title">
		<div class="flex items-center gap-2">
			{#if item.output}
				<Settings class="text-primary/60 h-4 w-4" />
			{:else}
				<span class="loading loading-xs loading-spinner"></span>
			{/if}
			<span class="text-primary/60 text-sm font-medium">Tool call: {item.name}</span>
		</div>
	</div>
	<div class="collapse-content">
		<div class="space-y-3 pt-2">
			{#if item.arguments}
				<div class="grid">
					<div class="text-base-content/70 mb-1 text-xs font-medium">Input:</div>
					{#if parseToolInput(item.arguments).success}
						<div class="bg-base-200 overflow-x-auto rounded p-3">
							<table class="table-zebra table-xs table w-full">
								<thead>
									<tr>
										<th class="text-xs">Key</th>
										<th class="text-xs">Value</th>
									</tr>
								</thead>
								<tbody>
									{#each Object.entries(parseToolInput(item.arguments).data) as [key, value] (key)}
										<tr>
											<td class="font-mono text-xs">{key}</td>
											<td class="font-mono text-xs break-all">
												{typeof value === 'object' ? JSON.stringify(value) : String(value)}
											</td>
										</tr>
									{/each}
									{#if Object.keys(parseToolInput(item.arguments).data).length === 0}
										<tr>
											<td class="font-mono text-xs">No arguments</td>
										</tr>
									{/if}
								</tbody>
							</table>
						</div>
					{:else}
						<div class="bg-base-200 overflow-x-auto rounded p-3 font-mono text-sm">
							{item.arguments}
						</div>
					{/if}
				</div>
			{/if}
			{#if item.output}
				<div class="flex flex-col">
					<div class="text-base-content/70 mb-1 text-xs font-medium">Output:</div>
					{#if item.output.isError}
						<div class="alert alert-error">
							<span>Tool execution failed</span>
						</div>
					{/if}
					{#if item.output.structuredContent}
						<div
							class="prose border-success/20 bg-success/10 prose-invert overflow-x-auto rounded border p-3"
						>
							{@html parseToolOutput(JSON.stringify(item.output.structuredContent)).data}
						</div>
					{/if}
					{#if item.output.content}
						<!-- Render tool output content items -->
						{#each item.output.content as contentItem, i (i)}
							<div
								class="prose border-success/20 bg-success/10 prose-invert overflow-x-auto rounded border p-3"
							>
								{#if contentItem.type === 'text' && 'text' in contentItem && parseToolOutput(contentItem.text).success}
									<!-- JSON Syntax Highlighted Display -->
									{@html parseToolOutput(contentItem.text).data}
								{:else if contentItem.type === 'text' && 'text' in contentItem && contentItem.text}
									{@html toHTMLFromMarkdown(contentItem.text)}
								{:else}
									{@html parseToolOutput(JSON.stringify(contentItem)).data}
								{/if}
							</div>
						{/each}
					{/if}
				</div>
			{:else}
				<div class="text-base-content/50 flex items-center gap-2 text-xs italic">
					<span class="loading loading-xs loading-spinner"></span>
					Running...
				</div>
			{/if}
		</div>
	</div>
</div>

<div class="flex w-full flex-wrap items-start justify-start gap-2 p-2">
	{#if item.output && item.output.content}
		{#each item.output.content as contentItem, i (i)}
			{#if contentItem.type === 'resource' && contentItem.resource && isUIResource(contentItem) && !contentItem.resource._meta?.['ai.nanobot.meta/workspace']}
				<MessageItemUI
					item={contentItem}
					{onSend}
					style={getStyle(contentItem, singleUIResource)}
				/>
			{/if}
			{#if contentItem.type === 'image' && isSafeImageMimeType(contentItem.mimeType)}
				<img
					src="data:{contentItem.mimeType};base64,{contentItem.data}"
					alt="Tool output"
					class="max-w-full rounded"
				/>
			{/if}
		{/each}
	{/if}
</div>
