<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { type KnowledgeFile } from '$lib/services';
	import { CircleX, FileText, Trash2 } from 'lucide-svelte/icons';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		onDelete?: () => void;
		file: KnowledgeFile;
		iconSize?: number;
		classes?: {
			delete?: string;
		};
	}

	const { onDelete, file, iconSize = 4, classes }: Props = $props();
	let isError = $derived(file.state === 'error' || file.state === 'failed');
</script>

<div class="space-between group flex items-center gap-2">
	<button
		class="flex flex-1 items-center gap-1 truncate"
		use:tooltip={isError ? (file.error ?? 'Failed') : file.fileName}
	>
		<div class="flex items-center gap-1">
			<FileText class={`size-${iconSize} min-w-fit`} />
			<span class="truncate">{file.fileName}</span>
		</div>
		{#if file.state === 'error' || file.state === 'failed'}
			<CircleX class={`ms-2 size-${iconSize} text-red-500`} />
		{:else if file.state === 'pending' || file.state === 'ingesting'}
			<Loading class="mx-1.5" />
		{/if}
	</button>

	{#if onDelete}
		<button
			class={classes?.delete}
			onclick={() => {
				if (file.state === 'ingested') {
					onDelete();
				}
			}}
		>
			<Trash2 class={`size-${iconSize} text-gray`} />
		</button>
	{/if}
</div>
