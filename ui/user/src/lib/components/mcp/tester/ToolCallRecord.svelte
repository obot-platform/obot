<script lang="ts">
	import { m } from '$lib/i18n';
	import type { TesterToolApproval } from '$lib/services/mcp/tester-chat.svelte';
	import CornerCopyButton from './CornerCopyButton.svelte';
	import McpResult from './McpResult.svelte';
	import { Ban, CircleAlert, Check, X } from '@lucide/svelte';

	interface Props {
		calls: TesterToolApproval[];
	}

	let { calls }: Props = $props();

	// Only finished calls reach the transcript; anything still awaiting a
	// decision lives in the approval prompt above the composer.
	function outcome(call: TesterToolApproval) {
		switch (call.modelResult?.status) {
			case 'success':
				return { label: m.mcps_tester_call_succeeded(), icon: Check, class: 'text-success' };
			case 'rejected':
				return { label: m.mcps_tester_call_rejected(), icon: X, class: 'text-muted-content' };
			case 'cancelled':
				return { label: m.mcps_tester_call_cancelled(), icon: Ban, class: 'text-muted-content' };
			default:
				return { label: m.mcps_tester_call_failed(), icon: CircleAlert, class: 'text-error' };
		}
	}
</script>

<section class="mt-3 space-y-2" aria-label={m.mcps_tester_tool_calls()}>
	{#each calls as call (call.id)}
		{@const result = outcome(call)}
		<article
			class="border-base-300 dark:border-base-400 bg-base-100 rounded-lg border p-3"
			aria-label={m.mcps_tester_tool_call_named({ name: call.name })}
		>
			<div class="flex flex-wrap items-center justify-between gap-2">
				<h4 class="min-w-0 font-medium break-all">{call.name}</h4>
				<span class={`flex items-center gap-1 text-xs ${result.class}`}>
					<result.icon class="size-3.5" aria-hidden="true" />
					{result.label}
				</span>
			</div>

			<details class="mt-2">
				<summary class="cursor-pointer text-xs font-medium">{m.mcps_runtime_arguments()}</summary>
				<CornerCopyButton
					text={JSON.stringify(call.arguments, null, 2)}
					label={m.mcps_tester_copy_arguments()}
					class="mt-1"
				>
					<pre
						class="default-scrollbar-thin bg-base-200 dark:bg-base-300 max-h-64 overflow-auto rounded-lg p-3 pr-10 text-xs whitespace-pre-wrap wrap-break-word"
						aria-label={m.mcps_tester_arguments_named({ name: call.name })}>{JSON.stringify(
							call.arguments,
							null,
							2
						)}</pre>
				</CornerCopyButton>
			</details>

			{#if call.modelResult}
				<section class="mt-3" aria-label={m.mcps_tester_result_named({ name: call.name })}>
					{#if call.result}
						<McpResult
							result={call.result}
							content={call.result.value?.content ?? []}
							structuredContent={call.result.value?.structuredContent}
						/>
					{:else}
						<p class="text-sm">{String(call.modelResult.content)}</p>
					{/if}
				</section>
			{/if}
		</article>
	{/each}
</section>
