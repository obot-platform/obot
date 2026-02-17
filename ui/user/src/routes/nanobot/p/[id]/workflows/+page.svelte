<script lang="ts">
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import { Search } from 'lucide-svelte';
	import { getContext } from 'svelte';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projectId);
	const chatApi = $derived(new ChatAPI(agent.connectURL));

	let workflows = $derived(
		$nanobotChat?.chat?.resources
			? $nanobotChat.chat.resources.filter((r) => r.uri.startsWith('workflow:///'))
			: []
	);

	let runs = $derived(
		($nanobotChat?.chat?.resources
			? $nanobotChat.chat.resources.filter((r) => r.uri.startsWith('file:///workflows/.runs/'))
			: []
		).map((r) => ({
			...r,
			displayLabel: r.uri.replace('file:///workflows/.runs/', '').replace('.md', '')
		}))
	);

	let selectedWorkflowName = $state<string | undefined>(undefined);
	let workflowsContainer = $state<HTMLElement | undefined>(undefined);

	const projectLayout = getContext<{
		chat: import('$lib/services/nanobot/chat/index.svelte').ChatService | null;
		handleFileOpen: (filename: string) => void;
		setThreadContentWidth: (w: number) => void;
	}>('nanobot-project-layout');

	function handleSelectWorkflow(workflowName: string) {
		selectedWorkflowName = workflowName;
		const newChat = new ChatService({
			api: chatApi
		});

		nanobotChat.update((data) => {
			if (data) {
				if (data.chat && data.chat !== newChat) {
					data.chat.close();
				}
				data.chat = newChat;
				data.threadId = undefined;
			}
			return data;
		});

		goto(`/nanobot/p/${projectId}?wid=${selectedWorkflowName}`, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
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
	<div>
		<h2 class="text-2xl font-semibold">Workflows</h2>

		<p class="text-base-content/50 text-sm font-light">
			Workflows are AI-powered tools that can be used to automate tasks and processes.
		</p>
	</div>

	<h3 class="text-lg font-semibold">All Workflows</h3>

	<label class="input w-full">
		<Search class="size-6" />
		<input type="search" required placeholder="Search workflows..." />
	</label>

	<table class="table">
		<!-- head -->
		<thead>
			<tr>
				<th class="w-14 text-center"></th>
				<th>Name</th>
			</tr>
		</thead>
		<tbody>
			{#if workflows.length > 0}
				{#each workflows as workflow, index (workflow.uri)}
					<tr
						class="hover:bg-base-200 cursor-pointer"
						onclick={() => handleSelectWorkflow(workflow.name)}
					>
						<td class="w-14 text-center">{index + 1}</td>
						<td>{workflow.name}</td>
					</tr>
				{/each}
			{:else}
				<tr>
					<td colspan="3" class="text-base-content/50 text-center text-sm font-light italic">
						<span>No workflows found.</span>
					</td>
				</tr>
			{/if}
		</tbody>
	</table>

	<div class="divider my-0"></div>

	<h3 class="text-lg font-semibold">All Runs</h3>

	<label class="input w-full">
		<Search class="size-6" />
		<input type="search" required placeholder="Search runs..." />
	</label>

	<table class="mb-8 table">
		<!-- head -->
		<thead>
			<tr>
				<th class="w-14 text-center"></th>
				<th>Name</th>
			</tr>
		</thead>
		<tbody>
			{#each runs as run, index (run.uri)}
				<tr
					class="hover:bg-base-200 cursor-pointer"
					onclick={() => projectLayout.handleFileOpen(run.uri)}
				>
					<td class="w-14 text-center">{index + 1}</td>
					<td>{run.displayLabel}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
