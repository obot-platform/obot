<script lang="ts">
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import { afterNavigate } from '$app/navigation';
	import { FolderInput, Play, Search, Trash2, Workflow } from 'lucide-svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { errors, profile } from '$lib/stores/index.js';
	import type { PublishedArtifact } from '$lib/services/nanobot/types';
	import { formatTimeAgo } from '$lib/time.js';
	import { SvelteMap } from 'svelte/reactivity';
	import { fly } from 'svelte/transition';
	import PublishedWorkflowDropdown from '$lib/components/nanobot/PublishedWorkflowDropdown.svelte';
	import PublishedWorkflowInstallModal from '$lib/components/nanobot/PublishedWorkflowInstallModal.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import PublishedWorkflowVersionDialog from '$lib/components/nanobot/PublishedWorkflowVersionDialog.svelte';
	import { NanobotService } from '$lib/services';
	import { untrack } from 'svelte';
	import ConfirmDiffWorkflow from '$lib/components/nanobot/ConfirmDiffWorkflow.svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let projectId = $derived(data.projects[0].id);
	let publishedWorkflows = $state<PublishedArtifact[]>(untrack(() => data.publishedWorkflows));

	let workflowQuery = $state('');
	let loading = $state(false);
	let sortBy = $state<'' | 'name-asc' | 'name-desc' | 'created-asc' | 'created-desc'>('');
	let activeTab = $state<'my' | 'shared'>('my');
	let publishing = new SvelteMap<string, boolean>();
	let showConfirmPublishWorkflow = $state<
		| {
				latestVersion: number;
				workflowUri: string;
				workflowId: string;
				workflowDisplayName: string;
				publishedArtifactId: string;
		  }
		| undefined
	>(undefined);
	let showConfirmUpdateWorkflow = $state<
		| {
				latestVersion: number;
				workflowUri: string;
				publishedArtifact: PublishedArtifact;
		  }
		| undefined
	>(undefined);

	let installing = new SvelteMap<string, boolean>();
	let installingPublishedArtifact = $state<PublishedArtifact | undefined>(undefined);
	let installType = $state<'new' | 'update'>();
	let confirmDeleteWorkflow = $state<
		| {
				id: string;
				displayName: string;
				uri: string;
		  }
		| undefined
	>();
	let deleting = $state(false);

	let workflows = $derived(
		$nanobotChat?.resources
			? $nanobotChat.resources.filter((r) => r.uri.startsWith('workflow:///'))
			: []
	);

	let workflowSet = $derived(new Map(workflows.map((w) => [w.name, w])));
	let { sharedWorkflows, myPublishedWorkflows } = $derived({
		sharedWorkflows: publishedWorkflows
			.filter((w) => w.visibility === 'public' && w.authorID !== profile.current.id)
			.map((w) => {
				const latestVersion =
					w.versions && w.versions.length > 0
						? w.versions[w.versions.length - 1]?.version
						: undefined;
				const latestVersionString = latestVersion != null ? String(latestVersion) : undefined;
				return {
					...w,
					isInstalled: workflowSet.has(w.name),
					isUpdated: workflowSet.get(w.name)?._meta?.version === latestVersionString,
					workflowUri: workflowSet.get(w.name)?.uri ?? ''
				};
			})
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
		return activeTab === 'shared'
			? sharedWorkflows.map((w) => ({
					id: w.id,
					workflowId: w.name,
					publishedArtifactId: w.id,
					name: w.displayName,
					published: w.created,
					visibility: w.visibility,
					createdBy: w.authorEmail,
					workflowUri: w.workflowUri ?? '',
					isInstalled: w.isInstalled,
					isInstalledFrom: undefined,
					isUpdated: w.isUpdated,
					versions: w.versions ?? []
				}))
			: workflows.map((w) => {
					const publishedMatch = myPublishedMap.get(w.name);
					const sharedMatch = sharedWorkflows.find(
						(sw) => sw.name === w.name && sw.authorEmail === w._meta?.['author-email']
					);
					const sharedLatestVersion =
						sharedMatch?.versions && sharedMatch.versions.length > 0
							? sharedMatch.versions[sharedMatch.versions.length - 1]?.version
							: undefined;
					const sharedLatestVersionString =
						sharedLatestVersion != null ? String(sharedLatestVersion) : undefined;
					const isInstalledFrom = sharedMatch && w._meta?.['author-email'];
					const isInstalled = !!(
						isInstalledFrom && w._meta?.['author-email'] !== profile.current.email
					);
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
						isInstalled,
						isInstalledFrom,
						isUpdated: sharedLatestVersionString === w._meta?.version,
						versions: publishedMatch?.versions ?? []
					};
				});
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
	let showWorkflowVersionDialog = $state<(typeof tableData)[number] | undefined>(undefined);

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

	function handlePublishWorkflow(workflowId: string) {
		publishing.set(workflowId, true);
		$nanobotChat?.api
			.publishArtifact(workflowId)
			.then(() => {
				NanobotService.listPublishedWorkflows().then((workflows) => {
					publishedWorkflows = workflows;
				});
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
		const clearInstalling = () => {
			if (installingPublishedArtifact?.id) {
				installing.delete(installingPublishedArtifact.id);
				installingPublishedArtifact = undefined;
			}
		};
		$nanobotChat?.api
			.listResources()
			.then((resources) => {
				const match = resources.find((r) => r.name === workflowName);
				nanobotChat.update((data) => {
					if (data) {
						data.resources = resources;
					}
					return data;
				});
				if (match && installingPublishedArtifact) {
					clearInstalling();
					goto(`/agent/p/${projectId}/workflows/${encodeURIComponent(workflowName)}`);
				} else if (retriesLeft > 0) {
					setTimeout(() => {
						pollAndNavigateToWorkflow(retriesLeft - 1);
					}, 1000);
				} else {
					clearInstalling();
					errors.append('Error: Could not find workflow after installation');
				}
			})
			.catch((error) => {
				clearInstalling();
				errors.append(error);
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
							class={twMerge(
								'btn btn-ghost btn-square',
								workflow.isInstalled
									? workflow.isUpdated
										? 'btn-disabled btn-ghost'
										: 'tooltip tooltip-left btn-warning btn-soft'
									: 'btn-ghost tooltip tooltip-left'
							)}
							data-tip={workflow.isInstalled ? 'An update is available' : 'Install workflow'}
							onclick={(e) => {
								e.preventDefault();
								e.stopPropagation();
								if (!workflow.id) return;
								if (workflow.isInstalled) {
									showConfirmUpdateWorkflow = {
										latestVersion: workflow.versions?.[workflow.versions.length - 1]?.version ?? 0,
										workflowUri: workflow.workflowUri ?? '',
										publishedArtifact: workflow
									};
								} else {
									handleInstallWorkflow(workflow);
								}
							}}
							disabled={workflow.isInstalled && workflow.isUpdated}
						>
							<FolderInput class="size-5" />
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
					<th>Last Published</th>
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
									-
								{/if}
							</td>
						{/if}
						{#if activeTab === 'shared' || showingSearchResults}
							<td>{workflow.createdBy}</td>
						{/if}
						<td class="text-right">
							{#if workflow.createdBy === 'Me'}
								{#if workflow.isInstalled}
									<button
										class={twMerge(
											'btn btn-square tooltip tooltip-left',
											workflow.isInstalled && !workflow.isUpdated
												? 'btn-warning btn-soft'
												: 'btn-ghost'
										)}
										data-tip="An update is available"
										onclick={(e) => {
											e.preventDefault();
											e.stopPropagation();

											const match = sharedWorkflows.find(
												(sw) =>
													sw.workflowUri === workflow.workflowUri &&
													sw.authorEmail === workflow.isInstalledFrom
											);

											if (!match) {
												errors.append('Error: Could not find related shared workflow');
												return;
											}
											showConfirmUpdateWorkflow = {
												latestVersion: match.versions?.[match.versions.length - 1]?.version ?? 0,
												workflowUri: workflow.workflowUri ?? '',
												publishedArtifact: match
											};
										}}
										disabled={workflow.isInstalled && workflow.isUpdated}
									>
										<FolderInput class="size-4" />
									</button>
								{/if}
								<button
									class="btn btn-ghost hover:btn-error btn-square tooltip tooltip-top flex-shrink-0"
									data-tip="Delete workflow"
									onclick={(e) => {
										e.preventDefault();
										e.stopPropagation();
										if (!workflow.workflowUri) {
											errors.append('Delete failed: Workflow uri not found');
											return;
										}
										confirmDeleteWorkflow = {
											id: workflow.workflowId,
											displayName: workflow.name,
											uri: workflow.workflowUri
										};
									}}
								>
									<Trash2 class="size-4" />
								</button>
								<button
									class="btn btn-ghost hover:btn-primary btn-square tooltip tooltip-top flex-shrink-0"
									data-tip="Run this workflow"
									onclick={(e) => {
										e.preventDefault();
										e.stopPropagation();
										handleSelectWorkflow(workflow.workflowId);
									}}
								>
									<Play class="size-4" />
								</button>
							{:else}
								<button
									class={twMerge(
										'btn btn-ghost btn-square',
										workflow.isInstalled
											? workflow.isUpdated
												? 'btn-disabled btn-ghost'
												: 'tooltip tooltip-left btn-warning btn-soft'
											: 'btn-ghost tooltip tooltip-left'
									)}
									data-tip={workflow.isInstalled ? 'An update is available' : 'Install workflow'}
									onclick={(e) => {
										e.preventDefault();
										e.stopPropagation();
										if (!workflow.id) return;
										const match = sharedWorkflows.find(
											(w) => w.id === workflow.publishedArtifactId
										);
										if (!match) {
											errors.append('Error: Could not find related shared workflow');
											return;
										}
										if (workflow.isInstalled) {
											showConfirmUpdateWorkflow = {
												latestVersion:
													workflow.versions?.[workflow.versions.length - 1]?.version ?? 0,
												workflowUri: workflow.workflowUri ?? '',
												publishedArtifact: match
											};
										} else {
											handleInstallWorkflow(match);
										}
									}}
									disabled={workflow.isInstalled && workflow.isUpdated}
								>
									<FolderInput class="size-5" />
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
				Updating <i
					>{installingPublishedArtifact?.displayName || installingPublishedArtifact?.name}...</i
				>
			{:else}
				Installing <i
					>{installingPublishedArtifact?.displayName || installingPublishedArtifact?.name}...</i
				>
			{/if}
		{/snippet}
	</PublishedWorkflowInstallModal>
{/if}

<Confirm
	msg={`Delete ${confirmDeleteWorkflow?.displayName || 'this workflow'}?`}
	show={confirmDeleteWorkflow !== undefined}
	loading={deleting}
	onsuccess={async () => {
		if (!confirmDeleteWorkflow) return;
		deleting = true;
		try {
			await $nanobotChat?.api.deleteWorkflow(confirmDeleteWorkflow.uri);
			nanobotChat.update((data) => {
				if (data) {
					data.resources = data.resources.filter((r) => r.uri !== confirmDeleteWorkflow?.uri);
				}
				return data;
			});
			confirmDeleteWorkflow = undefined;
		} catch (err) {
			errors.append(`Failed to delete workflow: ${err}`);
		} finally {
			deleting = false;
		}
	}}
	oncancel={() => (confirmDeleteWorkflow = undefined)}
/>

{#if showWorkflowVersionDialog}
	<PublishedWorkflowVersionDialog
		publishedArtifactId={showWorkflowVersionDialog.publishedArtifactId}
		versions={showWorkflowVersionDialog.versions}
		workflowDisplayName={showWorkflowVersionDialog.name}
		onClose={() => (showWorkflowVersionDialog = undefined)}
	/>
{/if}

{#if showConfirmUpdateWorkflow}
	<ConfirmDiffWorkflow
		latestVersion={showConfirmUpdateWorkflow.latestVersion}
		workflowUri={showConfirmUpdateWorkflow.workflowUri}
		publishedArtifactId={showConfirmUpdateWorkflow.publishedArtifact.id}
		onSubmit={() => {
			if (!showConfirmUpdateWorkflow) return;
			handleInstallWorkflow(showConfirmUpdateWorkflow.publishedArtifact);
			showConfirmUpdateWorkflow = undefined;
		}}
		variant="update"
		onCancel={() => {
			showConfirmUpdateWorkflow = undefined;
		}}
	/>
{/if}

{#if showConfirmPublishWorkflow}
	<ConfirmDiffWorkflow
		latestVersion={showConfirmPublishWorkflow.latestVersion}
		workflowUri={showConfirmPublishWorkflow.workflowUri}
		publishedArtifactId={showConfirmPublishWorkflow.publishedArtifactId}
		onSubmit={() => {
			if (!showConfirmPublishWorkflow) return;
			handlePublishWorkflow(showConfirmPublishWorkflow.workflowId);
			showConfirmPublishWorkflow = undefined;
		}}
		onCancel={() => {
			showConfirmPublishWorkflow = undefined;
		}}
	/>
{/if}

<svelte:head>
	<title>Obot | Workflows</title>
</svelte:head>
