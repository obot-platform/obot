<script lang="ts">
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext, ResourceContents, Chat } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';
	import MarkdownEditor from '$lib/components/nanobot/MarkdownEditor.svelte';
	import { PencilLine, Play, Workflow, Eye, Trash2 } from 'lucide-svelte';
	import { formatTimeAgo } from '$lib/time';
	import { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { goto } from '$lib/url';

	let { data } = $props();
	let workflowName = $derived(data.workflowName);
	let projectId = $derived(data.projectId);
	let agent = $derived(data.agent);
	let workflow = $derived(
		$nanobotChat?.resources?.length
			? $nanobotChat.resources.find((r) => r.name === workflowName)
			: undefined
	);
	let resource = $state<ResourceContents>();
	let threads = $state<Chat[]>([]);
	let loading = $state(false);
	let deletingWorkflow = $state(false);

	let workflowsContainer = $state<HTMLElement | undefined>(undefined);
	type SortOption = 'name-asc' | 'name-desc' | 'created-desc' | 'created-asc';
	let sortBy = $state<'' | SortOption>('');
	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	const sortedThreads = $derived.by(() => {
		const list = [...threads];
		const effective = sortBy || 'created-desc';
		switch (effective) {
			case 'name-asc':
				return list.sort((a, b) => (a.title ?? '').localeCompare(b.title ?? ''));
			case 'name-desc':
				return list.sort((a, b) => (b.title ?? '').localeCompare(a.title ?? ''));
			case 'created-asc':
				return list.sort((a, b) => new Date(a.created).getTime() - new Date(b.created).getTime());
			case 'created-desc':
			default:
				return list.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime());
		}
	});

	const recentRuns = $derived(
		[...threads]
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
			.slice(0, 3)
	);

	const chatApi = $derived(new ChatAPI(agent.connectURL));

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

	$effect(() => {
		if (!resource && workflow) {
			$nanobotChat?.chat
				?.readResource(workflow.uri)
				.then((result) => {
					if (result.contents?.length) {
						resource = result.contents[0];
					}
				})
				.catch((error) => {
					console.error(error);
				});

			chatApi.getThreads().then((threadData) => {
				threads = threadData.filter((t) => t.workflowURIs && t.workflowURIs.includes(workflow.uri));
			});
		}
	});

	function handleSetupWorkflowThread(message: string) {
		loading = true;
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

				loading = false;

				goto(`/nanobot/p/${projectId}?tid=${thread.id}&wid=${workflowName}`);
			}
		});
		newChat.sendMessage(message);
	}

	function handleModifyWorkflow() {
		handleSetupWorkflowThread(`I'd like to modify the workflow: ${workflowName}`);
	}

	function handleRunWorkflow() {
		handleSetupWorkflowThread(`Run the workflow: ${workflowName}`);
	}
</script>

