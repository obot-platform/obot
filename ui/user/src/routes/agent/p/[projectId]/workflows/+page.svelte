<script lang="ts">
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import { afterNavigate } from '$app/navigation';
	import { CircleArrowUp, FolderInput, Play, Search, Workflow } from 'lucide-svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { errors, profile } from '$lib/stores/index.js';
	import type { PublishedArtifact } from '$lib/services/nanobot/types';
	import { formatTimeAgo } from '$lib/time.js';
	import { NanobotService } from '$lib/services/index.js';
	import { SvelteMap } from 'svelte/reactivity';
	import { fly } from 'svelte/transition';
	import PublishedWorkflowDropdown from '$lib/components/nanobot/PublishedWorkflowDropdown.svelte';
	import PublishedWorkflowInstallModal from '$lib/components/nanobot/PublishedWorkflowInstallModal.svelte';

	let { data } = $props();
	let projectId = $derived(data.projects[0].id);
	let publishedWorkflows = $derived(data.publishedWorkflows);

	let workflowQuery = $state('');
	let loading = $state(false);
	let sortBy = $state<'' | 'name-asc' | 'name-desc' | 'created-asc' | 'created-desc'>('');
	let activeTab = $state<'my' | 'shared'>('my');
	let publishing = new SvelteMap<string, boolean>();

	let installing = new SvelteMap<string, boolean>();
	let installingPublishedArtifact = $state<PublishedArtifact | undefined>(undefined);
	let installType = $state<'new' | 'update'>();

	let workflows = $derived(
		$nanobotChat?.resources
			? $nanobotChat.resources.filter((r) => r.uri.startsWith('workflow:///'))
			: []
	);

	let workflowSet = $derived(new Set(workflows.map((w) => w.name)));
	let { sharedWorkflows, myPublishedWorkflows } = $derived({
		sharedWorkflows: publishedWorkflows
			.filter((w) => w.visibility === 'public' && w.authorID !== profile.current.id)
			.map((w) => ({ ...w, isInstalled: workflowSet.has(w.name) }))
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime()),
		myPublishedWorkflows: publishedWorkflows
			.filter((w) => w.authorID === profile.current.id)
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
	});
	let recentlySharedToMe = $derived(sharedWorkflows.slice(0, 3));
	let showingSearchResults = $derived(workflowQuery.trim().length > 0);

	let tableData = $derived.by(() => {
		const myPublishedMap = new Map<string, PublishedArtifact>(
			myPublishedWorkflows.map((w) => [w.name, w])
		);
		return [
			...(activeTab === 'shared'
				? sharedWorkflows.map((w) => ({
						id: w.id,
						workflowId: w.name,
						publishedArtifactId: w.id,
						name: w.displayName,
						published: w.created,
						visibility: w.visibility,
						createdBy: w.authorEmail,
						workflowUri: undefined,
						isInstalled: w.isInstalled
					}))
				: []),
			...(activeTab === 'my'
				? workflows.map((w) => {
						const publishedMatch = myPublishedMap.get(w.name);
						return {
							id: w.name,
							workflowId: w.name,
							publishedArtifactId: publishedMatch?.id,
							name:
								publishedMatch?.displayName ??
								((w._meta?.displayName ?? w._meta?.name ?? w.name ?? '') as string),
							published: publishedMatch?.created,
							visibility: publishedMatch?.visibility || 'private',
							createdBy: 'Me',
							workflowUri: w.uri,
							isInstalled: true
						};
					})
				: [])
		];
	});

	let filteredWorkflows = $derived(
		tableData
			.filter((w) => w.name?.toLowerCase().includes(workflowQuery.toLowerCase()))
			.sort((a, b) => {
				if (sortBy === 'created-desc') {
					if (a.published == null || b.published == null || a.published === b.published) return 0;
					if (a.published && !b.published) return -1;
					if (!a.published && b.published) return 1;
					return new Date(b.published).getTime() - new Date(a.published).getTime();
				} else if (sortBy === 'created-asc') {
					if (a.published == null || b.published == null || a.published === b.published) return 0;
					if (a.published && !b.published) return 1;
					if (!a.published && b.published) return -1;
					return new Date(a.published).getTime() - new Date(b.published).getTime();
				} else if (sortBy === 'name-asc') {
					return (a.name ?? '').localeCompare(b.name ?? '');
				} else if (sortBy === 'name-desc') {
					return (b.name ?? '').localeCompare(a.name ?? '');
				}
				return 0;
			})
	);

	let workflowsContainer = $state<HTMLElement | undefined>(undefined);

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	function handleSelectWorkflow(workflowName: string) {
		$nanobotChat?.api.createSession().then((sessionClient) => {
			nanobotChat.update((data) => {
				if (data) {
					if (data.chat) {
						data.chat.close();
					}
					data.chat = sessionClient;
					data.sessionId = sessionClient.chatId;
				}
				return data;
			});

			goto(
				`/agent/p/${projectId}?tid=${sessionClient.chatId}&wid=${encodeURIComponent(workflowName)}`,
				{
					replaceState: true,
					noScroll: true,
					keepFocus: true
				}
			);
			sessionClient.sendMessage(`Run workflow: ${workflowName}`);
		});
	}

	function handlePublishWorkflow(workflowId: string, workflowDisplayName: string) {
		publishing.set(workflowId, true);
		$nanobotChat?.api
			.publishArtifact(workflowId)
			.then((response) => {
				myPublishedWorkflows = [
					...myPublishedWorkflows,
					{
						// optimistic
						id: response.id,
						created: new Date().toISOString(),
						metadata: {},
						name: response.name,
						displayName: workflowDisplayName,
						description: '',
						authorID: profile.current.id,
						authorEmail: profile.current.email,
						latestVersion: response.version,
						visibility: 'private'
					}
				];
			})
			.finally(() => {
				publishing.delete(workflowId);
			});
	}

	function handleInstallWorkflow(publishedArtifact: PublishedArtifact) {
		installing.set(publishedArtifact.id, true);
		installingPublishedArtifact = publishedArtifact;
		installType = workflowSet.has(publishedArtifact.name) ? 'update' : 'new';
	}

	function refresh() {
		$nanobotChat?.api
			.listResources()
			.then((resources) => {
				nanobotChat.update((data) => {
					if (data) {
						data.resources = resources;
					}
					return data;
				});
			})
			.finally(() => {
				loading = false;
			});
	}

	const MAX_WORKFLOW_POLL_RETRIES = 5;
	function pollAndNavigateToWorkflow(retriesLeft = MAX_WORKFLOW_POLL_RETRIES) {
		if (!installingPublishedArtifact) return;
		const workflowName = installingPublishedArtifact.name;
		$nanobotChat?.api.listResources().then((resources) => {
			const match = resources.find((r) => r.name === workflowName);
			nanobotChat.update((data) => {
				if (data) {
					data.resources = resources;
				}
				return data;
			});
			if (match && installingPublishedArtifact) {
				installing.delete(installingPublishedArtifact.id);
				installingPublishedArtifact = undefined;
				goto(`/agent/p/${projectId}/workflows/${encodeURIComponent(workflowName)}`);
			} else if (retriesLeft > 0) {
				setTimeout(() => {
					pollAndNavigateToWorkflow(retriesLeft - 1);
				}, 1000);
			} else {
				errors.append('Error: Could not find workflow after installation');
			}
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

	afterNavigate(({ from }) => {
		if (!from?.url || !$nanobotChat?.api) return;
		loading = true;
		refresh();
	});
</script>

<div
	class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 md:px-8"
	bind:this={workflowsContainer}
>
	<div>
		<div class="flex items-center gap-1">
			<h2 class="text-2xl font-semibold">Workflows</h2>
			{#if loading}
				<div class="loading loading-spinner text-primary loading-sm ml-2"></div>
			{/if}
		</div>

		<p class="text-base-content/50 text-sm font-light">
			Workflows are AI-powered tools that can be used to automate tasks and processes.
		</p>
	</div>

	{#if recentlySharedToMe.length > 0}
		<div class="list bg-base-100 rounded-box" out:fly={{ x: 100, duration: 150 }}>
			<h3 class="px-4 pb-2 text-base font-semibold tracking-wide">Recently shared with me</h3>
			{#each recentlySharedToMe as workflow (workflow.id)}
				<div
					class="list-row text-left"
					out:fly={{ x: 100, duration: 150 }}
					role="presentation"
					onclick={(e) => {
						const row = e.currentTarget as HTMLElement;
						row.querySelector<HTMLButtonElement>('.dropdown button')?.click();
					}}
				>
					<div>
						<div class="rounded-box bg-base-300 flex size-10 items-center justify-center">
							<Workflow class="size-6" />
						</div>
					</div>
					<div class="list-col-grow">
						<div class="line-clamp-1 font-light">
							{workflow.displayName}
						</div>
						<div class="text-xs opacity-40">
							<span class="font-semibold uppercase">
								{formatTimeAgo(workflow.created).relativeTime}
							</span>
							<span class="font-light">by {workflow.authorEmail}</span>
						</div>
					</div>
					{#if installing.get(workflow.id)}
						<div class="loading loading-spinner text-primary loading-sm mr-3"></div>
					{:else}
						<button
							class="btn btn-ghost btn-square tooltip tooltip-left"
							data-tip={workflow.isInstalled ? 'Update workflow' : 'Install workflow'}
							onclick={(e) => {
								e.preventDefault();
								e.stopPropagation();
								if (!workflow.id) return;
								handleInstallWorkflow(workflow);
							}}
						>
							{#if workflow.isInstalled}
								<CircleArrowUp class="size-5" />
							{:else}
								<FolderInput class="size-5" />
							{/if}
						</button>
					{/if}
				</div>
			{/each}
		</div>

		<div class="divider my-0"></div>
	{/if}
	<div class="flex flex-col gap-1">
		<div role="tablist" class="tabs tabs-box">
			<button
				role="tab"
				class="tab {activeTab === 'my' ? 'tab-active' : ''}"
				onclick={() => {
					activeTab = 'my';
					workflowQuery = '';
				}}
			>
				My Workflows
			</button>
			<button
				role="tab"
				class="tab {activeTab === 'shared' ? 'tab-active' : ''}"
				onclick={() => {
					activeTab = 'shared';
					workflowQuery = '';
				}}
			>
				Shared With Me
			</button>
		</div>

		<label class="input mt-1 w-full">
			<Search class="size-6" />
			<input
				type="search"
				required
				placeholder={activeTab === 'my' ? 'Search my workflows...' : 'Search shared workflows...'}
				bind:value={workflowQuery}
			/>
		</label>
	</div>

	<table class="mb-8 table">
		<!-- head -->
		<thead>
			<tr>
				<th>Name</th>
				{#if activeTab === 'my' || showingSearchResults}
					<th>Published</th>
					<th>Visibility</th>
				{/if}
				{#if activeTab === 'shared' || showingSearchResults}
					<th>Owner</th>
				{/if}
				<th class="flex justify-end">
					<select class="select w-42" bind:value={sortBy}>
						<option value="" disabled>Sort by</option>
						<option value="created-desc">Sort by Created (Newest)</option>
						<option value="created-asc">Sort by Created (Oldest)</option>
						<option value="name-asc">Sort by Name (A-Z)</option>
						<option value="name-desc">Sort by Name (Z-A)</option>
					</select>
				</th>
			</tr>
		</thead>
		<tbody>
			{#if filteredWorkflows.length > 0}
				{#each filteredWorkflows as workflow (workflow.id)}
					<tr
						class="hover:bg-base-200 cursor-pointer"
						role="button"
						tabindex="0"
						onclick={workflow.createdBy === 'Me'
							? () => {
									goto(
										`/agent/p/${projectId}/workflows/${encodeURIComponent(workflow.workflowId)}`
									);
								}
							: undefined}
						onkeydown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault();
								if (workflow.createdBy === 'Me') {
									goto(
										`/agent/p/${projectId}/workflows/${encodeURIComponent(workflow.workflowId)}`
									);
								}
							}
						}}
					>
						<td>{workflow.name}</td>
						{#if activeTab === 'my' || showingSearchResults}
							<td class="capitalize">
								{#if publishing.get(workflow.workflowId)}
									<div class="loading loading-spinner text-primary loading-sm ml-3"></div>
								{:else if workflow.published}
									{formatTimeAgo(workflow.published).relativeTime}
								{:else if workflow.createdBy === 'Me'}
									<button
										class="btn btn-link btn-sm font-light"
										onclick={(e) => {
											e.preventDefault();
											e.stopPropagation();
											handlePublishWorkflow(workflow.workflowId, workflow.name);
										}}>Publish</button
									>
								{/if}
							</td>
							<td>
								{#if workflow.published && workflow.createdBy === 'Me'}
									<select
										class="select w-36"
										value={workflow.visibility}
										onclick={(e) => {
											e.preventDefault();
											e.stopPropagation();
										}}
										onchange={(e) => {
											e.preventDefault();
											e.stopPropagation();
											if (!workflow.publishedArtifactId) return;
											const visibility = e.currentTarget.value as 'public' | 'private';
											NanobotService.updatePublishedArtifact(workflow.publishedArtifactId, {
												visibility
											}).then(() => {
												myPublishedWorkflows = myPublishedWorkflows.map((w) =>
													w.id === workflow.publishedArtifactId ? { ...w, visibility } : w
												);
											});
											e.currentTarget.blur();
										}}
									>
										<option value="public">Public</option>
										<option value="private">Private</option>
									</select>
								{:else}
									<span> - </span>
								{/if}
							</td>
						{/if}
						{#if activeTab === 'shared' || showingSearchResults}
							<td>{workflow.createdBy}</td>
						{/if}
						<td class="text-right">
							{#if workflow.createdBy === 'Me'}
								<PublishedWorkflowDropdown
									publishedArtifactId={workflow.publishedArtifactId}
									workflowUri={workflow.workflowUri}
									onUnpublish={() => {
										myPublishedWorkflows = myPublishedWorkflows.filter(
											(w) => w.id !== workflow.publishedArtifactId
										);
									}}
									onPublish={() => {
										handlePublishWorkflow(workflow.workflowId, workflow.name);
									}}
									onCheckForUpdates={(id) => {
										installingPublishedArtifact = myPublishedWorkflows.find((w) => w.id === id);
										installType = 'update';
									}}
								/>
								<button
									class="btn btn-ghost btn-square tooltip tooltip-top flex-shrink-0"
									data-tip="Run this workflow"
									onclick={(e) => {
										e.preventDefault();
										e.stopPropagation();
										handleSelectWorkflow(workflow.workflowId);
									}}
								>
									<Play class="size-4" />
								</button>
							{:else if workflow.publishedArtifactId}
								<button
									class="btn btn-ghost btn-square tooltip tooltip-left"
									data-tip={workflow.isInstalled ? 'Update workflow' : 'Install workflow'}
									onclick={(e) => {
										e.preventDefault();
										e.stopPropagation();
										if (!workflow.publishedArtifactId) return;
										const match = sharedWorkflows.find(
											(w) => w.id === workflow.publishedArtifactId
										);
										if (!match) {
											errors.append('Error: Could not find related shared workflow');
											return;
										}
										handleInstallWorkflow(match);
									}}
								>
									{#if workflow.isInstalled}
										<CircleArrowUp class="size-4" />
									{:else}
										<FolderInput class="size-4" />
									{/if}
								</button>
							{/if}
						</td>
					</tr>
				{/each}
			{:else}
				<tr>
					<td
						colspan={activeTab === 'my' || showingSearchResults ? 5 : 3}
						class="text-base-content/50 text-center text-sm font-light italic"
					>
						<span>No workflows found.</span>
					</td>
				</tr>
			{/if}
		</tbody>
	</table>
</div>

{#if installingPublishedArtifact}
	<PublishedWorkflowInstallModal
		publishedArtifact={installingPublishedArtifact}
		onClose={() => {
			if (installingPublishedArtifact) {
				installing.delete(installingPublishedArtifact.id);
			}
			installingPublishedArtifact = undefined;
		}}
		onSuccess={() => {
			pollAndNavigateToWorkflow();
		}}
		title={installType === 'new' ? 'Install Workflow' : 'Update Workflow'}
		confirmButtonText={installType === 'new' ? 'Install' : 'Update'}
		message={installType === 'update'
			? 'Are you sure you want to update? Any existing changes will be overwritten.'
			: undefined}
	>
		{#snippet loadingText()}
			{#if installType === 'update'}
				Updating <i>{installingPublishedArtifact?.displayName}...</i>
			{:else}
				Installing <i>{installingPublishedArtifact?.displayName}...</i>
			{/if}
		{/snippet}
	</PublishedWorkflowInstallModal>
{/if}

<svelte:head>
	<title>Obot | Workflows</title>
</svelte:head>
