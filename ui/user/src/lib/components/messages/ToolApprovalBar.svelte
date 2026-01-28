<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import { slide } from 'svelte/transition';
	import { flip } from 'svelte/animate';
	import popover from '$lib/actions/popover.svelte';
	import { ChevronDown, ChevronRight, LoaderCircle } from 'lucide-svelte/icons';
	import MessageIcon from './MessageIcon.svelte';

	interface PendingApproval {
		id: string;
		toolName: string;
		description?: string;
		input?: string;
		time?: Date;
		icon?: string;
		sourceName: string;
		done: boolean;
	}

	interface Props {
		pendingApprovals: PendingApproval[];
		project: Project;
		currentThreadID: string;
	}

	let { pendingApprovals, project, currentThreadID }: Props = $props();

	let submittedIds = $state<Set<string>>(new Set());
	let expandedIds = $state<Set<string>>(new Set());

	function toggleExpanded(id: string) {
		if (expandedIds.has(id)) {
			expandedIds = new Set([...expandedIds].filter((i) => i !== id));
		} else {
			expandedIds = new Set([...expandedIds, id]);
		}
	}

	async function handleConfirm(
		approval: PendingApproval,
		decision: 'deny' | 'approve' | 'approve_thread',
		toolName?: string
	) {
		if (submittedIds.has(approval.id)) return;

		// Only show loading spinner for approve actions, not skip
		if (decision !== 'deny') {
			submittedIds = new Set([...submittedIds, approval.id]);
		}

		await ChatService.sendToolApproval(project.assistantID, project.id, currentThreadID, {
			id: approval.id,
			decision,
			toolName
		});
	}

	function formatJson(jsonString: string): string {
		try {
			const parsed = JSON.parse(jsonString);
			return JSON.stringify(parsed, null, 2);
		} catch {
			return jsonString;
		}
	}
</script>

{#if pendingApprovals.length > 0}
	<div
		class="mb-2 flex w-full max-w-[1000px] flex-col gap-1.5 px-5"
		transition:slide={{ duration: 200 }}
	>
		{#each pendingApprovals as approval (approval.id)}
			{@const dropdown = popover({ placement: 'bottom-end' })}
			{@const isSubmitted = submittedIds.has(approval.id)}
			{@const isExpanded = expandedIds.has(approval.id)}
			<div
				class="overflow-hidden rounded-xl bg-gray-900 shadow-lg dark:bg-gray-900"
				animate:flip={{ duration: 200 }}
				transition:slide={{ duration: 150 }}
			>
				<div class="flex items-center gap-3 px-4 py-2.5">
					<div class="flex-shrink-0">
						{#if isSubmitted}
							<LoaderCircle class="size-5 animate-spin text-gray-400" />
						{:else}
							<MessageIcon
								msg={{
									runID: '',
									sourceName: approval.sourceName,
									icon: approval.icon,
									message: []
								}}
								class="size-5"
							/>
						{/if}
					</div>

					<!-- Tool name + details toggle -->
					<div class="flex min-w-0 flex-1 items-center gap-2">
						<span class="text-sm font-medium text-gray-100">{approval.toolName}</span>
						{#if approval.input}
							<button
								class="flex items-center gap-1 text-xs text-gray-400 hover:text-gray-300"
								onclick={() => toggleExpanded(approval.id)}
							>
								{#if isExpanded}
									<ChevronDown class="size-3" />
									Hide details
								{:else}
									<ChevronRight class="size-3" />
									Show details
								{/if}
							</button>
						{/if}
					</div>

					<!-- Buttons -->
					<div class="flex flex-shrink-0 items-center gap-2">
						{#if !isSubmitted}
							<button
								class="rounded px-3 py-1 text-xs text-gray-400 transition-colors hover:bg-gray-800 hover:text-gray-200"
								onclick={() => handleConfirm(approval, 'deny')}
								aria-label="Skip tool request for {approval.toolName}"
							>
								Skip
							</button>
							<button
								use:dropdown.ref
								class="flex items-center gap-1 rounded bg-gray-700 px-3 py-1 text-xs font-medium text-gray-100 transition-colors hover:bg-gray-600"
								onclick={() => dropdown.toggle()}
								aria-label="Allow tool request for {approval.toolName}"
							>
								Blocked
								<ChevronDown class="size-3" />
							</button>
							<div
								use:dropdown.tooltip
								class="z-50 flex min-w-[180px] flex-col rounded-lg border border-gray-700 bg-gray-800 py-1 shadow-xl"
							>
								<button
									class="px-3 py-1.5 text-left text-xs text-gray-200 transition-colors hover:bg-gray-700"
									onclick={() => {
										handleConfirm(approval, 'approve');
										dropdown.toggle(false);
									}}
								>
									Allow this request
								</button>
								<button
									class="px-3 py-1.5 text-left text-xs text-gray-200 transition-colors hover:bg-gray-700"
									onclick={() => {
										handleConfirm(approval, 'approve_thread', approval.toolName);
										dropdown.toggle(false);
									}}
								>
									Allow {approval.toolName} requests
								</button>
								<button
									class="px-3 py-1.5 text-left text-xs text-gray-200 transition-colors hover:bg-gray-700"
									onclick={() => {
										handleConfirm(approval, 'approve_thread', '*');
										dropdown.toggle(false);
									}}
								>
									Allow all requests
								</button>
							</div>
						{/if}
					</div>
				</div>

				<!-- Expanded input details -->
				{#if isExpanded && approval.input}
					<div class="border-t border-gray-800 px-4 py-3" transition:slide={{ duration: 150 }}>
						<pre
							class="max-h-48 overflow-auto rounded bg-gray-950 p-3 text-xs text-gray-300">{formatJson(
								approval.input
							)}</pre>
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}
