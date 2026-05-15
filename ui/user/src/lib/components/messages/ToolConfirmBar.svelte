<script lang="ts">
	import popover from '$lib/actions/popover.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		ChatService,
		type Project,
		type ToolConfirm,
		type ToolConfirmDecision,
		type Message
	} from '$lib/services';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { slide } from 'svelte/transition';

	interface Props {
		messages: Message[];
		project: Project;
		currentThreadID: string;
	}

	let { messages, project, currentThreadID }: Props = $props();

	let current = $derived.by(() => {
		const msg = messages.find((m) => m.toolConfirm && !m.done);
		if (!msg?.toolConfirm) return undefined;

		const toolName = msg.toolConfirm.toolName ?? '';
		const [serverName] = toolName.split(' -> ');
		const hasServer = toolName.includes(' -> ');

		return {
			confirm: msg.toolConfirm,
			displayName: msg.sourceName || toolName || '',
			serverName: hasServer ? serverName : '',
			serverPrefix: hasServer ? `${serverName} -> ` : ''
		};
	});

	let lastConfirmId = $state<string | undefined>(undefined);
	let isSubmitted = $state(false);
	let isExpanded = $state(false);

	$effect(() => {
		const currentId = current?.confirm.id;
		if (currentId !== lastConfirmId) {
			lastConfirmId = currentId;
			isSubmitted = false;
			isExpanded = false;
		}
	});

	async function handleConfirm(
		confirm: ToolConfirm,
		decision: ToolConfirmDecision,
		toolName?: string
	) {
		if (!current || confirm.id !== current.confirm.id) return;
		if (isSubmitted) return;

		if (decision !== 'deny') isSubmitted = true;
		isExpanded = false;

		try {
			await ChatService.sendToolConfirm(project.assistantID, project.id, currentThreadID, {
				id: confirm.id,
				decision,
				toolName
			});
		} catch (e) {
			isSubmitted = false;
			console.error('failed to send tool confirmation', e);
		}
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
		class="bg-base-200 text-base-content mb-2 w-full max-w-[900px] overflow-hidden rounded-xl px-5 shadow-lg"
	>
		{#key current.confirm.id}
			<div class="flex min-h-[48px] items-center gap-3 px-4 py-2.5">
				<div class="flex min-w-0 flex-1 items-center gap-2">
					<span class="text-base-content text-sm font-medium">{current.displayName}</span>

					{#if current.confirm.input}
						<button
							class="text-muted-content hover:text-base-content flex items-center gap-1 text-xs"
							onclick={() => (isExpanded = !isExpanded)}
						>
							{#if isExpanded}Hide details{:else}Show details{/if}
						</button>
					{/if}

					{#if isSubmitted}
						<Loading class="text-muted-content" />
					{/if}
				</div>

				<div class="flex shrink-0 items-center gap-2">
					{#if !isSubmitted}
						<button
							class="text-muted-content hover:bg-base-300 hover:text-base-content rounded px-3 py-1 text-xs transition-colors"
							onclick={() => handleConfirm(current.confirm, 'deny')}
						>
							Deny
						</button>

						<div class="bg-base-300 border-base-300 flex rounded-lg border">
							<button
								class="text-muted-content hover:bg-base-400 border-base-400 flex flex-1 items-center justify-center gap-1 rounded-l-lg rounded-r-none border-r px-3 py-1 text-xs transition-colors hover:opacity-80"
								onclick={() => handleConfirm(current.confirm, 'approve')}
							>
								Allow
							</button>

							<button
								use:dropdown.ref
								class="hover:bg-base-400 flex items-center justify-center rounded-l-none rounded-r-lg px-2 py-1 transition-colors hover:opacity-80"
								onclick={() => dropdown.toggle()}
							>
								<ChevronDown class="text-base-content size-3" />
							</button>
						</div>

						<div
							use:dropdown.tooltip
							class="bg-base-300 border-base-400 z-50 flex min-w-[180px] flex-col rounded-lg border py-1 shadow-xl"
						>
							<button
								class="text-base-content hover:bg-base-400 px-3 py-1.5 text-left text-xs transition-colors"
								onclick={() => {
									dropdown.toggle(false);
									handleConfirm(current.confirm, 'approve_thread', current.confirm.toolName);
								}}
							>
								Allow all {current.displayName} requests
							</button>

							{#if current.serverPrefix}
								<button
									class="text-base-content hover:bg-base-400 px-3 py-1.5 text-left text-xs transition-colors"
									onclick={() => {
										dropdown.toggle(false);
										handleConfirm(current.confirm, 'approve_thread', current.serverPrefix + '*');
									}}
								>
									Allow all {current.serverName} requests
								</button>
							{/if}

							<button
								class="text-base-content hover:bg-base-400 px-3 py-1.5 text-left text-xs transition-colors"
								onclick={() => {
									dropdown.toggle(false);
									handleConfirm(current.confirm, 'approve_thread', '*');
								}}
							>
								Allow all requests
							</button>
						</div>
					{/if}
				</div>
			</div>

			{#if isExpanded && current.confirm.input}
				<div class="border-base-300 border-t px-4 py-3" transition:slide={{ duration: 150 }}>
					<pre class="bg-base-200 text-base-content max-h-48 overflow-auto rounded p-3 text-xs">
{formatJson(current.confirm.input)}</pre>
				</div>
			{/if}
		{/key}
	</div>
{/if}
