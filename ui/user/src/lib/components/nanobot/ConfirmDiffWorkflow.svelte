<script lang="ts">
	import { onMount } from 'svelte';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { NanobotService } from '$lib/services';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		latestVersion: number;
		workflowUri: string;
		publishedArtifactId: string;
		contentsToCompare?: string;
		onSubmit: () => void;
		onCancel: () => void;
		variant?: 'update' | 'publish';
	}

	let {
		latestVersion,
		workflowUri,
		publishedArtifactId,
		contentsToCompare,
		variant = 'publish',
		onSubmit,
		onCancel
	}: Props = $props();
	let latestVersionContents = $state<string>('');
	let currentWorkflowContents = $state<string>('');
	let loading = $state(false);

	onMount(async () => {
		loading = true;

		if (latestVersion > 0) {
			try {
				latestVersionContents = await NanobotService.getPublishedArtifactVersionContents(
					publishedArtifactId,
					latestVersion
				);

				if (contentsToCompare) {
					currentWorkflowContents = contentsToCompare;
				} else {
					const result = await $nanobotChat?.api.readResource(workflowUri);
					if (result?.contents?.length) {
						currentWorkflowContents = result.contents[0]?.text ?? '';
					}
				}
			} catch (err) {
				console.error(err);
			} finally {
				loading = false;
			}
		} else {
			loading = false;
		}
	});
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
				{#if variant === 'publish'}
					<div class="w-full md:w-1/2">
						<h4 class="text-md font-semibold">Most Recent Version</h4>
						<div class="bg-base-200 h-[calc(100dvh-16rem)] overflow-y-auto rounded-lg px-2">
							{#if loading}
								<div class="flex items-center justify-center gap-2 py-8">
									<span class="loading loading-sm loading-spinner"></span>
								</div>
							{:else}
								<MarkdownEditor value={latestVersionContents} readonly />
							{/if}
						</div>
					</div>
					<div class="w-full md:w-1/2">
						<h4 class="text-md font-semibold">Current</h4>
						{#if loading}
							<div class="flex items-center justify-center gap-2 py-8">
								<span class="loading loading-sm loading-spinner"></span>
							</div>
						{:else}
							<div class="bg-base-200 h-[calc(100dvh-16rem)] overflow-y-auto rounded-lg px-2">
								<MarkdownEditor value={currentWorkflowContents} readonly />
							</div>
						{/if}
					</div>
				{:else}
					<div class="w-full md:w-1/2">
						<h4 class="text-md font-semibold">Current Workflow</h4>
						<div class="bg-base-200 h-[calc(100dvh-16rem)] overflow-y-auto rounded-lg px-2">
							{#if loading}
								<div class="flex items-center justify-center gap-2 py-8">
									<span class="loading loading-sm loading-spinner"></span>
								</div>
							{:else}
								<MarkdownEditor value={currentWorkflowContents} readonly />
							{/if}
						</div>
					</div>
					<div class="w-full md:w-1/2">
						<h4 class="text-md font-semibold">Version {latestVersion.toFixed(1)}</h4>
						{#if loading}
							<div class="flex items-center justify-center gap-2 py-8">
								<span class="loading loading-sm loading-spinner"></span>
							</div>
						{:else}
							<div class="bg-base-200 h-[calc(100dvh-16rem)] overflow-y-auto rounded-lg px-2">
								<MarkdownEditor value={latestVersionContents} readonly />
							</div>
						{/if}
					</div>
				{/if}
			</div>
		{:else}
			<p>
				{variant === 'publish'
					? 'Would you like to publish this workflow?'
					: 'Would you like to update this workflow?'}
			</p>
		{/if}
		<div class="flex grow"></div>
		<div class="modal-action">
			<button class="btn btn-primary" onclick={onSubmit}>
				{variant === 'publish' ? 'Publish' : 'Update'}
			</button>
			<button class="btn btn-secondary" onclick={onCancel}>Cancel</button>
		</div>
	</div>
</dialog>
