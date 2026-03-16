<script lang="ts">
	import { NanobotService } from '$lib/services';
	import { parseFrontmatter, splitFrontmatter } from '$lib/services/nanobot/utils';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import {
		Book,
		BookCopy,
		BookDashed,
		CircleArrowUp,
		EllipsisVertical,
		Trash2
	} from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		publishedArtifactId?: string;
		onPublish: () => void;
		onUnpublish: () => void;
		onCheckForUpdates: (publishedArtifactId: string) => void;
		onDelete?: () => void;
		disabled?: boolean;
		workflowUri?: string;
		relatedPublishedArtifactId?: string;
	}

	let {
		publishedArtifactId,
		onPublish,
		onUnpublish,
		onCheckForUpdates,
		onDelete,
		disabled,
		workflowUri,
		relatedPublishedArtifactId
	}: Props = $props();
	let checkingForUpdates = $state(false);
	let publishedArtifactIdToUse = $state<string>('');

	async function handleUpdateCheck() {
		if (!publishedArtifactIdToUse) return;
		onCheckForUpdates(publishedArtifactIdToUse);
	}
</script>

<div class="dropdown dropdown-left" in:fade={{ duration: 150 }}>
	<button
		class="btn btn-ghost btn-square flex-shrink-0"
		onclick={async (e) => {
			e.preventDefault();
			e.stopPropagation();
			checkingForUpdates = true;
			if (relatedPublishedArtifactId) {
				publishedArtifactIdToUse = relatedPublishedArtifactId;
			} else if (workflowUri) {
				const resourceResponse = await $nanobotChat?.api.readResource(workflowUri);
				const content = resourceResponse?.contents[0].text || '';
				const { frontmatter } = splitFrontmatter(content);
				const parsed = parseFrontmatter(frontmatter);
				publishedArtifactIdToUse = (parsed.metadata?.id ?? '') as string;
			}
			checkingForUpdates = false;
		}}
		aria-label="More options"
	>
		<EllipsisVertical class="size-4" />
	</button>
	<ul class="dropdown-content menu dropdown-menu w-52">
		{#if publishedArtifactId}
			<li>
				<button
					class="text-sm"
					onclick={async (e) => {
						e.preventDefault();
						e.stopPropagation();
						onPublish();
						e.currentTarget.blur();
					}}
					{disabled}
				>
					<BookCopy class="size-4" /> Republish
				</button>
			</li>
			<li>
				<button
					class="text-sm"
					onclick={async (e) => {
						e.preventDefault();
						e.stopPropagation();
						await NanobotService.deletePublishedArtifact(publishedArtifactId);
						onUnpublish();
						e.currentTarget.blur();
					}}
					{disabled}
				>
					<BookDashed class="size-4" /> Unpublish
				</button>
			</li>
		{:else}
			<li>
				<button
					class="text-sm"
					onclick={(e) => {
						e.preventDefault();
						e.stopPropagation();
						onPublish();
						e.currentTarget.blur();
					}}
					{disabled}
				>
					<Book class="size-4" /> Publish
				</button>
			</li>
		{/if}
		{#if publishedArtifactIdToUse}
			{#if checkingForUpdates}
				<li>
					<div>
						<span class="loading loading-spinner loading-xs"></span>
					</div>
				</li>
			{:else}
				<li>
					<button
						class="text-sm"
						{disabled}
						onclick={(e) => {
							e.preventDefault();
							e.stopPropagation();
							handleUpdateCheck();
							e.currentTarget.blur();
						}}
					>
						<CircleArrowUp class="size-4" /> Update
					</button>
				</li>
			{/if}
		{/if}
		{#if onDelete}
			<li>
				<button
					class="text-error hover:bg-error/10 text-sm"
					onclick={(e) => {
						e.preventDefault();
						e.stopPropagation();
						onDelete();
					}}
					{disabled}
				>
					<Trash2 class="size-4" /> Delete workflow
				</button>
			</li>
		{/if}
	</ul>
</div>
