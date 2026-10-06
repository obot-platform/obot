<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ConfirmDiffWorkflow from '$lib/components/nanobot/ConfirmDiffWorkflow.svelte';
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	import MarkdownEditor from '$lib/components/nanobot/MarkdownEditor.svelte';
	import PublishedWorkflowInstallModal from '$lib/components/nanobot/PublishedWorkflowInstallModal.svelte';
	import PublishedWorkflowVersionDialog from '$lib/components/nanobot/PublishedWorkflowVersionDialog.svelte';
	import { latestVersionSubjects } from '$lib/components/nanobot/publishedArtifactSubjects';
	import { formatFileSize, formatFileTime } from '$lib/format';
	import { m } from '$lib/i18n';
	import { reloadPage } from '$lib/navigation';
	import { NanobotService } from '$lib/services';
	import type {
		ProjectLayoutContext,
		ResourceContents,
		Chat,
		PublishedArtifact
	} from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { hasNewerVersion } from '$lib/services/nanobot/versioning';
	import { profile, responsive, userDeviceSettings } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { PencilLine, Play, Workflow, Eye, FolderInput, Trash2 } from '@lucide/svelte';
	import { getContext, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let workflowId = $derived(data.workflowId);
	let projectId = $derived(data.projectId);
	let publishedWorkflows = $state<PublishedArtifact[]>(untrack(() => data.publishedWorkflows));
	let publishedInfo = $derived(
		publishedWorkflows.find((w) => w.name === workflowId && w.authorID === profile.current.id)
	);
	let publishedVersionSubjects = $derived(
		latestVersionSubjects(publishedInfo?.versions, publishedInfo?.latestVersion)
	);
	let latestVersion = $derived(publishedInfo?.latestVersion ?? 0);

	let workflow = $derived(
		$nanobotChat?.resources?.length
			? $nanobotChat.resources.find((r) => r.name === workflowId)
			: undefined
	);
	let workflowDisplayName = $derived(workflow?._meta?.displayName ?? workflow?._meta?.name);
	let workflowResources = $derived(
		$nanobotChat?.resources?.filter((r) => r.uri.startsWith(`file:///workflows/${workflowId}/`)) ??
			[]
	);

	let resource = $state<ResourceContents>();
	let sessions = $state<Chat[]>([]);
	let loading = $state(false);
	let deletingWorkflow = $state(false);
	let publishing = $state(false);
	let confirmInstallModal = $state<{ selectedVersion?: number } | undefined>(undefined);

	let showConfirmPublishWorkflow = $state(false);
	let showWorkflowVersionDialog = $state(false);
	let showConfirmUpdateWorkflow = $state(false);
	let showPublishSuccess = $state(false);
	let confirmUnpublish = $state(false);

	let relatedPublishedArtifact = $derived(
		publishedWorkflows.find(
			(w) =>
				w.authorEmail === (workflow?._meta?.['author-email'] as string) && w.name === workflow?.name
		)
	);

	let hasPublishUpdate = $derived.by(() => {
		if (!workflow) return false;
		const versionInstalled = workflow._meta?.version as string;
		if (!relatedPublishedArtifact || !versionInstalled) return false;
		const versions = relatedPublishedArtifact.versions;

		const latestVersion =
			relatedPublishedArtifact.latestVersion ??
			(versions && versions.length > 0
				? versions[versions.length - 1]?.version?.toString()
				: undefined);
		return hasNewerVersion(latestVersion, versionInstalled);
	});

	let workflowsContainer = $state<HTMLElement | undefined>(undefined);
	type SortOption = 'name-asc' | 'name-desc' | 'created-desc' | 'created-asc';
	let sortBy = $state<'' | SortOption>('');
	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	const sortedThreads = $derived.by(() => {
		const list = [...sessions];
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
		[...sessions]
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
			.slice(0, 3)
	);

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
		if (!resource && workflow && $nanobotChat?.api) {
			$nanobotChat.api
				.readResource(workflow.uri)
				.then((result) => {
					if (result.contents?.length) {
						resource = result.contents[0];
					}
				})
				.catch((error) => {
					console.error(error);
				});

			$nanobotChat.api.listSessions().then((sessionData) => {
				sessions = sessionData.filter(
					(t) => t.workflowURIs && t.workflowURIs.includes(workflow.uri)
				);
			});
		}
	});

	$effect(() => {
		if (workflowDisplayName) {
			projectLayout.setLayoutName(workflowDisplayName as string);
			projectLayout.setShowBackButton(true);
		}
	});

	function handleSetupWorkflowThread(message: string, showFile: boolean = false) {
		loading = true;
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
			sessionClient.sendMessage(message);

			loading = false;

			goto(
				`/agent/p/${projectId}?tid=${sessionClient.chatId}${showFile ? `&wid=${workflowId}` : ''}`
			);
		});
	}

	function handleModifyWorkflow() {
		handleSetupWorkflowThread(`I'd like to modify the workflow: ${workflowId}`, true);
	}

	function handleRunWorkflow() {
		handleSetupWorkflowThread(`Run the workflow: ${workflowId}`);
	}

	function handlePublishWorkflow() {
		publishing = true;
		$nanobotChat?.api
			.publishArtifact(workflowId)
			.then(async () => {
				publishedWorkflows = await NanobotService.listPublishedWorkflows();
				showPublishSuccess = true;
			})
			.finally(() => {
				publishing = false;
			});
	}

	async function handleUnpublish() {
		if (!publishedInfo?.id) return;
		await NanobotService.deletePublishedArtifact(publishedInfo.id);
		publishedWorkflows = await NanobotService.listPublishedWorkflows();
		confirmUnpublish = false;
	}

	function onFileOpen(filename: string) {
		projectLayout?.handleFileOpen(filename);
	}
