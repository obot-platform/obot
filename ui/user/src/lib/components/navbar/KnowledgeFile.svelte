<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { type KnowledgeFile } from '$lib/services';
	import { CircleX, FileText, Trash2 } from 'lucide-svelte/icons';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		onDelete?: () => void;
		file: KnowledgeFile;
	}

	const { onDelete, file }: Props = $props();
	let isError = $derived(file.state === 'error' || file.state === 'failed');
</script>

<div class="space-between group flex gap-2">
	<button
		class="flex flex-1 items-center truncate"
		use:tooltip={isError ? (file.error ?? 'Failed') : file.fileName}
	>
		<FileText class="size-4 min-w-fit" />
		<span class="ms-3 truncate text-sm">{file.fileName}</span>
		{#if file.state === 'error' || file.state === 'failed'}
			<CircleX class="ms-2 h-4 text-red-500" />
		{:else if file.state === 'pending' || file.state === 'ingesting'}
			<Loading class="mx-1.5" />
		{/if}
	</button>

	<button
		class="hidden group-hover:block"
		onclick={() => {
			if (file.state === 'ingested') {
				onDelete?.();
			}
		}}
	>
		<Trash2 class="text-gray size-5" />
	</button>
</div>
