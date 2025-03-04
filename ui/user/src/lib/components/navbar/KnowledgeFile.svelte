<script lang="ts">
	import Tooltip from '$lib/components/shared/tooltip/Tooltip.svelte';
	import Truncate from '$lib/components/shared/tooltip/Truncate.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { type KnowledgeFile } from '$lib/services';
	import { CircleX, FileText, Trash } from 'lucide-svelte/icons';

	interface Props {
		onDelete?: () => void;
		file: KnowledgeFile;
	}

	const { onDelete, file }: Props = $props();

	let truncateEl = $state<ReturnType<typeof Truncate>>();
	let isError = $derived(file.state === 'error' || file.state === 'failed');
</script>

<div class="space-between group flex gap-2">
	<Tooltip disabled={!isError && !truncateEl?.truncated}>
		<button class="flex flex-1 items-center">
			<FileText class="size-5 min-w-fit" />
			<Truncate class="ms-3" text={file.fileName} disabled bind:this={truncateEl} />
			{#if file.state === 'error' || file.state === 'failed'}
				<CircleX class="ms-2 h-4 text-red-600" />
			{:else if file.state === 'pending' || file.state === 'ingesting'}
				<Loading class="mx-1.5" />
			{/if}
		</button>

		{#snippet content()}
			<p
				class="rounded-xl bg-blue-500 px-2 py-1 text-white dark:text-black"
				class:bg-red-600={isError}
			>
				{isError ? (file.error ?? 'Failed') : file.fileName}
			</p>
		{/snippet}
	</Tooltip>

	<button
		class="hidden group-hover:block"
		onclick={() => {
			if (file.state === 'ingested') {
				onDelete?.();
			}
		}}
	>
		<Trash class="h-5 w-5 text-gray" />
	</button>
</div>