</script>

<div class="w-full">
	<div
		class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 md:px-8"
		bind:this={workflowsContainer}
	>
		<div class="mt-1 flex items-center justify-between gap-2">
			<div class="flex items-center gap-2">
				{#if publishing}
					<div class="skeleton skeleton-text">{m.chat_publishing()}</div>
				{:else}
					<button class="btn btn-primary" onclick={() => (showConfirmPublishWorkflow = true)}
						>{m.chat_publish()}</button
					>
				{/if}
				{#if publishedInfo}
					<button class="btn btn-link px-2" onclick={() => (showWorkflowVersionDialog = true)}>
						{m.chat_manage_published_versions()}
					</button>
				{/if}
			</div>
			<div class="flex items-center gap-2">
				{#if relatedPublishedArtifact}
					<button
						class={twMerge(
							'btn',
							hasPublishUpdate ? 'btn-warning btn-soft tooltip tooltip-left' : 'btn-ghost'
						)}
						onclick={() => (showConfirmUpdateWorkflow = true)}
						data-tip={hasPublishUpdate
							? m.chat_update_available()
							: m.chat_select_different_version()}
					>
						<FolderInput class="size-4" />
					</button>
				{/if}
				<button
					class="btn btn-ghost btn-error btn-square tooltip tooltip-left"
					data-tip={m.chat_delete_workflow()}
					onclick={() => (deletingWorkflow = true)}
					aria-label={m.chat_delete_workflow()}
				>
					<Trash2 class="size-4" />
				</button>
			</div>
		</div>
		<button
			class="mockup-window bg-base-100 border-base-300 group border"
			aria-label={m.chat_modify_workflow()}
			onclick={handleModifyWorkflow}
		>
			<div
				class="border-base-300 from-base-300 dark:to-base-200 to-base-100 relative grid h-[40dvh] overflow-hidden border-t bg-radial-[at_50%_50%] p-4 pt-0"
			>
				<div class="relative [isolation:isolate] z-0 text-left">
					<MarkdownEditor value={resource?.text ?? ''} readonly />
				</div>
				<div
					class="from-base-100 dark:from-base-200 pointer-events-none absolute inset-x-0 bottom-0 z-10 h-32 bg-gradient-to-t to-transparent"
					aria-hidden="true"
				></div>
			</div>
			<div
				class="bg-base-100/75 absolute flex h-full w-full items-center justify-center opacity-0 backdrop-blur-[2px] transition-all group-hover:opacity-100"
			>
				<div class="tooltip tooltip-open" data-tip={m.chat_modify_workflow()}>
					<PencilLine class="size-8" />
				</div>
			</div>
		</button>

		{#if workflowResources.length > 0}
			<div class="divider"></div>
			<h2 class="text-xl font-semibold">{m.chat_workflow_files()}</h2>

			<table class="table w-full table-fixed">
				<thead>
					<tr>
						<th>{m.core_name()}</th>
						<th>{m.core_col_size()}</th>
						<th>{m.chat_col_last_modified()}</th>
						<th>{m.chat_col_location()}</th>
					</tr>
				</thead>
				<tbody>
					{#each workflowResources as resource (resource.uri)}
						<tr
							onclick={() => {
								onFileOpen?.(resource.uri);
							}}
							class="hover:bg-base-200 cursor-pointer"
							role="button"
							tabindex="0"
							onkeydown={(e) => {
								if (e.key === 'Enter' || e.key === ' ') {
									e.preventDefault();
									onFileOpen?.(resource.uri);
								}
							}}
						>
							<td>
								<div class="flex items-center gap-2">
									<FileItem uri={resource.uri} classes={{ icon: 'size-4' }} compact />
									<span class="min-w-0 truncate font-normal">
										{resource.name}
									</span>
								</div>
							</td>
							<td>
								<p class="truncate text-nowrap break-all">
									{formatFileSize(resource.size ?? 0)}
								</p>
							</td>
							<td
								><p class="truncate text-nowrap break-all">
									{formatFileTime(resource.annotations?.lastModified, userDeviceSettings.timeFormat)
										.formatted || '-'}
								</p></td
							>
							<td>
								<div class="w-full min-w-0">
									<p
										class="text-muted-content w-full min-w-0 truncate text-sm font-light break-all italic"
									>
										{resource.uri}
									</p>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}

		<div class="divider"></div>

		<div class="flex items-center justify-between">
			<h2 class="text-xl font-semibold">{m.chat_workflow_runs()}</h2>
			<button class="btn btn-sm btn-primary" onclick={handleRunWorkflow}
				>{m.chat_start_new_run()} <Play class="size-4" /></button
			>
		</div>

		<div class="mb-8">
			{#if sessions.length > 3}
				<div class="list bg-base-100 rounded-box">
					<h3 class="px-4 pb-2 text-base font-semibold tracking-wide">
						{m.chat_most_recent_runs()}
					</h3>

					{#each recentRuns as thread, index (thread.id)}
						<button
							class="list-row hover:bg-base-200 text-left transition-colors"
							onclick={() => {
								goto(`/agent/p/${projectId}?tid=${thread.id}&pwid=${workflowId}`);
							}}
						>
							<div
								class="shrink-0 text-4xl font-thin tabular-nums opacity-30"
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

			{#if sessions.length === 0}
				<div
					class="bg-base-200/60 rounded-box flex flex-col items-center gap-3 px-6 py-10 text-center mt-2"
				>
					<div class="bg-base-100 rounded-full p-4">
						<Workflow class="size-7" />
					</div>
					<div class="space-y-1">
						<h3 class="font-medium">{m.chat_no_runs_yet()}</h3>
						<p class="text-base-content/60 text-sm">{m.chat_workflow_no_runs_desc()}</p>
					</div>
				</div>
			{:else}
				{#if sessions.length > 3}
					<h3 class="px-4 text-base font-semibold tracking-wide mt-8">{m.chat_all_runs()}</h3>
				{/if}
				<table class="table w-full">
					<thead>
						<tr>
							<th>{m.chat_title()}</th>
							{#if !responsive.isMobile}
								<th>{m.core_col_created()}</th>
							{/if}
							<th class="flex justify-end">
								<select class="select w-42" bind:value={sortBy}>
									<option value="" disabled selected>{m.chat_sort_by()}</option>
									<option value="created-desc">{m.chat_sort_created_newest()}</option>
									<option value="created-asc">{m.chat_sort_created_oldest()}</option>
									<option value="name-asc">{m.chat_sort_name_az()}</option>
									<option value="name-desc">{m.chat_sort_name_za()}</option>
								</select>
							</th>
						</tr>
					</thead>
					<tbody>
						{#each sortedThreads as thread (thread.id)}
							<tr
								class="list-row"
								onclick={() => {
									goto(`/agent/p/${projectId}?tid=${thread.id}&pwid=${workflowId}`);
								}}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										goto(`/agent/p/${projectId}?tid=${thread.id}&pwid=${workflowId}`);
									}
								}}
								aria-label={m.chat_view_thread_named({ title: thread.title ?? '' })}
								tabindex="0"
								role="button"
							>
								<td><span class="line-clamp-2">{thread.title}</span></td>
								{#if !responsive.isMobile}
									<td class="whitespace-nowrap">{formatTimeAgo(thread.created).relativeTime}</td>
								{/if}
								<td class="flex justify-end">
									<button class="btn btn-square btn-ghost">
										<Eye class="size-6" />
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	</div>
</div>

{#if loading}
	<div class="fixed top-0 left-0 z-50 flex h-full w-full items-center justify-center bg-black/50">
		<div class="loading loading-bars loading-lg"></div>
	</div>
{/if}

<Confirm
	msg={m.chat_delete_named({ name: workflowDisplayName || m.chat_this_workflow() })}
	show={deletingWorkflow}
	onsuccess={async () => {
		if (!workflow) return;
		if (publishedInfo) {
			await NanobotService.deletePublishedArtifact(publishedInfo.id);
			publishedWorkflows = await NanobotService.listPublishedWorkflows();
		}
		await $nanobotChat?.api.deleteWorkflow(workflow.uri);
		nanobotChat.update((data) => {
			if (data) {
				data.resources = data.resources.filter((r) => r.uri !== workflow.uri);
			}
			return data;
		});
		goto(`/agent/p/${projectId}/workflows`, { replaceState: true });
	}}
	oncancel={() => (deletingWorkflow = false)}
/>

{#if confirmInstallModal && relatedPublishedArtifact}
	<PublishedWorkflowInstallModal
		title={m.chat_workflow_update_title()}
		data={{
			...relatedPublishedArtifact,
			selectedVersion: confirmInstallModal.selectedVersion ?? relatedPublishedArtifact.latestVersion
		}}
		onClose={() => (confirmInstallModal = undefined)}
		onSuccess={() => {
			confirmInstallModal = undefined;
			reloadPage();
		}}
		confirmButtonText={m.core_update()}
		message={m.chat_update_workflow_confirm()}
	>
		{#snippet loadingText()}
			{m.chat_updating()}
			<i>{workflow?._meta?.displayName ?? workflow?._meta?.name ?? workflowId}...</i>
		{/snippet}
	</PublishedWorkflowInstallModal>
{/if}

{#if showWorkflowVersionDialog}
	<PublishedWorkflowVersionDialog
		publishedArtifactId={publishedInfo?.id}
		versions={publishedInfo?.versions ?? []}
		workflowDisplayName={publishedInfo?.displayName}
		onClose={() => (showWorkflowVersionDialog = false)}
		onChangeSubjects={(version, subjects) => {
			if (!publishedInfo) return;
			publishedInfo = {
				...publishedInfo,
				id: publishedInfo.id ?? '',
				versions: (publishedInfo.versions ?? []).map((entry) =>
					entry.version === version ? { ...entry, subjects } : entry
				)
			};
		}}
		onUnpublish={() => {
			showWorkflowVersionDialog = false;
			confirmUnpublish = true;
		}}
	/>
{/if}

{#if showConfirmPublishWorkflow}
	<ConfirmDiffWorkflow
		latestVersion={publishedInfo?.versions?.[publishedInfo?.versions?.length - 1]?.version ?? 0}
		workflowUri={workflow?.uri ?? ''}
		publishedArtifactId={publishedInfo?.id ?? ''}
		onSubmit={() => {
			if (!showConfirmPublishWorkflow) return;
			handlePublishWorkflow();
			showConfirmPublishWorkflow = false;
		}}
		onCancel={() => {
			showConfirmPublishWorkflow = false;
		}}
	/>
{/if}

{#if showConfirmUpdateWorkflow && relatedPublishedArtifact}
	<ConfirmDiffWorkflow
		latestVersion={relatedPublishedArtifact?.latestVersion ?? 0}
		workflowUri={workflow?.uri ?? ''}
		publishedArtifactId={relatedPublishedArtifact?.id ?? ''}
		onSubmit={(selectedVersion) => {
			if (!showConfirmUpdateWorkflow) return;
			confirmInstallModal = { selectedVersion: selectedVersion ?? undefined };
			showConfirmUpdateWorkflow = false;
		}}
		onCancel={() => {
			showConfirmUpdateWorkflow = false;
		}}
		variant="update"
		versions={relatedPublishedArtifact.versions}
		currentInstalledVersion={workflow?._meta?.version as string}
	/>
{/if}

{#if publishedInfo && latestVersion > 0}
	<Confirm
		show={confirmUnpublish}
		onsuccess={handleUnpublish}
		oncancel={() => (confirmUnpublish = false)}
		msg={latestVersion > 1 ? m.chat_unpublish_all_versions_q() : m.chat_unpublish_workflow_q()}
		type="info"
		title={m.chat_confirm_unpublish()}
	>
		{#snippet note()}
			<p>
				{latestVersion > 1 ? m.chat_unpublish_note_all() : m.chat_unpublish_note_one()}
			</p>
		{/snippet}
	</Confirm>
{/if}

<Confirm
	msg={m.chat_workflow_published_msg({ name: workflowDisplayName ?? workflowId })}
	title={m.chat_workflow_published_title()}
	cancelText={m.core_close()}
	show={showPublishSuccess}
	oncancel={() => (showPublishSuccess = false)}
	type="info"
>
	{#snippet note()}
		<p>
			{m.chat_published_to_version_prefix({ name: workflowDisplayName ?? workflowId })}
			<b class="font-semibold">{publishedInfo?.latestVersion?.toFixed(1)}</b
			>{m.chat_published_to_version_suffix()}
		</p>
		{#if publishedVersionSubjects.length === 0}
			<p class="mt-2">
				{m.chat_share_workflow_hint()}
			</p>
		{/if}
	{/snippet}
</Confirm>

<svelte:head>
	<title>{m.chat_page_title_named({ name: workflowDisplayName ?? workflowId })}</title>
</svelte:head>

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
				background-color: var(--color-base-200);
				transition: background-color 0.2s ease;
			}
		}
	}
</style>
