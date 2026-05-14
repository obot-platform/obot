<script lang="ts">
	import { generateLineDiff, formatTextWithDiffHighlighting } from '$lib/diff';
	import { NanobotService } from '$lib/services';
	import type { PublishedArtifactVersion } from '$lib/services/nanobot/types';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { onMount, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		latestVersion: number;
		workflowUri: string;
		publishedArtifactId: string;
		contentsToCompare?: string;
		onSubmit: (version?: number) => void;
		onCancel: () => void;
		variant?: 'update' | 'publish';
		versions?: PublishedArtifactVersion[];
		currentInstalledVersion?: string;
	}

	let {
		latestVersion,
		workflowUri,
		publishedArtifactId,
		contentsToCompare,
		variant = 'publish',
		onSubmit,
		onCancel,
		versions,
		currentInstalledVersion
	}: Props = $props();
	let latestVersionContents = $state<string>('');
	let currentWorkflowContents = $state<string>('');
	let loading = $state(false);
	let selectedVersion = $state(untrack(() => latestVersion));

	let sortedVersions = $derived(
		versions ? [...versions].sort((a, b) => b.version - a.version) : []
	);
	let workflowDiff = $derived(
		latestVersionContents && currentWorkflowContents
			? variant === 'publish'
				? generateLineDiff(latestVersionContents, currentWorkflowContents)
				: generateLineDiff(currentWorkflowContents, latestVersionContents)
			: null
	);
	let currentHighlightedHtml = $derived(
		workflowDiff ? formatTextWithDiffHighlighting(workflowDiff, true) : ''
	);
	let latestHighlightedHtml = $derived(
		workflowDiff ? formatTextWithDiffHighlighting(workflowDiff, false) : ''
	);

	async function getVersionContents(version: number) {
		latestVersionContents = await NanobotService.getPublishedArtifactVersionContents(
			publishedArtifactId,
			version
		);

		if (contentsToCompare) {
			currentWorkflowContents = contentsToCompare;
		} else {
			const result = await $nanobotChat?.api.readResource(workflowUri);
			if (result?.contents?.length) {
				currentWorkflowContents = result.contents[0]?.text ?? '';
			}
		}
	}

	onMount(async () => {
		loading = true;

		if (latestVersion > 0) {
			try {
				await getVersionContents(latestVersion);
			} catch (err) {
				console.error('Failed to initialize with latest workflow version contents', err);
			} finally {
				loading = false;
			}
		} else {
			loading = false;
		}
	});

	function handleSelectVersion(version: number) {
		loading = true;
		getVersionContents(version)
			.catch((err) => {
				console.error(`Failed to get version ${version} contents`, err);
			})
			.finally(() => {
				loading = false;
			});
	}
</script>

<dialog class="modal-open modal">
	<div
		class={twMerge(
			'modal-box dialog-container flex h-full max-h-full w-full max-w-full flex-col',
			latestVersion === 0
				? 'max-h-fit max-w-md'
				: 'max-h-[calc(100dvh-4rem)] max-w-[calc(100dvw-4rem)]'
		)}
	>
		<form method="dialog">
			<button class="btn btn-circle btn-ghost btn-sm absolute top-2 right-2" onclick={onCancel}
				>✕</button
			>
		</form>
		<h3 class="mb-2 text-xl font-semibold">
			{variant === 'publish' ? 'Publish Workflow' : 'Update Workflow'}
		</h3>
		{#if latestVersion > 0}
			<div class="flex w-full flex-col gap-2 md:flex-row">
				<div class="w-full md:w-1/2">
					<h4 class="text-md font-semibold">
						{variant === 'publish' ? 'Most Recent Version' : 'Current Workflow'}
					</h4>
					{#if loading}
						<div class="flex items-center justify-center gap-2 py-8">
							<span class="loading loading-sm loading-spinner"></span>
						</div>
					{:else if workflowDiff}
						<div class="bg-base-200 h-[calc(100dvh-16rem)] overflow-y-auto rounded-lg px-2">
							<div class="py-1">{@html currentHighlightedHtml}</div>
						</div>
					{:else}
						<div class="bg-base-200 h-[calc(100dvh-16rem)] overflow-y-auto rounded-lg px-2">
							<MarkdownEditor value={currentWorkflowContents} readonly />
						</div>
					{/if}
				</div>
				<div class="w-full md:w-1/2">
					<h4 class="text-md font-semibold">
						{#if variant === 'publish'}
							Current
						{:else}
							Version {selectedVersion.toFixed(1)}
							{#if selectedVersion === latestVersion}
								<span class="text-base-content/50 text-xs font-light">(latest)</span>
							{/if}
						{/if}
					</h4>
					<div class="bg-base-200 h-[calc(100dvh-16rem)] overflow-y-auto rounded-lg px-2">
						{#if loading}
							<div class="flex items-center justify-center gap-2 py-8">
								<span class="loading loading-sm loading-spinner"></span>
							</div>
						{:else if workflowDiff}
							<div class="py-1">{@html latestHighlightedHtml}</div>
						{:else}
							<MarkdownEditor value={latestVersionContents} readonly />
						{/if}
					</div>
				</div>
			</div>
		{:else}
			<p>
				{variant === 'publish'
					? 'Would you like to publish this workflow?'
					: 'Would you like to update this workflow?'}
			</p>
		{/if}
		<div class="flex grow"></div>
		<div class="modal-action flex gap-4">
			{#if variant === 'update'}
				<select
					class="select select-bordered flex grow"
					onchange={(e) => {
						const version = Number((e.target as HTMLSelectElement).value);
						if (isNaN(version)) return;
						handleSelectVersion(version);
					}}
					bind:value={selectedVersion}
				>
					{#each sortedVersions as version (version.version)}
						<option value={version.version}>
							{version.version.toFixed(1)}
							{#if version.version === latestVersion}
								(latest)
							{/if}
						</option>
					{/each}
				</select>
			{/if}

			{#if currentInstalledVersion === selectedVersion.toString()}
				<button class="btn w-64" disabled>
					{currentInstalledVersion === latestVersion.toString()
						? 'Currently up-to-date'
						: 'Currently installed'}
				</button>
			{:else}
				<button
					class={twMerge('btn btn-primary', variant === 'update' ? 'w-64' : 'w-full')}
					onclick={() => onSubmit(variant === 'publish' ? undefined : selectedVersion)}
				>
					{variant === 'publish' ? 'Publish' : `Update to ${selectedVersion.toFixed(1)}`}
				</button>
			{/if}
		</div>
	</div>
</dialog>
