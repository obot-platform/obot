<script lang="ts">
	import { NanobotService } from '$lib/services';
	import type { PublishedArtifactVersion } from '$lib/services/nanobot/types';
	import { formatTimeAgo } from '$lib/time';
	import { ChevronLeft, Eye } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { fly } from 'svelte/transition';
	import { responsive } from '$lib/stores';

	interface Props {
		versions: PublishedArtifactVersion[];
		publishedArtifactId?: string;
		workflowDisplayName?: string;
		onClose: () => void;
	}

	let { versions, workflowDisplayName, onClose, publishedArtifactId }: Props = $props();
	let versionToShow = $state<PublishedArtifactVersion | undefined>(undefined);

	let loadingVersion = $state(false);
	let versionContents = $state<string>('');

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
</script>

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

{#snippet versionHistoryDialog()}
	<div
		class="modal-box dialog-container flex h-full w-full max-w-full flex-col md:max-h-fit md:max-w-lg"
	>
		<form method="dialog">
			<button class="btn btn-circle btn-ghost btn-sm absolute top-2 right-2" onclick={onClose}
				>✕</button
			>
		</form>
		<h3 class="text-lg font-semibold">{workflowDisplayName} | Version History</h3>
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
						<div class="timeline-end mt-2 mb-4 w-full pl-2">
							<div class="flex items-center gap-2">
								<div class="flex w-[calc(100%-32px)] flex-col gap-1">
									<p class="text-xs font-light">
										{formatTimeAgo(version.createdAt).relativeTime}
									</p>
									<p class="line-clamp-1 truncate text-sm">{version.description}</p>
								</div>
								<button
									class="btn btn-square btn-sm btn-ghost tooltip tooltip-left"
									data-tip="View version SKILLS.md"
									onclick={() => fetchVersionContents(version)}
								>
									<Eye class="size-4" />
								</button>
							</div>
						</div>
						{#if index < sortedVersions.length - 1}
							<hr />
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
		<div class="flex grow"></div>

		<div class="modal-action mt-2">
			<button class="btn" onclick={onClose}>Close</button>
		</div>
	</div>
{/snippet}

{#snippet versionContentsDialog()}
	<div
		in:fly={{ x: 100, duration: 150 }}
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
