<script lang="ts">
	import { NanobotService } from '$lib/services';
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
		onPublish?: () => void;
		onUnpublish: () => void;
		onCheckForUpdates?: (publishedArtifactId: string) => void;
		onDelete?: () => void;
		disabled?: boolean;
		numVersions?: number;
	}

	let {
		publishedArtifactId,
		onPublish,
		onUnpublish,
		onCheckForUpdates,
		onDelete,
		disabled,
		numVersions = 0
	}: Props = $props();
</script>

<div class="dropdown dropdown-left" in:fade={{ duration: 150 }}>
	<button
		class="btn btn-ghost btn-square flex-shrink-0"
		onclick={async (e) => {
			e.preventDefault();
			e.stopPropagation();
		}}
		aria-label="More options"
	>
		<EllipsisVertical class="size-4" />
	</button>
	<ul class="dropdown-content menu dropdown-menu w-52">
		{#if publishedArtifactId}
			{#if onPublish}
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
						<BookCopy class="size-4" /> Publish
					</button>
				</li>
			{/if}
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
					<BookDashed class="size-4" />
					{numVersions > 1 ? 'Unpublish All' : 'Unpublish'}
				</button>
			</li>
		{:else if onPublish}
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
		{#if onCheckForUpdates}
			<li>
				<button
					class="text-sm"
					{disabled}
					onclick={(e) => {
						e.preventDefault();
						e.stopPropagation();
						if (!publishedArtifactId) return;
						onCheckForUpdates?.(publishedArtifactId);
						e.currentTarget.blur();
					}}
				>
					<CircleArrowUp class="size-4" /> Update
				</button>
			</li>
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
