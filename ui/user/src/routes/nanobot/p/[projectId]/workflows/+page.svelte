<script lang="ts">
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import { Play, Search } from 'lucide-svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projectId);
	const chatApi = $derived(new ChatAPI(agent.connectURL));

	let workflowQuery = $state('');

	let workflows = $derived(
		$nanobotChat?.resources
			? $nanobotChat.resources.filter((r) => r.uri.startsWith('workflow:///'))
			: []
	);

	let filteredWorkflows = $derived(
		workflows.filter((w) => w.name.toLowerCase().includes(workflowQuery.toLowerCase()))
	);

	let workflowsContainer = $state<HTMLElement | undefined>(undefined);

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	function handleSelectWorkflow(workflowName: string) {
		const newChat = new ChatService({
			api: chatApi,
			onThreadCreated: (thread) => {
				nanobotChat.update((data) => {
					if (data) {
						if (data.chat && data.chat !== newChat) {
							data.chat.close();
						}
						data.chat = newChat;
						data.threadId = newChat.chatId;
					}
					return data;
				});

				goto(`/nanobot/p/${projectId}?tid=${thread.id}&wid=${workflowName}`, {
					replaceState: true,
					noScroll: true,
					keepFocus: true
				});
			}
		});

		newChat.sendMessage(`Run workflow: ${workflowName}`);
	}

	$effect(() => {
		const container = workflowsContainer;
		if (!container) return;

		const ro = new ResizeObserver((entries) => {
			const entry = entries[0];
			projectLayout.setThreadContentWidth(entry.contentRect.width);
		});
		ro.observe(container);
		projectLayout.setThreadContentWidth(container.getBoundingClientRect().width);
		return () => ro.disconnect();
	});
</script>

<div
	class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 md:px-8"
	bind:this={workflowsContainer}
>
	<label class="input w-full">
		<Search class="size-6" />
		<input type="search" required placeholder="Search workflows..." bind:value={workflowQuery} />
	</label>

	<div>
		<h2 class="text-2xl font-semibold">Workflows</h2>

		<p class="text-base-content/50 text-sm font-light">
			Workflows are AI-powered tools that can be used to automate tasks and processes.
		</p>
	</div>

	<table class="table">
		<!-- head -->
		<thead>
			<tr>
				<th>Name</th>
				<th></th>
			</tr>
		</thead>
		<tbody>
			{#if filteredWorkflows.length > 0}
				{#each filteredWorkflows as workflow (workflow.uri)}
					<tr
						class="hover:bg-base-200 cursor-pointer"
						role="button"
						tabindex="0"
						onclick={() => {
							goto(`/nanobot/p/${projectId}/workflows/${encodeURIComponent(workflow.name)}`);
						}}
						onkeydown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault();
								goto(`/nanobot/p/${projectId}/workflows/${encodeURIComponent(workflow.name)}`);
							}
						}}
					>
						<td>{workflow.name}</td>
						<td class="text-right">
							<button
								class="btn btn-ghost btn-square tooltip tooltip-left flex-shrink-0"
								data-tip="Run this workflow"
								onclick={(e) => {
									e.preventDefault();
									e.stopPropagation();
									handleSelectWorkflow(workflow.name);
								}}
							>
								<Play class="size-4" />
							</button>
						</td>
					</tr>
				{/each}
			{:else}
				<tr>
					<td colspan="2" class="text-base-content/50 text-center text-sm font-light italic">
						<span>No workflows found.</span>
					</td>
				</tr>
			{/if}
		</tbody>
	</table>
</div>
