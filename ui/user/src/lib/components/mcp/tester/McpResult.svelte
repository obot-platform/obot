<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import type {
		DirectOperationResult,
		DirectOperationStatus
	} from '$lib/services/mcp/tester.svelte';
	import CornerCopyButton from './CornerCopyButton.svelte';
	import McpContent from './McpContent.svelte';
	import { Ban, CircleCheck, CircleAlert, Clock3, TriangleAlert } from '@lucide/svelte';

	interface Props {
		result: DirectOperationResult<unknown>;
		content?: unknown[];
		structuredContent?: unknown;
	}

	let { result, content = [], structuredContent }: Props = $props();
	let rawJSON = $derived(
		result.value === undefined ? undefined : JSON.stringify(result.value, null, 2)
	);

	function label(status: DirectOperationStatus): string {
		switch (status) {
			case 'success':
				return 'Succeeded';
			case 'mcp-error':
				return 'MCP error result';
			case 'denied':
				return 'Denied';
			case 'timeout':
				return 'Timed out';
			case 'cancelled':
				return 'Cancelled';
			default:
				return 'Request failed';
		}
	}
</script>

<section class="space-y-4" aria-label="Operation result">
	<div
		class={`flex flex-wrap items-center gap-2 rounded-lg p-3 text-sm ${
			result.status === 'success'
				? 'bg-success/10'
				: ['timeout', 'cancelled'].includes(result.status)
					? 'bg-warning/10'
					: 'bg-error/10'
		}`}
		role={result.status === 'success' ? 'status' : 'alert'}
	>
		{#if result.status === 'success'}
			<CircleCheck class="size-4 text-success" aria-hidden="true" />
		{:else if result.status === 'cancelled'}
			<Ban class="size-4 text-warning" aria-hidden="true" />
		{:else if result.status === 'timeout'}
			<Clock3 class="size-4 text-warning" aria-hidden="true" />
		{:else if result.status === 'mcp-error'}
			<CircleAlert class="size-4 text-error" aria-hidden="true" />
		{:else}
			<TriangleAlert class="size-4 text-error" aria-hidden="true" />
		{/if}
		<strong>{label(result.status)}</strong>
		<span class="text-muted-content">{Math.round(result.durationMs)} ms</span>
		{#if result.message}<span>{result.message}</span>{/if}
	</div>

	{#if content.length}
		<div class="space-y-3" aria-label="Rendered content">
			{#each content as item, index (index)}
				<McpContent content={item} collapseLongText />
			{/each}
		</div>
	{/if}

	{#if structuredContent !== undefined}
		<div>
			<h4 class="mb-2 text-sm font-medium">Structured content</h4>
			<CornerCopyButton
				text={JSON.stringify(structuredContent, null, 2)}
				label="Copy structured JSON"
			>
				<JsonPreview value={structuredContent} ariaLabel="Structured MCP content" />
			</CornerCopyButton>
		</div>
	{/if}

	{#if result.value !== undefined}
		<details>
			<summary class="cursor-pointer text-sm font-medium">Raw response</summary>
			<div class="mt-2">
				<!-- Offset so the copy icon sits beside the preview's maximize button. -->
				<CornerCopyButton text={rawJSON} label="Copy raw JSON" offset="2.25rem">
					<JsonPreview value={result.value} ariaLabel="Raw MCP response" maximizable />
				</CornerCopyButton>
			</div>
		</details>
	{/if}
</section>
