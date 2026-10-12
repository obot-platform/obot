<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import { m } from '$lib/i18n';
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
				return m.mcps_tester_call_succeeded();
			case 'mcp-error':
				return m.mcps_tester_status_mcp_error();
			case 'denied':
				return m.mcps_tester_status_denied();
			case 'timeout':
				return m.mcps_tester_status_timeout();
			case 'cancelled':
				return m.mcps_tester_call_cancelled();
			default:
				return m.mcps_tester_status_request_failed();
		}
	}
</script>

<section class="space-y-4" aria-label={m.mcps_tester_operation_result()}>
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
		<div class="space-y-3" aria-label={m.mcps_tester_rendered_content()}>
			{#each content as item, index (index)}
				<McpContent content={item} collapseLongText />
			{/each}
		</div>
	{/if}

	{#if structuredContent !== undefined}
		<div>
			<h4 class="mb-2 text-sm font-medium">{m.mcps_tester_structured_content()}</h4>
			<CornerCopyButton
				text={JSON.stringify(structuredContent, null, 2)}
				label={m.mcps_tester_copy_structured_json()}
			>
				<JsonPreview value={structuredContent} ariaLabel={m.mcps_tester_structured_mcp_content()} />
			</CornerCopyButton>
		</div>
	{/if}

	{#if result.value !== undefined}
		<details
			ontoggle={(event) => {
				if (event.currentTarget.open) {
					event.currentTarget.scrollIntoView({
						block: 'nearest',
						behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches
							? 'instant'
							: 'smooth'
					});
				}
			}}
		>
			<summary class="cursor-pointer text-sm font-medium">{m.mcps_tester_raw_response()}</summary>
			<div class="mt-2">
				<!-- Offset so the copy icon sits beside the preview's maximize button. -->
				<CornerCopyButton text={rawJSON} label={m.mcps_tester_copy_raw_json()} offset="2.25rem">
					<JsonPreview
						value={result.value}
						ariaLabel={m.mcps_tester_raw_mcp_response()}
						maximizable
					/>
				</CornerCopyButton>
			</div>
		</details>
	{/if}
</section>
