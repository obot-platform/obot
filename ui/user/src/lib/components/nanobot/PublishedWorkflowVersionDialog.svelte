<script lang="ts">
	import { NanobotService } from '$lib/services';
	import type { PublishedArtifactVersion } from '$lib/services/nanobot/types';
	import { responsive } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import Confirm from '../Confirm.svelte';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { ChevronLeft, CircleAlert } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		versions: PublishedArtifactVersion[];
		publishedArtifactId?: string;
		workflowDisplayName?: string;
		onClose: () => void;
		onChangeStatus: (status: 'public' | 'private') => void;
		onUnpublish: () => void;
		status?: 'public' | 'private';
	}

	let {
		versions,
		workflowDisplayName,
		onClose,
		onChangeStatus,
		onUnpublish,
		publishedArtifactId,
		status
	}: Props = $props();
	let versionToShow = $state<PublishedArtifactVersion | undefined>(undefined);

	let loadingVersion = $state(false);
	let versionContents = $state<string>('');

	let changingStatus = $state<'public' | 'private' | undefined>(undefined);

	let sortedVersions = $derived([...versions].sort((a, b) => b.version - a.version));

	async function fetchVersionContents(selectedVersion: PublishedArtifactVersion) {
		if (!publishedArtifactId) return;
		versionToShow = selectedVersion;
		loadingVersion = true;
		versionContents = '';
		try {
			versionContents = await NanobotService.getPublishedArtifactVersionContents(
				publishedArtifactId,
				selectedVersion.version
			);
		} catch (error) {
			console.error('Failed to fetch published artifact version contents', error);
		} finally {
			loadingVersion = false;
		}
	}

	async function handleStatusChange() {
		if (!publishedArtifactId || !changingStatus) return;

		try {
			await NanobotService.updatePublishedArtifact(publishedArtifactId, {
				visibility: changingStatus
			});
			onChangeStatus?.(changingStatus as 'public' | 'private');
		} catch (err) {
			console.error('Failed to update published artifact status', err);
		} finally {
			changingStatus = undefined;
		}
	}
</script>

{#if !changingStatus}
	<dialog class="modal-open modal flex items-center justify-center gap-4">
		{#if responsive.isMobile}
			{#if versionToShow}
				{@render versionContentsDialog()}
			{:else}
				{@render versionHistoryDialog()}
			{/if}
		{:else}
			{@render versionHistoryDialog()}
			{#if versionToShow}
				{@render versionContentsDialog()}
			{/if}
		{/if}
	</dialog>
{/if}

{#snippet versionHistoryDialog()}
	<div
		class="modal-box dialog-container flex h-full w-full max-w-full flex-col md:max-h-fit md:max-w-lg"
	>
		<form method="dialog">
			<button class="btn btn-circle btn-ghost btn-sm absolute top-2 right-2" onclick={onClose}
				>✕</button
			>
		</form>
		<div class="flex items-center gap-2">
			<h3 class="text-lg font-semibold">{workflowDisplayName}</h3>
			{#if status}
				<span
					class={twMerge(
						'badge badge-sm font-semibold uppercase',
						status === 'public' ? 'badge-success text-white' : 'badge-secondary'
					)}
				>
					{status}
				</span>
			{/if}
		</div>

		<div class="bg-primary/10 mt-2 flex items-center gap-2 rounded-md px-4 py-2">
			<CircleAlert class="text-primary size-5 flex-shrink-0" />
			<p class="text-base-content mt-1 text-sm font-light">
				{#if status === 'public'}
					This workflow is currently public and all versions are visible to other users.
				{:else}
					This workflow is currently private and only visible to you.
				{/if}
			</p>
		</div>

		{#if versions.length === 0}
			<div class="text-base-content/50 my-4 text-center text-sm">
				<p class="font-medium">No versions found.</p>
			</div>
		{:else}
			<ul class="timeline timeline-snap-icon timeline-compact timeline-vertical mt-4 pr-2">
				{#each sortedVersions as version, index (version.version)}
					<li>
						{#if index > 0}
							<hr />
						{/if}
						<div class="timeline-middle">
							<div
								class={twMerge(
									'badge badge-sm badge-soft w-12',
									index === 0 ? 'badge-primary' : 'badge-secondary'
								)}
							>
								{parseFloat(version.version.toString()).toFixed(1)}
							</div>
						</div>
						<button
							class="timeline-end hover:bg-base-200 group mt-1 mb-2 w-full rounded-sm px-2 py-1 text-left"
							onclick={() => fetchVersionContents(version)}
						>
							<div class="flex items-center gap-2">
								<div class="flex w-[calc(100%-32px)] flex-col gap-1">
									<p class="text-xs font-light">
										{formatTimeAgo(version.createdAt).relativeTime}
									</p>
									<p class="btn-link group-hover:text-primary line-clamp-1 truncate text-sm">
										{version.description}
									</p>
								</div>
							</div>
						</button>
						{#if index < sortedVersions.length - 1}
							<hr />
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
		<div class="flex grow"></div>

		<div class="modal-action mt-4">
			<button
				class="btn btn-secondary"
				onclick={() => {
					onUnpublish();
				}}
			>
				{versions.length > 1 ? 'Unpublish All' : 'Unpublish'}
			</button>
			<button
				class="btn btn-primary"
				onclick={() => {
					changingStatus = status === 'public' ? 'private' : 'public';
				}}
			>
				{status == 'public' ? 'Make Private' : 'Make Public'}
			</button>
		</div>
	</div>
{/snippet}

{#snippet versionContentsDialog()}
	<div
		transition:fly={{ x: 100, duration: 150 }}
		class="modal-box dialog-container h-full max-h-full w-full max-w-full md:max-h-[70dvh] md:max-w-4xl"
	>
		<form method="dialog">
			<button
				class="btn btn-circle btn-ghost btn-sm absolute top-2 right-2"
				onclick={() => (versionToShow = undefined)}>✕</button
			>
		</form>
		<div class="flex items-center gap-2">
			{#if responsive.isMobile}
				<button class="btn btn-circle btn-ghost" onclick={() => (versionToShow = undefined)}>
					<ChevronLeft class="size-5" />
				</button>
			{/if}
			<h3 class="text-lg font-semibold">
				{workflowDisplayName} | Version {versionToShow?.version}
			</h3>
		</div>
		<div class="mt-2 min-h-[200px]">
			{#if loadingVersion}
				<div class="flex items-center justify-center gap-2 py-8">
					<span class="loading loading-sm loading-spinner"></span>
					<span>Loading version contents...</span>
				</div>
			{:else if versionContents}
				<div
					class="nanobot border-base-300 bg-base-200 max-h-[60vh] overflow-auto rounded-md border p-2"
				>
					<MarkdownEditor value={versionContents} readonly />
				</div>
			{/if}
		</div>
	</div>
{/snippet}

<Confirm
	show={!!changingStatus}
	onsuccess={handleStatusChange}
	oncancel={() => (changingStatus = undefined)}
	msg={changingStatus === 'public'
		? `Make ${workflowDisplayName} Public?`
		: `Make ${workflowDisplayName} Private?`}
	type="info"
	title="Confirm Workflow Status"
>
	{#snippet note()}
		{#if changingStatus}
			<p>
				Are you sure you want to make this workflow {changingStatus}?
			</p>
			<p>
				{changingStatus === 'public'
					? 'All versions will be made public and will be visible to other users.'
					: 'All versions will be made private and will no longer be visible to other users.'}
			</p>
		{/if}
	{/snippet}
</Confirm>