<div class="w-full">
	<div
		class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 md:px-8"
		bind:this={workflowsContainer}
	>
		<div class="flex justify-between gap-2">
			<button
				class="btn btn-error btn-soft btn-square tooltip tooltip-right"
				data-tip="Delete workflow"
				onclick={() => (deletingWorkflow = true)}
			>
				<Trash2 class="size-4" />
			</button>
		</div>
		<button
			class="mockup-window bg-base-100 border-base-300 group border"
			aria-label="Modify workflow"
			onclick={handleModifyWorkflow}
		>
			<div
				class="border-base-300 from-base-300 dark:to-base-200 to-base-100 relative grid h-[40dvh] overflow-hidden border-t bg-radial-[at_50%_50%] p-4 pt-0"
			>
				<div class="relative [isolation:isolate] z-0 text-left">
					<MarkdownEditor value={resource?.text ?? ''} readonly />
				</div>
				<div
					class="from-base-100 dark:from-base-200 pointer-events-none absolute inset-x-0 bottom-0 z-[100] h-32 bg-gradient-to-t to-transparent"
					aria-hidden="true"
				></div>
			</div>
			<div
				class="bg-base-100/75 absolute flex h-full w-full items-center justify-center opacity-0 backdrop-blur-[2px] transition-all group-hover:opacity-100"
			>
				<div class="tooltip tooltip-open" data-tip="Modify workflow">
					<PencilLine class="size-8" />
				</div>
			</div>
		</button>
		<div class="divider"></div>

		<div class="flex items-center justify-between">
			<h2 class="text-xl font-semibold">Workflow Runs</h2>
			<button class="btn btn-sm btn-primary" onclick={handleRunWorkflow}
				>Start New Run <Play class="size-4" /></button
			>
		</div>

		<div class="mb-8">
			{#if threads.length > 3}
				<div class="list bg-base-100 rounded-box">
					<h3 class="px-4 pb-2 text-base font-semibold tracking-wide">Most recent runs</h3>

					{#each recentRuns as thread, index (thread.id)}
						<button
							class="list-row text-left"
							onclick={() => {
								goto(`/nanobot/p/${projectId}?tid=${thread.id}&pwid=${workflowName}`);
							}}
						>
							<div
								class="flex-shrink-0 text-4xl font-thin tabular-nums opacity-30"
								style={index === 0 ? 'letter-spacing: 0.155em;' : ''}
							>
								{index < 9 ? `0${index + 1}` : index + 1}
							</div>
							<div>
								<div class="rounded-box bg-base-300 flex size-10 items-center justify-center">
									<Workflow class="size-6" />
								</div>
							</div>
							<div class="list-col-grow">
								<div class="line-clamp-1 font-light">{thread.title}</div>
								<div class="text-xs font-semibold uppercase opacity-40">
									{formatTimeAgo(thread.created).relativeTime}
								</div>
							</div>
							<div class="btn btn-square btn-ghost">
								<Eye class="size-6" />
							</div>
						</button>
					{/each}
				</div>

				<div class="divider"></div>
			{/if}

			<h3 class="mt-8 px-4 text-base font-semibold tracking-wide">All Runs</h3>

			<table class="table w-full">
				<thead>
					<tr>
						<th>Title</th>
						<th>Created</th>
						<th class="flex justify-end">
							<select class="select w-42" bind:value={sortBy}>
								<option value="" disabled selected>Sort by</option>
								<option value="created-desc">Sort by Created (Newest)</option>
								<option value="created-asc">Sort by Created (Oldest)</option>
								<option value="name-asc">Sort by Name (A-Z)</option>
								<option value="name-desc">Sort by Name (Z-A)</option>
							</select>
						</th>
					</tr>
				</thead>
				<tbody>
					{#if threads.length > 0}
						{#each sortedThreads as thread (thread.id)}
							<tr
								class="list-row"
								onclick={() => {
									goto(`/nanobot/p/${projectId}?tid=${thread.id}&pwid=${workflowName}`);
								}}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										goto(`/nanobot/p/${projectId}?tid=${thread.id}&pwid=${workflowName}`);
									}
								}}
								aria-label={`View thread ${thread.title}`}
								tabindex="0"
								role="button"
							>
								<td><span class="line-clamp-2">{thread.title}</span></td>
								<td class="whitespace-nowrap">{formatTimeAgo(thread.created).relativeTime}</td>
								<td class="flex justify-end">
									<button class="btn btn-square btn-ghost">
										<Eye class="size-6" />
									</button>
								</td>
							</tr>
						{/each}
					{:else}
						<tr>
							<td
								colspan="3"
								class="text-base-content/50 py-8 text-center text-sm font-light italic"
							>
								No runs found.
							</td>
						</tr>
					{/if}
				</tbody>
			</table>
		</div>
	</div>
</div>

{#if loading}
	<div class="fixed top-0 left-0 z-50 flex h-full w-full items-center justify-center bg-black/50">
		<div class="loading loading-bars loading-lg"></div>
	</div>
{/if}

<Confirm
	msg={`Delete ${workflowName || 'this workflow'}?`}
	show={deletingWorkflow}
	onsuccess={async () => {
		if (!workflow) return;
		await chatApi.deleteWorkflow(workflow.uri);
		nanobotChat.update((data) => {
			if (data) {
				data.resources = data.resources.filter((r) => r.uri !== workflow.uri);
			}
			return data;
		});
		goto(`/nanobot/p/${projectId}/workflows`, { replaceState: true });
	}}
	oncancel={() => (deletingWorkflow = false)}
/>

<style lang="postcss">
	:global(.mockup-window .milkdown) {
		background-color: transparent;
		position: relative;
		z-index: 0;
	}

	:global {
		button.list-row,
		tr.list-row {
			cursor: pointer;
			&:hover {
				background-color: color-mix(in oklch, var(--color-base-100) 95%, var(--color-black));
				transition: background-color 0.2s ease;
			}

			.dark &:hover {
				background-color: color-mix(in oklch, var(--color-base-100) 80%, var(--color-white));
			}
		}
	}
</style>
