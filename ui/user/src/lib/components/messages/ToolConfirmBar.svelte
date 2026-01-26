<script lang="ts">
	import {
		ChatService,
		type Project,
		type ToolConfirm,
		type ToolConfirmDecision,
		type Message
	} from '$lib/services';
	import { slide } from 'svelte/transition';
	import popover from '$lib/actions/popover.svelte';
	import { ChevronDown, LoaderCircle } from 'lucide-svelte/icons';

	interface Props {
		messages: Message[];
		project: Project;
		currentThreadID: string;
	}

	let { messages, project, currentThreadID }: Props = $props();

	// Only track the first pending toolConfirm and its message, avoiding full array scans
	let current = $derived.by(() => {
		for (const msg of messages) {
			if (msg.toolConfirm && !msg.done) {
				return { confirm: msg.toolConfirm, message: msg };
			}
		}
		return undefined;
	});

	let displayName = $derived(current?.message.sourceName || current?.confirm.toolName || '');

	let isSubmitted = $state(false);
	let isExpanded = $state(false);

	// Reset state when the current confirm changes
	$effect(() => {
		if (current) {
			isSubmitted = false;
			isExpanded = false;
		}
	});

	async function handleConfirm(
		confirm: ToolConfirm,
		decision: ToolConfirmDecision,
		toolName?: string
	) {
		if (isSubmitted) return;

		// Only show loading spinner for approve actions, not deny
		if (decision !== 'deny') {
			isSubmitted = true;
		}

		await ChatService.sendToolConfirm(project.assistantID, project.id, currentThreadID, {
			id: confirm.id,
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

{#if current}
	{@const dropdown = popover({ placement: 'bottom-end' })}
	<div
		class="mb-2 w-full max-w-[900px] overflow-hidden rounded-xl bg-gray-900 px-5 shadow-lg dark:bg-gray-900"
		transition:slide={{ duration: 150 }}
	>
		{#key current.confirm.id}
			<div class="flex min-h-[48px] items-center gap-3 px-4 py-2.5">
				<!-- Tool name + details toggle -->
				<div class="flex min-w-0 flex-1 items-center gap-2">
					<span class="text-sm font-medium text-gray-100">{displayName}</span>
					{#if current.confirm.input}
						<button
							class="flex items-center gap-1 text-xs text-gray-400 hover:text-gray-300"
							onclick={() => (isExpanded = !isExpanded)}
						>
							{#if isExpanded}
								Hide details
							{:else}
								Show details
							{/if}
						</button>
					{/if}
					{#if isSubmitted}
						<LoaderCircle class="size-5 animate-spin text-gray-400" />
					{/if}
				</div>

				<!-- Buttons -->
				<div class="flex flex-shrink-0 items-center gap-2">
					{#if !isSubmitted}
						<button
							class="rounded px-3 py-1 text-xs text-gray-400 transition-colors hover:bg-gray-800 hover:text-gray-200"
							onclick={() => handleConfirm(current.confirm, 'deny')}
						>
							Deny
						</button>

						<div class="flex rounded-lg border border-gray-700 bg-gray-700 shadow-sm">
							<button
								class="flex flex-1 items-center justify-center gap-1 rounded-l-lg rounded-r-none border-r border-gray-600 px-3 py-1 text-xs font-medium text-gray-100 transition-colors hover:bg-gray-600"
								onclick={() => handleConfirm(current.confirm, 'approve')}
							>
								Allow
							</button>

							<button
								use:dropdown.ref
								class="flex items-center justify-center rounded-l-none rounded-r-lg px-2 py-1 transition-colors hover:bg-gray-600"
								onclick={() => dropdown.toggle()}
							>
								<ChevronDown class="size-3 text-gray-100" />
							</button>
						</div>

						<div
							use:dropdown.tooltip
							class="z-50 flex min-w-[180px] flex-col rounded-lg border border-gray-700 bg-gray-800 py-1 shadow-xl"
						>
							<button
								class="px-3 py-1.5 text-left text-xs text-gray-200 transition-colors hover:bg-gray-700"
								onclick={() => {
									handleConfirm(current.confirm, 'approve_thread', current.confirm.toolName);
									dropdown.toggle(false);
								}}
							>
								Allow {current.confirm.toolName} requests
							</button>
							<button
								class="px-3 py-1.5 text-left text-xs text-gray-200 transition-colors hover:bg-gray-700"
								onclick={() => {
									handleConfirm(current.confirm, 'approve_thread', '*');
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
			{#if isExpanded && current.confirm.input}
				<div class="border-t border-gray-800 px-4 py-3" transition:slide={{ duration: 150 }}>
					<pre
						class="max-h-48 overflow-auto rounded bg-gray-950 p-3 text-xs text-gray-300">{formatJson(
							current.confirm.input
						)}</pre>
				</div>
			{/if}
		{/key}
	</div>
{/if}
