<script lang="ts">
	import { m } from '$lib/i18n';
	import type { MCPTesterChat } from '$lib/services/mcp/tester-chat.svelte';
	import CornerCopyButton from './CornerCopyButton.svelte';
	import { Ban, Check, X } from '@lucide/svelte';
	import { tick } from 'svelte';

	interface Props {
		chat: MCPTesterChat;
	}

	let { chat }: Props = $props();
	let approveButton = $state<HTMLButtonElement>();
	let focusedCallID = $state<string>();
	let queue = $derived(chat.pendingApprovals ?? []);
	let call = $derived(chat.activeApproval ?? chat.executingApproval);
	// The model can request several tools at once, but they are decided in order,
	// so the prompt only ever shows the head of the queue.
	let position = $derived(call ? queue.indexOf(call) + 1 : 0);
	let argumentsJSON = $derived(call ? JSON.stringify(call.arguments, null, 2) : '');

	$effect(() => {
		const pending = chat.activeApproval;
		if (!pending || !approveButton || pending.id === focusedCallID) return;
		focusedCallID = pending.id;
		// A pending call blocks the turn, so the decision takes the focus.
		void tick().then(() => approveButton?.focus());
	});
</script>

{#if call}
	<section
		class="border-warning/50 dark:border-warning/40 bg-base-100 dark:bg-base-200 mt-4 shrink-0 rounded-lg border p-4 shadow-sm"
		aria-label={m.mcps_tester_tool_approval()}
	>
		<div class="flex flex-wrap items-start justify-between gap-2">
			<div class="min-w-0">
				<h3 class="font-semibold break-all">{call.name}</h3>
				<p class="text-xs text-muted-content" aria-live="polite">
					{call.execution === 'executing'
						? m.mcps_tester_running()
						: m.mcps_tester_approval_needed()}
				</p>
			</div>
			{#if queue.length > 1}
				<span class="badge badge-outline"
					>{m.mcps_tester_tool_request_position({ position, total: queue.length })}</span
				>
			{/if}
		</div>

		<p class="mt-3 text-xs font-medium">{m.mcps_runtime_arguments()}</p>
		<CornerCopyButton text={argumentsJSON} label={m.mcps_tester_copy_arguments()} class="mt-1">
			<pre
				class="default-scrollbar-thin bg-base-200 dark:bg-base-300 max-h-40 overflow-auto rounded-lg p-3 pr-10 text-xs whitespace-pre-wrap wrap-break-word"
				aria-label={m.mcps_tester_arguments_named({ name: call.name })}>{argumentsJSON}</pre>
		</CornerCopyButton>

		{#if call.execution === 'executing'}
			<button
				type="button"
				class="btn btn-secondary btn-sm mt-3"
				onclick={() => chat.cancelExecutingTool()}
			>
				<Ban class="size-4" aria-hidden="true" />
				{m.mcps_tester_cancel_tool({ name: call.name })}
			</button>
		{:else}
			<div class="mt-3 flex flex-wrap gap-2">
				<button
					bind:this={approveButton}
					type="button"
					class="btn btn-primary btn-sm"
					onclick={() => chat.approve(call.id)}
				>
					<Check class="size-4" aria-hidden="true" />
					{m.mcps_tester_approve_tool({ name: call.name })}
				</button>
				<button type="button" class="btn btn-secondary btn-sm" onclick={() => chat.reject(call.id)}>
					<X class="size-4" aria-hidden="true" />
					{m.mcps_tester_reject_tool({ name: call.name })}
				</button>
			</div>
		{/if}
	</section>
{/if}
